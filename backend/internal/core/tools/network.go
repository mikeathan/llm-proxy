package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core/proxy"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/units"
	"llm-proxy/models"
)

// NetworkTools provides native capability-based network operations.
type NetworkTools struct {
	configProvider func(ctx context.Context) models.NetworkGuardrailsConfig
	httpClient     *http.Client
	logger         logging.Logger
	// proxyAddr is the configured egress-proxy "host:port" ("" = direct). The
	// guarded DialContext must allow dialing THIS address (loopback) even
	// though loopback is otherwise blocked for agents — the policy applies to
	// the proxied target, not to the proxy itself. Set once at construction
	// (composition root) before the client serves any request.
	proxyAddr string
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
			// Dialing the configured egress proxy itself is permitted by
			// construction: it is the single loopback exception. For proxied
			// requests the full LAN/internet policy runs in validateAddress (the
			// original URL and every redirect target), and the proxy applies
			// network.CheckAlwaysBlocked to the IP it actually dials. Residual: the
			// proxy resolves the name again without the LAN policy, so a DNS answer
			// that changes to a LAN address between the two lookups is not caught.
			// Requests that do not ride the proxy (no SetProxy) go through the full
			// validation here, on the dialed IP.
			if addr == n.proxyAddr {
				return dialer.DialContext(ctx, network, addr)
			}

			cfg := n.configProvider(ctx)

			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address format: %w", err)
			}

			// Use pure-Go resolver (PreferGo) scoped to network tool; respects
			// context cancellation where the default cgo resolver does not.
			ips, err := (&net.Resolver{PreferGo: true}).LookupIP(ctx, "ip", host)
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
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// Each redirect target gets the same pre-check as the original URL.
			// Without a proxy the dial guard would also catch it, but with the
			// egress proxy on, the transport only dials the proxy, so this is the
			// only place the LAN/internet policy sees a redirect.
			return n.validateAddress(req.Context(), req.URL.String(), n.configProvider(req.Context()))
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
		return "", fmt.Errorf("failed to fetch content from '%s': %s", targetURL, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned unexpected status: %s", resp.Status)
	}

	limit := units.KiB(cfg.MaxFetchSizeKB)
	if limit <= 0 {
		limit = units.MiB(1) // 1 MiB default
	}

	lr := &io.LimitedReader{R: resp.Body, N: limit}
	data, err := io.ReadAll(lr)
	if err != nil && err != io.EOF {
		return "", err
	}

	capChars := resultCapOf(ctx)
	return proxy.TruncateResult(fetchedText(resp.Header.Get("Content-Type"), data, lr.N <= 0, capChars), capChars, ""), nil
}

// guardrailCutNote follows a body the size guardrail stopped reading.
const guardrailCutNote = "\n... (content truncated by network guardrails)"

// fetchedText is what the model reads of a response: an HTML page as its readable text (the result cap would
// otherwise spend itself on the page's markup), anything else as it came. cutShort marks a body the size
// guardrail stopped reading.
func fetchedText(contentType string, data []byte, cutShort bool, capChars int) string {
	text := string(data)
	note := ""
	if cutShort {
		note = guardrailCutNote
	}
	if isHTML(contentType, data) {
		text = pageHead(htmlToText(text), encodedLen(note), capChars)
	}
	return text + note
}

// resolveAndQueueTargets handles the expansion and validation of comma-separated targets.
func (n *NetworkTools) resolveAndQueueTargets(ctx context.Context, targetStr string, cfg models.NetworkGuardrailsConfig, jobs chan<- string) {
	for _, rawTarget := range strings.Split(targetStr, ",") {
		target := strings.TrimSpace(rawTarget)
		if target == "" {
			continue
		}
		queue := n.queueHost
		if strings.Contains(target, "/") {
			queue = n.queueSubnet
		}
		if !queue(ctx, target, cfg, jobs) {
			return
		}
	}
}

// maxScanSubnetIPs caps how many addresses of one subnet are queued, so a /16 cannot hang a scan.
const maxScanSubnetIPs = 1024

// queueSubnet queues the allowed addresses of a CIDR range, without its network address. It returns false once ctx has
// ended, which stops the whole scan; an unparsable range queues nothing and the scan goes on.
func (n *NetworkTools) queueSubnet(ctx context.Context, target string, cfg models.NetworkGuardrailsConfig, jobs chan<- string) bool {
	ip, ipnet, err := net.ParseCIDR(target)
	if err != nil {
		return true
	}
	count := 0
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		if count > maxScanSubnetIPs {
			n.logger.Warn("Network scan capped to prevent hang", "target", target, "limit", maxScanSubnetIPs)
			break
		}
		// Skip the network address (heuristic for /24-like subnets). For larger subnets everything else is scanned.
		if ip.Equal(ipnet.IP) {
			continue
		}
		count++
		if n.validateIP(ip, cfg) != nil {
			continue
		}
		if !sendScanJob(ctx, jobs, ip.String()) {
			return false
		}
	}
	return true
}

