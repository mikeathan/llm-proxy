// Package egress implements the agent egress proxy (agent-os-sandboxing plan
// D2 / §4.5): a loopback-only HTTP forward proxy that applies host-level policy
// to agent outbound traffic. It is the only mechanism that gives domain-level
// egress filtering — OS sandboxes can only do network on/off.
//
// Design notes (plan §9 T5):
//   - Listener is loopback-only (127.0.0.1 / ::1); the constructor refuses any
//     other bind address, so no other host can reach it.
//   - Policy is evaluated on the CONNECT authority / absolute-URI host BEFORE
//     any byte is forwarded (pre-TLS, no MITM — TLS passthrough for CONNECT).
//   - DNS resolution happens inside this proxy process (the backend dials),
//     so an HTTP(S)-proxied client cannot exfiltrate via its own UDP/53.
//   - HTTP absolute-form requests and CONNECT tunnels are both supported.
//   - It is infrastructure egress (in-process tools point their transport at
//     it; shells ride it via HTTP(S)_PROXY env) and is NEVER on the agent tool
//     guardrail path itself.
//
// Wiring to in-process tools (registry SetProxy), shell env (HTTP(S)_PROXY),
// and the sandboxing.egress_proxy config field lives in the app composition
// root (bootstrap startEgressProxy → InitializeAgentStack); this package is
// the policy + server core with its own integration tests.
package egress

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Policy decides which outbound destinations agent egress may reach. Evaluated
// on the destination hostname (no port) before any forwarding.
type Policy interface {
	AllowHost(host string) bool
}

// HostListPolicy allows and denies by exact hostname or a suffix entry
// (".example.com" matches any subdomain). Deny wins over allow. An empty allow
// list denies nothing (allow-all) unless DefaultDeny is set; when DefaultDeny
// is true, only explicitly allowed entries are reachable.
type HostListPolicy struct {
	Allow       []string
	Deny        []string
	DefaultDeny bool
}

// AllowHost implements Policy.
func (p HostListPolicy) AllowHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, d := range p.Deny {
		if matchHost(host, d) {
			return false
		}
	}
	if matchAny(host, p.Allow) {
		return true
	}
	return !p.DefaultDeny
}

func matchAny(host string, list []string) bool {
	for _, h := range list {
		if matchHost(host, h) {
			return true
		}
	}
	return false
}

// matchHost matches an exact hostname or a ".suffix" entry.
func matchHost(host, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, ".") {
		return strings.HasSuffix(host, pattern) && host != strings.TrimPrefix(pattern, ".")
	}
	return host == pattern
}

// Server is a loopback-only HTTP forward proxy with host policy and an
// optional shared-secret token. When a token is set, every request (including
// CONNECT) must carry it as Proxy-Authorization Basic credentials, so other
// local processes cannot ride the proxy (plan §9 T5 local-abuse hardening).
type Server struct {
	policy Policy
	token  string
	srv    *http.Server
	tr     *http.Transport
}

// New builds a proxy server with the given policy. Nothing starts until
// Serve is called.
func New(policy Policy) *Server {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	// One pooled transport for all absolute-form requests: Proxy nil (this
	// proxy resolves and dials — DNS at proxy), idle keep-alive per host.
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		MaxIdleConnsPerHost: 8,
		MaxIdleConns:        64,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &Server{policy: policy, tr: tr}
}

// SetToken requires callers to present this shared secret via
// Proxy-Authorization (Basic). Empty disables authentication. Must be called
// before Serve.
func (s *Server) SetToken(token string) { s.token = token }

// authorized reports whether r may use the proxy. No token configured ⇒ open.
// Credentials arrive in Proxy-Authorization (not Authorization, which
// http.Request.BasicAuth reads), so the header is parsed here; the comparison
// is constant-time.
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	const prefix = "Basic "
	hv := r.Header.Get("Proxy-Authorization")
	if len(hv) <= len(prefix) || !strings.EqualFold(hv[:len(prefix)], prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(hv[len(prefix):]))
	if err != nil {
		return false
	}
	_, pass, found := strings.Cut(string(raw), ":")
	if !found {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(pass), []byte(s.token)) == 1
}

// Serve binds addr and serves until the context is cancelled or Close is
// called. addr's host must be a loopback address (127.0.0.1 or ::1) — see §9
// T5 hardening.
func (s *Server) Serve(ctx context.Context, addr string) error {
	if err := requireLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("egress proxy listen %s: %w", addr, err)
	}
	return s.ServeOnListener(ctx, ln)
}

