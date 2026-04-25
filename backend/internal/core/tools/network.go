package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// NetworkTools provides native capability-based network operations.
type NetworkTools struct {
	configProvider func(ctx context.Context) models.NetworkGuardrailsConfig
	httpClient     *http.Client
	logger         logging.Logger
}

func NewNetworkTools(provider func(ctx context.Context) models.NetworkGuardrailsConfig, logger logging.Logger) *NetworkTools {
	n := &NetworkTools{
		configProvider: provider,
		logger:         logger,
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Custom transport with DialContext for DNS rebinding protection and connection pooling
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			cfg := n.configProvider(ctx)

			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address format: %w", err)
			}

			// Use net.DefaultResolver with context for DNS lookups
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}

			// Validate all resolved IPs to prevent rebinding attacks
			for _, ip := range ips {
				if err := n.validateIP(ip, cfg); err != nil {
					return nil, err
				}
			}

			// Dial the first resolved IP
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	n.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil // Transport handles IP validation for redirects
		},
	}

	return n
}

// FetchURL downloads the content of a remote URL.
func (n *NetworkTools) Config(ctx context.Context) models.NetworkGuardrailsConfig {
	return n.configProvider(ctx)
}

func (n *NetworkTools) FetchURL(ctx context.Context, targetURL string) (string, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled {
		return "", fmt.Errorf("network tools are disabled")
	}

	if !cfg.AllowInternetAccess && !cfg.AllowLanAccess {
		return "", fmt.Errorf("network access is fully blocked by guardrails")
	}

	// Security Check: Blocked Domains
	host := ExtractHost(targetURL)
	if err := ValidateDomainBoundary(host, cfg.BlockedDomains); err != nil {
		return "", err
	}

	// Initial pre-check (best effort, real check is in DialContext)
	if err := n.validateAddress(ctx, targetURL, cfg); err != nil {
		return "", err
	}

	// Use the shared httpClient which has the security-locked DialContext and pooling
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", "LLM-Proxy-Agent/1.0")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		n.logger.Error("Network fetch failed", "url", targetURL, "error", err)
		return "", fmt.Errorf("failed to fetch content from the provided URL")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned unexpected status: %s", resp.Status)
	}

	limit := int64(cfg.MaxFetchSizeKB) * 1024
	if limit <= 0 {
		limit = 1024 * 1024 // 1MB default
	}

	lr := &io.LimitedReader{R: resp.Body, N: limit}
	data, err := io.ReadAll(lr)
	if err != nil && err != io.EOF {
		return "", err
	}

	result := string(data)
	if lr.N <= 0 {
		result += "\n... (content truncated by network guardrails)"
	}

	return result, nil
}

// resolveAndQueueTargets handles the expansion and validation of comma-separated targets.
func (n *NetworkTools) resolveAndQueueTargets(ctx context.Context, targetStr string, cfg models.NetworkGuardrailsConfig, jobs chan<- string) {
	targetParts := strings.SplitSeq(targetStr, ",")
	for rawTarget := range targetParts {
		target := strings.TrimSpace(rawTarget)
		if target == "" {
			continue
		}

		if strings.Contains(target, "/") {
			// Subnet expansion
			_, ipnet, err := net.ParseCIDR(target)
			if err != nil {
				continue
			}
			prefix := ipnet.IP.Mask(ipnet.Mask).String()
			prefixBase := prefix[:strings.LastIndex(prefix, ".")+1]
			for i := 1; i < 255; i++ {
				targetIP := fmt.Sprintf("%s%d", prefixBase, i)
				if err := n.validateIP(net.ParseIP(targetIP), cfg); err != nil {
					continue
				}
				select {
				case jobs <- targetIP:
				case <-ctx.Done():
					return
				}
			}
		} else {
			// Single IP/Host
			if ip := net.ParseIP(target); ip != nil {
				if err := n.validateIP(ip, cfg); err != nil {
					continue
				}
			} else if err := n.validateAddress(ctx, target, cfg); err != nil {
				continue
			}
			select {
			case jobs <- target:
			case <-ctx.Done():
				return
			}
		}
	}
}