// queueHost queues one IP or host name if the guardrails allow it. It returns false once ctx has ended.
func (n *NetworkTools) queueHost(ctx context.Context, target string, cfg models.NetworkGuardrailsConfig, jobs chan<- string) bool {
	if ip := net.ParseIP(target); ip != nil {
		if n.validateIP(ip, cfg) != nil {
			return true
		}
	} else if n.validateAddress(ctx, target, cfg) != nil {
		return true
	}
	return sendScanJob(ctx, jobs, target)
}

// sendScanJob hands one address to the workers; false means ctx ended first.
func sendScanJob(ctx context.Context, jobs chan<- string, job string) bool {
	select {
	case jobs <- job:
		return true
	case <-ctx.Done():
		return false
	}
}

// validateAddress ensures the target host complies with LAN/Internet access rules.
func (n *NetworkTools) validateAddress(ctx context.Context, address string, cfg models.NetworkGuardrailsConfig) error {
	host := ExtractHost(address)

	// Domain Check at label boundaries
	if err := ValidateDomainBoundary(host, cfg.BlockedDomains); err != nil {
		return err
	}

	// SEC-H2: DNS pre-check with pure-Go resolver for context-aware cancellation.
	ips, err := (&net.Resolver{PreferGo: true}).LookupIP(ctx, "ip", host)
	if err != nil {
		// Fail closed, like the dial guard: an address that cannot be classified
		// is not reachable.
		return fmt.Errorf("cannot resolve '%s' to validate it: %w", host, err)
	}

	for _, ip := range ips {
		if err := n.validateIP(ip, cfg); err != nil {
			return err
		}
	}

	return nil
}

func (n *NetworkTools) validateIP(ip net.IP, cfg models.NetworkGuardrailsConfig) error {
	// SAFETY: unspecified, loopback, link-local and multicast addresses are
	// ALWAYS blocked for agents to prevent poking at host-only services (0.0.0.0
	// routes to localhost) or cloud metadata APIs. One shared classifier.
	if err := network.CheckAlwaysBlocked(ip); err != nil {
		return err
	}

	isPrivate := network.IsLAN(ip)

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

// maxRedirects bounds how many redirects a fetch follows.
const maxRedirects = 5

type PortList []int

func (p *PortList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Try standard array of integers first
	var ints []int
	if err := json.Unmarshal(data, &ints); err == nil {
		*p = ints
		return nil
	}

	// Fallback: Try a comma-separated string (common output from some LLMs)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		for _, part := range strings.Split(s, ",") {
			port, err := strconv.Atoi(strings.TrimSpace(part))
			if err == nil {
				*p = append(*p, port)
			}
		}
		return nil
	}

	return fmt.Errorf("invalid ports format: expected array of integers or comma-separated string")
}

