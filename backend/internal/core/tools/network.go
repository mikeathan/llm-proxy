package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// Open makes a guarded GET and returns the response for the caller to stream and close: network tools on, internet
// access allowed, the host not blocked, every resolved address and redirect target vetted. Only a 200 answer is returned.
// FetchURL reads such a response as capped text; a caller that needs more (an operator-triggered download of a data file)
// streams the body itself and sets its own cap.
func (n *NetworkTools) Open(ctx context.Context, targetURL string) (*http.Response, error) {
	cfg := n.configProvider(ctx)
	if !cfg.Enabled {
		return nil, fmt.Errorf("network tools are disabled")
	}

	if !cfg.AllowInternetAccess && !cfg.AllowLanAccess {
		return nil, fmt.Errorf("network access is fully blocked by guardrails")
	}

	// Security Check: Blocked Domains
	host := ExtractHost(targetURL)
	if err := ValidateDomainBoundary(host, cfg.BlockedDomains); err != nil {
		return nil, err
	}

	// Initial pre-check (best effort, real check is in DialContext)
	if err := n.validateAddress(ctx, targetURL, cfg); err != nil {
		return nil, err
	}

	// Use the shared httpClient which has the security-locked DialContext and pooling
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", "LLM-Proxy-Agent/1.0")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		n.logger.Error("Network fetch failed", "url", targetURL, "error", err)
		return nil, fmt.Errorf("failed to fetch content from '%s': %s", targetURL, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned unexpected status: %s", resp.Status)
	}
	return resp, nil
}

func (n *NetworkTools) FetchURL(ctx context.Context, targetURL string) (string, error) {
	cfg := n.configProvider(ctx)
	resp, err := n.Open(ctx, targetURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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
	shown, full := fetchedText(resp.Header.Get("Content-Type"), data, lr.N <= 0, capChars)
	out := fitHeadTail(shown, capChars)
	n.reportCut(full, capChars, out)
	return out, nil
}

// reportCut logs a fetch whose page exceeded the cap, in the format of the agent's own "tool result cut" line: what the
// page held and what was kept, both JSON-encoded, once per fetch. The tool cuts its output before the agent's generic cut
// sees it, so that cut never counted this one.
func (n *NetworkTools) reportCut(full, capChars int, kept string) {
	if full > capChars {
		n.logger.Info("tool result cut", "tool", "fetch_url", "chars", full, "kept", encodedLen(kept))
	}
}

// guardrailCutNote follows a body the size guardrail stopped reading.
const guardrailCutNote = "\n... (content truncated by network guardrails)"

// fetchedText is what the model reads of a response: an HTML page as its readable text (the result cap would
// otherwise spend itself on the page's markup), anything else as it came. cutShort marks a body the size
// guardrail stopped reading.
func fetchedText(contentType string, data []byte, cutShort bool, capChars int) (shown string, full int) {
	text := string(data)
	note := ""
	if cutShort {
		note = guardrailCutNote
	}
	if isHTML(contentType, data) {
		text = htmlToText(text)
		full = encodedLen(text) + encodedLen(note)
		text = pageHead(text, encodedLen(note), capChars)
	} else {
		full = encodedLen(text) + encodedLen(note)
	}
	return text + note, full
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

func (n *NetworkTools) getLocalSubnet() (string, *net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Listing interfaces needs a netlink socket, which a service run under systemd's RestrictAddressFamilies may not
		// open. The kernel can still say which address the default route sends from (an UDP "connection" sends nothing).
		return outboundSubnet(err)
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

// outboundSubnet is the fallback when interfaces cannot be listed: the default route's local address on an assumed /24.
func outboundSubnet(listErr error) (string, *net.IPNet, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", nil, listErr
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return "", nil, listErr
	}
	ipnet := &net.IPNet{IP: addr.IP.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}
	return addr.IP.String(), ipnet, nil
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
