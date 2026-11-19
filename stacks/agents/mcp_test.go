package agents

import (
	"context"
	"encoding/json"
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

// mcpHost is a host whose settings really save, whose vault seals with a visible prefix, and whose HTTP port is a plain client.
type mcpHost struct {
	mu       sync.Mutex
	settings map[string]json.RawMessage
	logged   []string
	files    map[string][]byte
}

func newMCPHost() *mcpHost {
	return &mcpHost{settings: map[string]json.RawMessage{}, files: map[string][]byte{}}
}

func (m *mcpHost) host() stacksdk.Host {
	f := &fakeHost{files: m.files, settings: "{}"}
	h := f.host()
	h.Config = mcpConfig{m}
	h.Log = mcpLog{m}
	h.Vault = prefixVault{}
	h.HTTP = plainHTTP{}
	return h
}

func (m *mcpHost) doc() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.Marshal(m.settings)
	return string(b)
}

func (m *mcpHost) set(key, raw string) {
	m.mu.Lock()
	m.settings[key] = json.RawMessage(raw)
	m.mu.Unlock()
}

type mcpConfig struct{ m *mcpHost }

func (c mcpConfig) Load(context.Context) (json.RawMessage, error) {
	return json.RawMessage(c.m.doc()), nil
}

func (c mcpConfig) Save(_ context.Context, doc json.RawMessage) error {
	var over map[string]json.RawMessage
	if err := json.Unmarshal(doc, &over); err != nil {
		return err
	}
	c.m.mu.Lock()
	for k, v := range over {
		c.m.settings[k] = v
	}
	c.m.mu.Unlock()
	return nil
}

type mcpLog struct{ m *mcpHost }

func (l mcpLog) Log(_ stacksdk.Level, msg string, fields ...string) {
	l.m.mu.Lock()
	l.m.logged = append(l.m.logged, msg+" "+strings.Join(fields, " "))
	l.m.mu.Unlock()
}

type prefixVault struct{}

func (prefixVault) Tag(context.Context, string, []byte) ([]byte, error) { return nil, nil }
func (prefixVault) Seal(_ context.Context, purpose string, v []byte) ([]byte, error) {
	return []byte("sealed[" + purpose + "]:" + string(v)), nil
}
func (prefixVault) Open(_ context.Context, purpose string, s []byte) ([]byte, error) {
	rest, ok := strings.CutPrefix(string(s), "sealed["+purpose+"]:")
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, "does not open")
	}
	return []byte(rest), nil
}

type plainHTTP struct{}

func (plainHTTP) Client(stacksdk.HTTPOptions) (*http.Client, error) { return &http.Client{}, nil }

