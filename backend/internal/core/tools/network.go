package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// NetworkTools provides native capability-based network operations.
// These tools run natively on the host rather than in the sandbox to provide
// safe, governed access to external and local resources.
type NetworkTools struct {
	configProvider func(ctx context.Context) models.NetworkGuardrailsConfig
}

func NewNetworkTools(provider func(ctx context.Context) models.NetworkGuardrailsConfig) *NetworkTools {
	return &NetworkTools{
		configProvider: provider,
	}
}

// FetchURL downloads the content of a remote URL.
func (n *NetworkTools) FetchURL(ctx context.Context, url string) (string, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled {
		return "", fmt.Errorf("network tools are disabled")
	}

	if !cfg.AllowInternetAccess && !cfg.AllowLanAccess {
		return "", fmt.Errorf("network access is fully blocked by guardrails")
	}

	// Security Check: Blocked Domains
	for _, domain := range cfg.BlockedDomains {
		if strings.Contains(url, domain) {
			return "", fmt.Errorf("access to domain '%s' is blocked by guardrails", domain)
		}
	}

	client := &http.Client{
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return n.validateAddress(req.URL.Host, cfg)
		},
	}

	if err := n.validateAddress(url, cfg); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "LLM-Proxy-Agent/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned error: %s", resp.Status)
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

// validateAddress ensures the target host complies with LAN/Internet access rules.
func (n *NetworkTools) validateAddress(address string, cfg models.NetworkGuardrailsConfig) error {
	host := address
	if strings.Contains(address, "://") {
		// Extract host from URL
		parts := strings.Split(address, "://")
		if len(parts) > 1 {
			host = strings.Split(parts[1], "/")[0]
		}
	}
	
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS fails, we might be dealing with a raw IP or a blocked segment
		return nil // We'll let the dialer handle the error
	}

	for _, ip := range ips {
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
	var start, end int
	
	if args.Target != "" {
		if strings.Contains(args.Target, "/") {
			// Subnet
			_, ipnet, err := net.ParseCIDR(args.Target)
			if err != nil {
				return "", fmt.Errorf("invalid CIDR: %w", err)
			}
			prefix := ipnet.IP.Mask(ipnet.Mask).String()
			base = prefix[:strings.LastIndex(prefix, ".")+1]
			start, end = 1, 255
		} else {
			// Single IP
			base = args.Target
			start, end = 0, 1 // Flag to only scan the base
		}
	} else {
		// Auto-detect local
		_, subnet, err := n.getLocalSubnet()
		if err != nil {
			return "", fmt.Errorf("failed to detect local subnet: %w", err)
		}
		prefix := subnet.IP.Mask(subnet.Mask).String()
		base = prefix[:strings.LastIndex(prefix, ".")+1]
		start, end = 1, 255
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
	var wg sync.WaitGroup

	// 3. Scan Loop
	for i := start; i < end; i++ {
		targetIP := base
		if end > 1 {
			targetIP = fmt.Sprintf("%s%d", base, i)
		}

		// Security boundary check for target
		if err := n.validateAddress(targetIP, cfg); err != nil {
			continue // Skip blocked IPs
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			for _, port := range ports {
				addr := fmt.Sprintf("%s:%d", ip, port)
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
						serviceInfo = n.probeHTTP(ip, port)
					}
					
					conn.Close()
					results <- fmt.Sprintf("- %s:%d [%s]", ip, port, serviceInfo)
					if end > 1 && args.Mode != "deep" {
						return // Only report first port in discovery mode per IP
					}
				}
			}
		}(targetIP)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var output strings.Builder
	scanContext := base
	if end > 1 {
		scanContext += "0/24"
	}
	output.WriteString(fmt.Sprintf("Network Scan (%s) of %s completed:\n", args.Mode, scanContext))
	
	found := 0
	for r := range results {
		output.WriteString(r + "\n")
		found++
	}

	if found == 0 {
		output.WriteString("No active services identified.")
	}

	return output.String(), nil
}

// probeHTTP tries to grab the 'Server' header for HTTP ports
func (n *NetworkTools) probeHTTP(ip string, port int) string {
	client := &http.Client{Timeout: 1 * time.Second}
	url := fmt.Sprintf("http://%s:%d", ip, port)
	resp, err := client.Get(url)
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
