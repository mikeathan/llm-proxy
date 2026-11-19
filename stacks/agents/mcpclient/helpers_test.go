package mcpclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"llm-proxy-stacks/sdk"
)

// fastTiming makes every wait short so a test that waits for a reconnect does not take seconds.
func fastTiming() Timing {
	return Timing{
		Connect: 2 * time.Second, ListPage: 2 * time.Second, Call: 2 * time.Second, Ping: time.Second,
		PingEvery: 50 * time.Millisecond, Refresh: 80 * time.Millisecond,
		BackoffMin: 10 * time.Millisecond, BackoffMax: 40 * time.Millisecond,
	}
}

// testHTTP is the stack's HTTP port in a test: a plain client, trusting one extra certificate when asked.
type testHTTP struct{ trust *x509.CertPool }

func (h testHTTP) Client(opts stacksdk.HTTPOptions) (*http.Client, error) {
	pool := h.trust
	if len(opts.RootCAPEM) > 0 {
		pool = x509.NewCertPool()
		if !pool.AppendCertsFromPEM(opts.RootCAPEM) {
			return nil, stacksdk.Errorf(stacksdk.CodeInvalid, "bad certificate")
		}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}, nil
}

// noSecrets opens a sealed value as itself with a prefix removed, so a test can see what reached the server.
func openPlain(_ context.Context, sealed string) (string, error) {
	return strings.TrimPrefix(sealed, "sealed:"), nil
}

// fixture is a running in-process MCP server with a few tools.
type fixture struct {
	mcp   *server.MCPServer
	http  *httptest.Server
	URL   string
	mu    sync.Mutex
	calls []string
	auths []string
	seen  map[string]bool // sessions the server issued
	dead  map[string]bool // sessions it now answers with 404, as after a restart
}

// KillSessions makes the server forget every session it has issued so far.
func (f *fixture) KillSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id := range f.seen {
		f.dead[id] = true
	}
}

// sessionGone answers 404 for a dead session, as the specification says a server does for one it no longer knows.
func (f *fixture) sessionGone(r *http.Request) bool {
	id := r.Header.Get("Mcp-Session-Id")
	if id == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dead[id] {
		return true
	}
	f.seen[id] = true
	return false
}

func (f *fixture) record(r *http.Request) {
	f.mu.Lock()
	f.auths = append(f.auths, r.Header.Get("Authorization")+"|"+r.Header.Get("X-Api-Key"))
	f.mu.Unlock()
}

func (f *fixture) Auths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auths...)
}

func (f *fixture) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func newMCPServer(f *fixture, opts ...server.ServerOption) *server.MCPServer {
	opts = append([]server.ServerOption{server.WithToolCapabilities(true)}, opts...)
	s := server.NewMCPServer("fixture", "1.2.3", opts...)
	s.AddTool(mcp.NewTool("echo", mcp.WithDescription("Echo the text back."), mcp.WithString("text", mcp.Required(), mcp.Description("what to echo"))),
		func(_ context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			f.mu.Lock()
			f.calls = append(f.calls, "echo")
			f.mu.Unlock()
			return mcp.NewToolResultText("echo: " + r.GetString("text", "")), nil
		})
	s.AddTool(mcp.NewTool("fail", mcp.WithDescription("Always fails.")),
		func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultError("it failed"), nil
		})
	s.AddTool(mcp.NewTool("slow", mcp.WithDescription("Waits until cancelled.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			<-ctx.Done()
			return mcp.NewToolResultText("late"), nil
		})
	s.AddTool(mcp.NewTool("big", mcp.WithDescription("A very long answer.")),
		func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(strings.Repeat("x", stacksdk.MaxDynamicResult+1000)), nil
		})
	return s
}

// startStreamable runs the fixture over Streamable HTTP, behind a handler that records the sign-in headers and may require one.
func startStreamable(t *testing.T, require func(*http.Request) bool, opts ...server.ServerOption) *fixture {
	t.Helper()
	f := &fixture{seen: map[string]bool{}, dead: map[string]bool{}}
	f.mcp = newMCPServer(f, opts...)
	inner := server.NewStreamableHTTPServer(f.mcp)
	f.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if f.sessionGone(r) {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if require != nil && !require(r) {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(f.http.Close)
	f.URL = f.http.URL + "/mcp"
	return f
}

// startSSE runs the fixture over the old HTTP+SSE transport only.
func startSSE(t *testing.T, opts ...server.ServerOption) *fixture {
	t.Helper()
	f := &fixture{}
	f.mcp = newMCPServer(f, opts...)
	f.http = server.NewTestServer(f.mcp)
	t.Cleanup(f.http.Close)
	f.URL = f.http.URL + "/sse"
	return f
}

func newTestPool(t *testing.T, h stacksdk.HTTP) *Pool {
	t.Helper()
	p := NewPool(h, openPlain, nil, fastTiming())
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	t.Cleanup(func() { cancel(); p.Stop() })
	return p
}

func settled(t *testing.T, p *Pool) {
	t.Helper()
	p.WaitReady(context.Background(), 5*time.Second)
}

func byName(views []View, name string) View {
	for _, v := range views {
		if v.Name == name {
			return v
		}
	}
	return View{}
}