// ServeOnListener serves on an already-bound listener (used by callers that
// need to own the socket, e.g. tests).
func (s *Server) ServeOnListener(ctx context.Context, ln net.Listener) error {
	// ReadHeaderTimeout bounds a stalled client (the listener is loopback-only,
	// but agent processes are untrusted); no WriteTimeout — CONNECT tunnels are
	// long-lived. IdleTimeout reaps abandoned keep-alive connections.
	s.srv = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	done := make(chan error, 1)
	go func() {
		err := s.srv.Serve(ln)
		if err == http.ErrServerClosed {
			err = nil
		}
		done <- err
	}()
	select {
	case <-ctx.Done():
		_ = s.srv.Close()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Allow ":port" (defaults to all interfaces)? No — refuse: the proxy
		// must never listen outside loopback.
		return fmt.Errorf("egress proxy addr must be loopback (host:port), got %q", addr)
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("egress proxy addr must be loopback (127.0.0.1/::1), got %q", addr)
}

// serveHTTP handles absolute-form HTTP and CONNECT. Returns 403 for
// policy-denied destinations; nothing is forwarded for a denial.
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="llm-proxy-egress"`)
		http.Error(w, "egress proxy: proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	s.handleHTTP(w, r)
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() {
		http.Error(w, "egress proxy: absolute-form request URI required", http.StatusBadRequest)
		return
	}
	host := hostnameOf(r.URL.Host)
	if !s.policy.AllowHost(host) {
		http.Error(w, "egress proxy: destination denied by policy", http.StatusForbidden)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	// The proxy owns its upstream connection lifecycle: the client's Close
	// flag (set from its Connection header) must not be re-emitted upstream.
	outReq.Close = false
	// Strip hop-by-hop headers (RFC 7230 §6.1), including Proxy-Authorization:
	// it carries THIS proxy's shared secret and must never reach the origin.
	removeHopByHopHeaders(outReq.Header)
	resp, err := s.tr.RoundTrip(outReq)
	if err != nil {
		http.Error(w, "egress proxy: upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	// Do not echo the origin's hop-by-hop headers to the client either.
	respHeader := resp.Header.Clone()
	removeHopByHopHeaders(respHeader)
	h := w.Header()
	for k, vv := range respHeader {
		for _, v := range vv {
			h.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// hopByHopHeaders are the per-connection headers RFC 7230 §6.1 forbids a proxy
// from forwarding (plus Proxy-Connection, a long-standing non-standard alias).
var hopByHopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive",
	"Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

// removeHopByHopHeaders deletes the hop-by-hop headers, including any header
// named by the Connection header, so they cross neither the request nor the
// response leg of the proxy.
func removeHopByHopHeaders(h http.Header) {
	for _, conn := range h.Values("Connection") {
		for _, field := range strings.Split(conn, ",") {
			if field = strings.TrimSpace(field); field != "" {
				h.Del(field)
			}
		}
	}
	for _, k := range hopByHopHeaders {
		h.Del(k)
	}
}

// handleConnect tunnels a raw TCP connection to the CONNECT authority after
// policy approval (TLS passthrough — no MITM).
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := hostnameOf(r.Host)
	if !s.policy.AllowHost(host) {
		http.Error(w, "egress proxy: destination denied by policy", http.StatusForbidden)
		return
	}
	dest := r.Host
	if _, _, err := net.SplitHostPort(r.Host); err != nil {
		// CONNECT without an explicit port — default to 443.
		dest = net.JoinHostPort(r.Host, "443")
	}
	upstream, err := s.tr.DialContext(r.Context(), "tcp", dest)
	if err != nil {
		http.Error(w, "egress proxy: dial failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "egress proxy: hijacking unsupported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		http.Error(w, "egress proxy: hijack failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	copyBoth(conn, upstream)
}

// copyBoth shuttles bytes in both directions until either side closes, then
// closes both connections.
func copyBoth(a, b net.Conn) {
	errc := make(chan error, 2)
	go func() { _, e1 := io.Copy(b, a); errc <- e1 }()
	go func() { _, e2 := io.Copy(a, b); errc <- e2 }()
	<-errc
	_ = a.Close()
	_ = b.Close()
	<-errc
}

func hostnameOf(authority string) string {
	h := authority
	if u, err := url.Parse("//" + authority); err == nil && u.Hostname() != "" {
		h = u.Hostname()
	} else if host, _, err := net.SplitHostPort(authority); err == nil {
		h = host
	}
	return strings.TrimSuffix(h, ".")
}