// validateAddress ensures the target host complies with LAN/Internet access rules.
func (n *NetworkTools) validateAddress(ctx context.Context, address string, cfg models.NetworkGuardrailsConfig) error {
	host := ExtractHost(address)

	// Domain Check at label boundaries
	if err := ValidateDomainBoundary(host, cfg.BlockedDomains); err != nil {
		return err
	}

	// SEC-H2: Use net.DefaultResolver with context
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		// If DNS fails, we might be dealing with a raw IP or a blocked segment
		return nil // We'll let the dialer handle the error
	}

	for _, ip := range ips {
		if err := n.validateIP(ip, cfg); err != nil {
			return err
		}
	}

	return nil
}

func (n *NetworkTools) validateIP(ip net.IP, cfg models.NetworkGuardrailsConfig) error {
	// SAFETY: Loopback and Link-Local are ALWAYS blocked for agents to prevent
	// poking at host-only services or cloud metadata APIs.
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("access to loopback/link-local address '%s' is strictly prohibited", ip.String())
	}

	isPrivate := ip.IsPrivate()

	if isPrivate && !cfg.AllowLanAccess {
		return fmt.Errorf("access to private address '%s' is blocked", ip.String())
	}
	if !isPrivate && !cfg.AllowInternetAccess {
		return fmt.Errorf("outbound internet access to '%s' is blocked", ip.String())
	}

	for _, blocked := range cfg.BlockedIPs {
		if ip.String() == blocked {
			return fmt.Errorf("access to IP '%s' is explicitly blocked", ip.String())
		}
	}
	return nil
}

type ScanArgs struct {
	Target    string `json:"target,omitempty"`     // IP or CIDR (e.g. 192.168.1.1 or 192.168.1.0/24)
	Mode      string `json:"mode,omitempty"`       // "fast" or "deep"
	Ports     []int  `json:"ports,omitempty"`      // Custom port list
	TimeoutMs int    `json:"timeout_ms,omitempty"` // Per-IP timeout
}

// GetNetworkInfo returns the local IP and subnet mask.
func (n *NetworkTools) GetNetworkInfo(ctx context.Context) (string, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled || !cfg.AllowLanAccess {
		return "", fmt.Errorf("network discovery is disabled or LAN access is blocked")
	}

	ip, subnet, err := n.getLocalSubnet()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Local IP: %s\nSubnet Mask: %s\nSubnet CIDR: %s", ip, subnet.Mask.String(), subnet.String()), nil
}

// ScanLocalNetwork performs a concurrent discovery or audit scan.
func (n *NetworkTools) ScanLocalNetwork(ctx context.Context, args ScanArgs) (string, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled || !cfg.AllowLanAccess {
		return "", fmt.Errorf("network scanning is disabled or LAN access is blocked")
	}

	// 1. Determine Target Range
	var base string
	if args.Target != "" {
		if err := ValidateScanTargets(args.Target); err != nil {
			return "", err
		}
		base = args.Target
	} else {
		// Auto-detect local
		_, subnet, err := n.getLocalSubnet()
		if err != nil {
			return "", fmt.Errorf("failed to detect local subnet: %w", err)
		}
		base = subnet.String()
	}

	// 2. Select Ports
	ports := args.Ports
	if len(ports) == 0 {
		if args.Mode == "deep" {
			// Top 20 common ports
			ports = []int{21, 22, 23, 25, 53, 80, 110, 111, 135, 139, 143, 443, 445, 993, 995, 1723, 3306, 3389, 5900, 8080}
		} else {
			// Top 5 discovery ports
			ports = []int{22, 80, 443, 8080, 3000}
		}
	}

	timeout := 500 * time.Millisecond
	if args.TimeoutMs > 0 {
		timeout = time.Duration(args.TimeoutMs) * time.Millisecond
	}

	dialer := &net.Dialer{Timeout: timeout}
	results := make(chan string, 1000)
	jobs := make(chan string, 256) // Sized for a /24 subnet

	// 3. Worker Pool for concurrent scanning
	// Increase worker pool to 50 to handle larger scans efficiently while maintaining goroutine bound (NET-L1)
	const numWorkers = 50
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				for _, port := range ports {
					// Check context for early termination
					select {
					case <-ctx.Done():
						return
					default:
					}

					addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
					conn, err := dialer.DialContext(ctx, "tcp", addr)
					if err == nil {
						// Banner Grabbing
						conn.SetReadDeadline(time.Now().Add(timeout))
						banner := make([]byte, 256)
						bytesRead, _ := conn.Read(banner)

						serviceInfo := "Active Service"
						if bytesRead > 0 {
							serviceInfo = strings.TrimSpace(string(banner[:bytesRead]))
							serviceInfo = strings.ReplaceAll(serviceInfo, "\n", " ")
							if len(serviceInfo) > 64 {
								serviceInfo = serviceInfo[:61] + "..."
							}
						} else {
							serviceInfo = n.probeHTTP(ctx, ip, port)
						}

						conn.Close()
						results <- fmt.Sprintf("- %s:%d [%s]", ip, port, serviceInfo)
						if args.Mode != "deep" {
							break // Only report first port in discovery mode per IP
						}
					}
				}
			}
		}()
	}

	// 4. Feed jobs to workers
	go func() {
		n.resolveAndQueueTargets(ctx, args.Target, cfg, jobs)
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var output strings.Builder
	fmt.Fprintf(&output, "Network Scan (%s) of %s completed:\n", args.Mode, base)

	found := 0
	for r := range results {
		fmt.Fprintln(&output, r)
		found++
	}

	if found == 0 {
		output.WriteString("No active services identified.")
	}

	return output.String(), nil
}

