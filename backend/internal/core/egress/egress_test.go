package egress

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// startProxy serves on a random loopback port for the test and blocks until it
// accepts connections (readiness poll) so tests never race the accept loop.
func startProxy(t *testing.T, p Policy) (addr string, stop func()) {
	t.Helper()
	return startProxyWithToken(t, p, "")
}

// startProxyWithToken serves a token-protected proxy (empty token = open).
func startProxyWithToken(t testing.TB, p Policy, token string) (addr string, stop func()) {
	t.Helper()
	srv := New(p)
	srv.SetToken(token)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.ServeOnListener(ctx, ln) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("egress proxy did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return addr, cancel
}

// proxyURL builds the proxy URL for an http.Client transport.
func proxyURL(addr string) func(*http.Request) (*url.URL, error) {
	u := &url.URL{Scheme: "http", Host: addr}
	return func(*http.Request) (*url.URL, error) { return u, nil }
}

func TestPolicyMatch(t *testing.T) {
	cases := []struct {
		name   string
		policy HostListPolicy
		host   string
		want   bool
	}{
		{"deny beats allow", HostListPolicy{Allow: []string{".example.com"}, Deny: []string{"evil.example.com"}}, "evil.example.com", false},
		{"suffix allow", HostListPolicy{Allow: []string{".telegram.org"}}, "api.telegram.org", true},
		{"suffix does not match apex", HostListPolicy{DefaultDeny: true, Allow: []string{".telegram.org"}}, "telegram.org", false},
		{"exact allow", HostListPolicy{Allow: []string{"api.telegram.org"}}, "api.telegram.org", true},
		{"allow-all default", HostListPolicy{}, "anything.example", true},
		{"default-deny blocks unknown", HostListPolicy{DefaultDeny: true, Allow: []string{"a.example"}}, "b.example", false},
		{"default-deny allows listed", HostListPolicy{DefaultDeny: true, Allow: []string{"a.example"}}, "a.example", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.AllowHost(tc.host); got != tc.want {
				t.Errorf("AllowHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestServerRefusesNonLoopbackBind(t *testing.T) {
	srv := New(HostListPolicy{})
	if err := srv.Serve(context.Background(), "0.0.0.0:0"); err == nil {
		t.Fatal("expected error binding to a non-loopback address")
	}
	if err := srv.Serve(context.Background(), ":8080"); err == nil {
		t.Fatal("expected error for all-interfaces bind")
	}
}

func TestHTTPAbsoluteFormProxying(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello via proxy"))
	}))
	defer upstream.Close()

	upURL, _ := url.Parse(upstream.URL)
	// Default-deny policy that allows exactly the upstream host.
	policy := HostListPolicy{DefaultDeny: true, Allow: []string{upURL.Hostname()}}
	addr, stop := startProxy(t, policy)
	defer stop()

	client := &http.Client{Transport: &http.Transport{Proxy: proxyURL(addr)}, Timeout: 5 * time.Second}
	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("proxied GET failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello via proxy" {
		t.Errorf("unexpected proxied response: %d %q", resp.StatusCode, body)
	}
}

func TestHTTPDeniedByPolicy(t *testing.T) {
	hit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer upstream.Close()

	addr, stop := startProxy(t, HostListPolicy{DefaultDeny: true}) // deny everything
	defer stop()

	client := &http.Client{Transport: &http.Transport{Proxy: proxyURL(addr)}, Timeout: 5 * time.Second}
	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("GET through deny-all proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	if hit {
		t.Error("denied request must never reach the upstream")
	}
}

func TestConnectTunnel(t *testing.T) {
	// A raw TCP "echo" upstream stands in for a TLS server (CONNECT is a
	// passthrough tunnel, so plain TCP is enough to prove byte transport).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _, _ = io.Copy(c, c) }(c) // echo
		}
	}()

	upHost, upPort, _ := net.SplitHostPort(ln.Addr().String())
	policy := HostListPolicy{DefaultDeny: true, Allow: []string{upHost}}
	addr, stop := startProxy(t, policy)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, _ = conn.Write([]byte("CONNECT " + upHost + ":" + upPort + " HTTP/1.1\r\nHost: " + upHost + ":" + upPort + "\r\n\r\n"))
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "200") {
		t.Fatalf("CONNECT not established: %q", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading CONNECT headers: %v", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	msg := "ping-through-tunnel"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	echoed := make([]byte, len(msg))
	if _, err := io.ReadFull(br, echoed); err != nil {
		t.Fatalf("tunnel echo failed: %v", err)
	}
	if string(echoed) != msg {
		t.Errorf("echo mismatch: %q", echoed)
	}
}

func TestConnectDeniedByPolicy(t *testing.T) {
	addr, stop := startProxy(t, HostListPolicy{DefaultDeny: true})
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("CONNECT blocked.example.com:443 HTTP/1.1\r\nHost: blocked.example.com:443\r\n\r\n"))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "403") {
		t.Errorf("expected 403 for denied CONNECT, got %q", status)
	}
}