// startMCPServer runs a small MCP server over Streamable HTTP; need, when set, must accept the Authorization header.
func startMCPServer(t *testing.T, need string) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("fixture", "1.0", server.WithToolCapabilities(true))
	s.AddTool(mcp.NewTool("echo", mcp.WithDescription("Echo."), mcp.WithString("text", mcp.Required())),
		func(_ context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("echo: " + r.GetString("text", "")), nil
		})
	inner := server.NewStreamableHTTPServer(s)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if need != "" && r.Header.Get("Authorization") != need {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func startedStack(t *testing.T) (*Stack, *mcpHost) {
	t.Helper()
	h := newMCPHost()
	s := New()
	if err := s.Start(t.Context(), h.host()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s, h
}

func call(t *testing.T, s *Stack, method, path, body string) (int, map[string]any) {
	t.Helper()
	path, query, _ := strings.Cut(path, "?")
	req := stacksdk.Request{Method: method, Path: path, Query: query, Body: []byte(body), ContentType: "application/json"}
	resp, err := s.Handle(t.Context(), req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	var out map[string]any
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &out)
	}
	return resp.Status, out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestMCP_AddingAServerConnectsItAndItsToolsAreOfferedAndCallable(t *testing.T) {
	ts := startMCPServer(t, "")
	s, h := startedStack(t)
	status, out := call(t, s, http.MethodPost, RouteMCPServers, `{"name":"files","url":"`+ts.URL+`/mcp"}`)
	if status != http.StatusCreated || out["name"] != "files" || out["trust"] != "ask" {
		t.Fatalf("add: %d %v", status, out)
	}
	waitFor(t, "the tool to be offered", func() bool { tools, _ := s.ListTools(t.Context()); return len(tools) == 1 })
	tools, _ := s.ListTools(t.Context())
	if tools[0].Name != "mcp__files__echo" || !tools[0].Ask || tools[0].Source != "files" {
		t.Fatalf("tool: %+v", tools[0])
	}
	res, err := s.CallDynamic(t.Context(), "mcp__files__echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil || res.Text != "echo: hi" {
		t.Fatalf("call: %+v %v", res, err)
	}
	if !strings.Contains(h.doc(), `"mcp_servers"`) {
		t.Error("the server is saved in the stack's settings")
	}
	status, list := call(t, s, http.MethodGet, RouteMCPServers, "")
	if status != http.StatusOK || list["enabled"] != true {
		t.Fatalf("list: %d %v", status, list)
	}
}

func TestMCP_TheSignInValueIsSealedAndNeverComesBackOrIsLogged(t *testing.T) {
	ts := startMCPServer(t, "Bearer top-secret-token")
	s, h := startedStack(t)
	status, out := call(t, s, http.MethodPost, RouteMCPServers,
		`{"name":"files","url":"`+ts.URL+`/mcp","auth_kind":"bearer","secret":"top-secret-token"}`)
	if status != http.StatusCreated || out["has_secret"] != true {
		t.Fatalf("add: %d %v", status, out)
	}
	waitFor(t, "the tool", func() bool { tools, _ := s.ListTools(t.Context()); return len(tools) == 1 })
	doc := h.doc()
	if strings.Contains(doc, "top-secret-token") && !strings.Contains(doc, "sealed[mcp]:top-secret-token") {
		t.Errorf("the settings must hold the sealed value only: %s", doc)
	}
	_, list := call(t, s, http.MethodGet, RouteMCPServers, "")
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "top-secret-token") || strings.Contains(string(raw), "sealed[") {
		t.Errorf("no route may return the value: %s", raw)
	}
	for _, line := range h.logged {
		if strings.Contains(line, "top-secret-token") {
			t.Errorf("the value was logged: %s", line)
		}
	}
	// A bearer sign-in with no value is refused at the door.
	status, out = call(t, s, http.MethodPost, RouteMCPServers, `{"name":"nokey","url":"`+ts.URL+`/mcp","auth_kind":"bearer"}`)
	if status != http.StatusBadRequest || !strings.Contains(out["error"].(string), "required") {
		t.Errorf("a sign-in without a value: %d %v", status, out)
	}
}

func TestMCP_AStoredValueIsNeverSentToAnotherAddress(t *testing.T) {
	ts := startMCPServer(t, "")
	other := startMCPServer(t, "")
	s, _ := startedStack(t)
	call(t, s, http.MethodPost, RouteMCPServers, `{"name":"files","url":"`+ts.URL+`/mcp","auth_kind":"bearer","secret":"v1"}`)

	// The same address keeps the value without it being typed again.
	status, out := call(t, s, http.MethodPut, RouteMCPServers, `{"name":"files","url":"`+ts.URL+`/other-path","auth_kind":"bearer","trust":"allow"}`)
	if status != http.StatusOK || out["has_secret"] != true || out["trust"] != "allow" {
		t.Fatalf("same host: %d %v", status, out)
	}
	// Another address needs the value again.
	status, out = call(t, s, http.MethodPut, RouteMCPServers, `{"name":"files","url":"`+other.URL+`/mcp","auth_kind":"bearer"}`)
	if status != http.StatusBadRequest || !strings.Contains(out["error"].(string), "enter the sign-in value again") {
		t.Fatalf("another host: %d %v", status, out)
	}
	// With the value typed again it is accepted.
	status, out = call(t, s, http.MethodPut, RouteMCPServers, `{"name":"files","url":"`+other.URL+`/mcp","auth_kind":"bearer","secret":"v2"}`)
	if status != http.StatusOK || out["has_secret"] != true {
		t.Fatalf("typed again: %d %v", status, out)
	}
	// Signing in with nothing drops the stored value.
	status, out = call(t, s, http.MethodPut, RouteMCPServers, `{"name":"files","url":"`+other.URL+`/mcp","auth_kind":"none"}`)
	if status != http.StatusOK || out["has_secret"] != false {
		t.Fatalf("no sign-in: %d %v", status, out)
	}
}

func TestMCP_TheRoutesRefuseWhatIsWrong(t *testing.T) {
	ts := startMCPServer(t, "")
	s, _ := startedStack(t)
	good := `{"name":"files","url":"` + ts.URL + `/mcp"}`
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"add", http.MethodPost, RouteMCPServers, good, http.StatusCreated},
		{"the same name twice", http.MethodPost, RouteMCPServers, good, http.StatusConflict},
		{"a bad name", http.MethodPost, RouteMCPServers, `{"name":"Bad Name","url":"http://x/mcp"}`, http.StatusBadRequest},
		{"a bad address", http.MethodPost, RouteMCPServers, `{"name":"b","url":"ftp://x"}`, http.StatusBadRequest},
		{"not JSON", http.MethodPost, RouteMCPServers, `nope`, http.StatusBadRequest},
		{"change an unknown server", http.MethodPut, RouteMCPServers, `{"name":"ghost","url":"http://x/mcp"}`, http.StatusNotFound},
		{"remove an unknown server", http.MethodDelete, RouteMCPServers + "?name=ghost", "", http.StatusNotFound},
		{"remove with no name", http.MethodDelete, RouteMCPServers, "", http.StatusBadRequest},
		{"test an unknown server", http.MethodPost, RouteMCPTest, `{"name":"ghost"}`, http.StatusNotFound},
		{"remove", http.MethodDelete, RouteMCPServers + "?name=files", "", http.StatusNoContent},
	}
	for _, c := range cases {
		if status, out := call(t, s, c.method, c.path, c.body); status != c.want {
			t.Errorf("%s: got %d %v, want %d", c.name, status, out, c.want)
		}
	}
	if tools, _ := s.ListTools(t.Context()); len(tools) != 0 {
		t.Error("a removed server offers nothing")
	}
}