// probeHTTP tries to grab the 'Server' header for HTTP ports
func (n *NetworkTools) probeHTTP(ctx context.Context, ip string, port int) string {
	// Use a short-lived context for probing
	probeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
	req, err := http.NewRequestWithContext(probeCtx, "GET", url, nil)
	if err != nil {
		return "Active Service (Probe Failed)"
	}

	resp, err := n.httpClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		server := resp.Header.Get("Server")
		if server != "" {
			return "HTTP Server: " + server
		}
		return "HTTP Service (No banner)"
	}
	return "Active Service (No banner)"
}

func (n *NetworkTools) getLocalSubnet() (string, *net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", nil, err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), ipnet, nil
			}
		}
	}
	return "", nil, fmt.Errorf("no valid local IPv4 address found")
}

// ValidateDomainBoundary checks if a host matches any blocked domains at label boundaries.
func ValidateDomainBoundary(host string, blockedDomains []string) error {
	hostLower := strings.ToLower(host)
	for _, blocked := range blockedDomains {
		blocked = strings.ToLower(blocked)
		if hostLower == blocked || strings.HasSuffix(hostLower, "."+blocked) {
			return fmt.Errorf("access to domain '%s' is blocked by guardrails", blocked)
		}
	}
	return nil
}

func ExtractHost(address string) string {
	u, err := url.Parse(address)
	if err == nil && u.Hostname() != "" {
		return u.Hostname()
	}

	// Fallback for raw hosts or malformed URLs
	host := address
	if strings.Contains(address, "://") {
		parts := strings.Split(address, "://")
		if len(parts) > 1 {
			host = strings.Split(parts[1], "/")[0]
		}
	}

	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			return h
		}
	}
	return host
}

// HTTPClient returns the underlying guarded http.Client.
func (n *NetworkTools) HTTPClient() *http.Client {
	return n.httpClient
}

// DialContext returns a guarded dial function compatible with http.Transport.DialContext.
func (n *NetworkTools) DialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return n.httpClient.Transport.(*http.Transport).DialContext
}

// ValidateScanTargets ensures all targets in a comma-separated list are valid IPs or CIDRs.
func ValidateScanTargets(targetStr string) error {
	targetParts := strings.SplitSeq(targetStr, ",")
	for rawTarget := range targetParts {
		target := strings.TrimSpace(rawTarget)
		if target == "" {
			continue
		}
		if strings.Contains(target, "/") {
			if _, _, err := net.ParseCIDR(target); err != nil {
				return fmt.Errorf("invalid CIDR '%s': %w", target, err)
			}
		} else {
			if ip := net.ParseIP(target); ip == nil {
				// Could be a domain name or hostname, validateAddress handles deeper validation
			}
		}
	}
	return nil
}