type ScanArgs struct {
	Targets   string   `json:"targets,omitempty"`    // Plural (preferred for batching)
	Target    string   `json:"target,omitempty"`     // Singular (legacy)
	IP        string   `json:"ip,omitempty"`         // Hallucination alias
	CIDR      string   `json:"cidr,omitempty"`       // Hallucination alias
	Mode      string   `json:"mode,omitempty"`       // "fast" or "deep"
	Ports     PortList `json:"ports,omitempty"`      // Custom port list (handles []int or string)
	TimeoutMs int      `json:"timeout_ms,omitempty"` // Per-IP timeout
	Timeout   int      `json:"timeout,omitempty"`    // Alias for some models
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

// scanPlan is what a scan does with each address: which ports, how long to wait, and whether to report every open port.
type scanPlan struct {
	ports   []int
	timeout time.Duration
	dialer  *net.Dialer
	deep    bool
}

// ScanLocalNetwork performs a concurrent discovery or audit scan.
func (n *NetworkTools) ScanLocalNetwork(ctx context.Context, args ScanArgs) (string, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled || !cfg.AllowLanAccess {
		return "", fmt.Errorf("network scanning is disabled or LAN access is blocked")
	}
	base, err := n.scanRange(args)
	if err != nil {
		return "", err
	}
	timeout := scanTimeout(args)
	plan := scanPlan{ports: scanPorts(args), timeout: timeout, dialer: &net.Dialer{Timeout: timeout}, deep: args.Mode == "deep"}

	results := make(chan string, 1000)
	jobs := make(chan string, 256) // Sized for a /24 subnet

	// Worker pool for concurrent scanning. 50 workers handle larger scans efficiently while keeping the goroutines bounded (NET-L1).
	const numWorkers = 50
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.scanWorker(ctx, plan, jobs, results)
		}()
	}

	// Feed jobs to the workers, then close up once they are done.
	go func() {
		n.resolveAndQueueTargets(ctx, base, cfg, jobs)
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

// scanRange is the range to scan: the target the model named (under any of its aliases, validated), else the local subnet.
func (n *NetworkTools) scanRange(args ScanArgs) (string, error) {
	if target := scanTargetOf(args); target != "" {
		if err := ValidateScanTargets(target); err != nil {
			return "", err
		}
		return target, nil
	}
	_, subnet, err := n.getLocalSubnet()
	if err != nil {
		return "", fmt.Errorf("failed to detect local subnet: %w", err)
	}
	return subnet.String(), nil
}

// scanTargetOf is the first of the target field and its aliases (singular, "ip", "cidr") that the model filled in.
func scanTargetOf(args ScanArgs) string {
	for _, t := range []string{args.Targets, args.Target, args.IP, args.CIDR} {
		if t != "" {
			return t
		}
	}
	return ""
}

// scanPorts is the ports the model named, else 20 common ones in deep mode and 5 discovery ports otherwise.
func scanPorts(args ScanArgs) []int {
	if len(args.Ports) > 0 {
		return args.Ports
	}
	if args.Mode == "deep" {
		return []int{21, 22, 23, 25, 53, 80, 110, 111, 135, 139, 143, 443, 445, 993, 995, 1723, 3306, 3389, 5900, 8080}
	}
	return []int{22, 80, 443, 8080, 3000}
}

// scanTimeout is the per-address wait: timeout_ms, else its alias "timeout", else half a second.
func scanTimeout(args ScanArgs) time.Duration {
	switch {
	case args.TimeoutMs > 0:
		return time.Duration(args.TimeoutMs) * time.Millisecond
	case args.Timeout > 0:
		return time.Duration(args.Timeout) * time.Millisecond
	}
	return 500 * time.Millisecond
}

// scanWorker probes the ports of each address it is given until jobs closes or ctx ends. Outside deep mode it reports
// only the first open port per address.
func (n *NetworkTools) scanWorker(ctx context.Context, plan scanPlan, jobs <-chan string, results chan<- string) {
	for ip := range jobs {
		for _, port := range plan.ports {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if info, open := n.probePort(ctx, plan, ip, port); open {
				results <- fmt.Sprintf("- %s:%d [%s]", ip, port, info)
				if !plan.deep {
					break
				}
			}
		}
	}
}

// probePort connects to one port and describes the service: its banner if it sends one, else the HTTP Server header.
func (n *NetworkTools) probePort(ctx context.Context, plan scanPlan, ip string, port int) (string, bool) {
	conn, err := plan.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
	if err != nil {
		return "", false
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(plan.timeout))
	banner := make([]byte, 256)
	if bytesRead, _ := conn.Read(banner); bytesRead > 0 {
		return serviceBanner(banner[:bytesRead]), true
	}
	return n.probeHTTP(ctx, ip, port), true
}

// serviceBanner is a service's greeting on one line, at most 64 characters.
func serviceBanner(raw []byte) string {
	info := strings.ReplaceAll(strings.TrimSpace(string(raw)), "\n", " ")
	if len(info) > 64 {
		info = info[:61] + "..."
	}
	return info
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

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
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

// SetProxy routes this client's outbound traffic through an HTTP forward proxy
// (egress proxy, sandboxing plan D2). The guarded DialContext remains the
// transport dialer — it now dials the loopback proxy (the single permitted
// loopback exception, see the DialContext above), which resolves and forwards
// per its policy. Agent tool-level guardrails (blocked domains, allow flags,
// scope) still run before the request is sent. Must be called before the
// client serves requests; setting a nil proxyURL is a no-op.
func (n *NetworkTools) SetProxy(proxyURL *url.URL) {
	if proxyURL == nil {
		return
	}
	tr, ok := n.httpClient.Transport.(*http.Transport)
	if !ok {
		return
	}
	n.proxyAddr = proxyURL.Host
	tr.Proxy = http.ProxyURL(proxyURL)
}

// DialContext returns a guarded dial function compatible with http.Transport.DialContext.
func (n *NetworkTools) DialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return n.httpClient.Transport.(*http.Transport).DialContext
}

// ValidateScanTargets ensures all targets in a comma-separated list are valid IPs or CIDRs.
func ValidateScanTargets(targetStr string) error {
	for _, rawTarget := range strings.Split(targetStr, ",") {
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