func TestMCP_TestingAnEntryReportsWhatHappenedWithoutSavingIt(t *testing.T) {
	ts := startMCPServer(t, "Bearer ok-token")
	s, h := startedStack(t)
	status, out := call(t, s, http.MethodPost, RouteMCPTest, `{"url":"`+ts.URL+`/mcp","auth_kind":"bearer","secret":"ok-token"}`)
	if status != http.StatusOK || out["status"] != "ready" {
		t.Fatalf("good entry: %d %v", status, out)
	}
	status, out = call(t, s, http.MethodPost, RouteMCPTest, `{"url":"`+ts.URL+`/mcp","auth_kind":"bearer","secret":"wrong"}`)
	if status != http.StatusOK || out["status"] != "stopped" || out["problem"] == "" {
		t.Fatalf("wrong value: %d %v", status, out)
	}
	if strings.Contains(h.doc(), "mcp_servers") {
		t.Error("a test saves nothing")
	}
	call(t, s, http.MethodPost, RouteMCPServers, `{"name":"files","url":"`+ts.URL+`/mcp","auth_kind":"bearer","secret":"ok-token"}`)
	status, out = call(t, s, http.MethodPost, RouteMCPTest, `{"name":"files"}`)
	if status != http.StatusOK || out["status"] != "ready" {
		t.Fatalf("saved entry: %d %v", status, out)
	}
}

func TestMCP_WithTheModuleOffNothingIsOfferedCalledOrDialled(t *testing.T) {
	ts := startMCPServer(t, "")
	s, h := startedStack(t)
	call(t, s, http.MethodPost, RouteMCPServers, `{"name":"files","url":"`+ts.URL+`/mcp"}`)
	waitFor(t, "the tool", func() bool { tools, _ := s.ListTools(t.Context()); return len(tools) == 1 })

	h.set("mcp_off", "true")
	if err := s.ConfigChanged(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tools, _ := s.ListTools(t.Context()); len(tools) != 0 {
		t.Fatalf("a switched-off module offers nothing: %v", tools)
	}
	if _, err := s.CallDynamic(t.Context(), "mcp__files__echo", json.RawMessage(`{"text":"x"}`)); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable || !strings.Contains(err.Error(), "switched off") {
		t.Fatalf("a call is refused in words: %v", err)
	}
	st, _ := s.Status(t.Context())
	if mod := st[len(st)-1]; mod.ID != MCPModuleID || mod.Enabled {
		t.Errorf("status: %+v", mod)
	}
	_, list := call(t, s, http.MethodGet, RouteMCPServers, "")
	if list["enabled"] != false {
		t.Errorf("the page can say why nothing runs: %v", list)
	}

	h.set("mcp_off", "false")
	_ = s.ConfigChanged(t.Context())
	waitFor(t, "the tool to return", func() bool { tools, _ := s.ListTools(t.Context()); return len(tools) == 1 })
}

func TestMCP_AServerThatCannotBeReachedMakesTheModuleSayso(t *testing.T) {
	ts := startMCPServer(t, "")
	url := ts.URL + "/mcp"
	ts.Close()
	s, _ := startedStack(t)
	call(t, s, http.MethodPost, RouteMCPServers, `{"name":"files","url":"`+url+`"}`)
	waitFor(t, "the problem", func() bool {
		st, _ := s.Status(t.Context())
		return st[len(st)-1].Problem == ProblemMCPServer
	})
}

func TestMCP_BeforeStartAndAfterStopNothingIsOfferedAndRoutesSayNotRunning(t *testing.T) {
	s := New()
	if tools, err := s.ListTools(t.Context()); err != nil || len(tools) != 0 {
		t.Errorf("before start: %v %v", tools, err)
	}
	if _, err := s.CallDynamic(t.Context(), "mcp__a__b", nil); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("before start a call is unavailable: %v", err)
	}
	if resp, _ := s.Handle(t.Context(), stacksdk.Request{Method: http.MethodGet, Path: RouteMCPServers}); resp.Status == http.StatusOK {
		t.Error("the list route needs the running stack")
	}
}