// TestConnectTunnelByHostname proves CONNECT authorities are policy-checked on
// the HOSTNAME and that DNS resolution happens inside the proxy (the dialer
// resolves "localhost" to loopback here): the policy sees the hostname, not a
// raw IP, and nothing is forwarded for a denied hostname.
func TestConnectTunnelByHostname(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _, _ = io.Copy(c, c) }(c) // echo
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	addr, stop := startProxy(t, HostListPolicy{DefaultDeny: true, Allow: []string{"localhost"}})
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// "localhost" resolves inside the proxy (127.0.0.1) but must be allowed by
	// the hostname policy before any dial.
	_, _ = conn.Write([]byte("CONNECT localhost:" + port + " HTTP/1.1\r\nHost: localhost:" + port + "\r\n\r\n"))
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "200") {
		t.Fatalf("CONNECT by hostname not established: %q", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading CONNECT headers: %v", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	msg := "hostname-tunnel"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	echoed := make([]byte, len(msg))
	if _, err := io.ReadFull(br, echoed); err != nil {
		t.Fatalf("tunnel echo failed: %v", err)
	}
	if string(echoed) != msg {
		t.Errorf("echo mismatch: %q", echoed)
	}
}

// Token hardening (plan §9 T5): a caller without the shared secret is refused,
// and the agent's credentialed proxy URL works end-to-end.
func TestProxyTokenAuth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("authorized"))
	}))
	defer upstream.Close()
	upURL, _ := url.Parse(upstream.URL)

	policy := HostListPolicy{DefaultDeny: true, Allow: []string{upURL.Hostname()}}
	addr, stop := startProxyWithToken(t, policy, "s3cr3t-token")
	defer stop()

	// Without credentials: 407, never forwarded.
	anon := &http.Client{Transport: &http.Transport{Proxy: proxyAddrFunc(addr)}, Timeout: 5 * time.Second}
	resp, err := anon.Get(upstream.URL)
	if err != nil {
		t.Fatalf("anonymous proxied GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("anonymous request status = %d, want 407", resp.StatusCode)
	}

	// With the shared secret (as the agent's transport is configured): allowed.
	authURL, _ := url.Parse("http://agent:s3cr3t-token@" + addr)
	authorized := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(authURL)}, Timeout: 5 * time.Second}
	resp, err = authorized.Get(upstream.URL)
	if err != nil {
		t.Fatalf("authenticated proxied GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "authorized" {
		t.Fatalf("authenticated request = %d %q, want 200 authorized", resp.StatusCode, body)
	}
}

// proxyAddrFunc returns a Proxy callback pointing at addr (no credentials).
func proxyAddrFunc(addr string) func(*http.Request) (*url.URL, error) {
	u := &url.URL{Scheme: "http", Host: addr}
	return func(*http.Request) (*url.URL, error) { return u, nil }
}

// Hop-by-hop headers and the proxy's own credentials must never cross the
// proxy (RFC 7230 §6.1): the origin must not receive Proxy-Authorization (this
// proxy's shared secret) or Connection-listed fields, and the client must not
// receive the origin's hop-by-hop headers. End-to-end headers survive.
func TestHTTPStripsHopByHopAndProxyAuth(t *testing.T) {
	var got http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("X-Origin", "yes")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	upURL, _ := url.Parse(upstream.URL)

	addr, stop := startProxyWithToken(t, HostListPolicy{DefaultDeny: true, Allow: []string{upURL.Hostname()}}, "s3cr3t-token")
	defer stop()

	authURL, _ := url.Parse("http://agent:s3cr3t-token@" + addr)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(authURL)}, Timeout: 5 * time.Second}
	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("proxied GET: %v", err)
	}
	defer resp.Body.Close()

	if v := got.Get("Proxy-Authorization"); v != "" {
		t.Errorf("proxy secret leaked upstream: Proxy-Authorization=%q", v)
	}
	if v := got.Get("Connection"); v != "" {
		t.Errorf("hop-by-hop Connection forwarded upstream: %q", v)
	}
	if v := resp.Header.Get("Keep-Alive"); v != "" {
		t.Errorf("hop-by-hop Keep-Alive echoed to client: %q", v)
	}
	if v := resp.Header.Get("X-Origin"); v != "yes" {
		t.Errorf("end-to-end header must survive: X-Origin=%q", v)
	}
}

// A raw request proves headers named by Connection (not just the standard set)
// are stripped on the request leg.
func TestHTTPStripsConnectionNamedHeaders(t *testing.T) {
	var got http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	upURL, _ := url.Parse(upstream.URL)

	addr, stop := startProxy(t, HostListPolicy{DefaultDeny: true, Allow: []string{upURL.Hostname()}})
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET " + upstream.URL + " HTTP/1.1\r\n" +
		"Host: " + upURL.Host + "\r\n" +
		"Connection: close, X-Hop\r\n" +
		"X-Hop: should-be-stripped\r\n\r\n"))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := http.ReadResponse(bufio.NewReader(conn), nil); err != nil {
		t.Fatalf("read proxied response: %v", err)
	}

	if v := got.Get("Connection"); v != "" {
		t.Errorf("Connection forwarded upstream: %q", v)
	}
	if v := got.Get("X-Hop"); "" != v {
		t.Errorf("Connection-named X-Hop forwarded upstream: %q", v)
	}
}

// Baseline for the localhost proxy hop (plan §9 T7): steady-state absolute-form
// GET through the proxy to a local upstream. Compare against a direct client to
// quantify the per-request overhead on the target host.
func BenchmarkProxyHTTPRoundTrip(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upURL, _ := url.Parse(upstream.URL)

	addr, stop := startProxyWithToken(b, HostListPolicy{DefaultDeny: true, Allow: []string{upURL.Hostname()}}, "")
	defer stop()

	proxy, _ := url.Parse("http://" + addr)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
	defer client.CloseIdleConnections()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(upstream.URL)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// Baseline for the policy decision on the hot path (every proxied request).
func BenchmarkPolicyAllowHost(b *testing.B) {
	p := HostListPolicy{DefaultDeny: true, Allow: []string{"api.telegram.org", ".example.com"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !p.AllowHost("api.telegram.org") {
			b.Fatal("expected allowed")
		}
	}
}
