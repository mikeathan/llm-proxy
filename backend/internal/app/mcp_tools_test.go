package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

func echoServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("fixture", "1.0", server.WithToolCapabilities(true))
	s.AddTool(mcp.NewTool("echo", mcp.WithDescription("Echo."), mcp.WithString("text", mcp.Required())),
		func(_ context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("echo: " + r.GetString("text", "")), nil
		})
	ts := httptest.NewServer(server.NewStreamableHTTPServer(s))
	t.Cleanup(ts.Close)
	return ts
}

func postJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// The whole path with the real adapters: a server added through the stack's own route is connected, its tools reach the run path under the
// stack's names, the trust the owner set decides whether the guardrail asks, and the module switch takes the tools away again.
func TestMCPTools_AServerAddedThroughTheRouteReachesTheRunPathAndTheSwitchTakesItAway(t *testing.T) {
	r := newAgentsRig(t)
	ts := echoServer(t)
	router := api.NewRouter()
	if err := api.MountStackRoutes(router, r.host.stack, nil); err != nil {
		t.Fatal(err)
	}
	src, ok := r.host.stack.(stacksdk.ToolSource)
	if !ok {
		t.Fatal("the hosted Agents stack must provide tools")
	}
	tools := toolregistry.NewStackTools()
	tools.Set(src)

	rec := postJSON(t, router, http.MethodPost, "/admin/api/mcp/servers", `{"name":"files","url":"`+ts.URL+`/mcp"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	waitUntil(t, "the tool reaches the run path", func() bool {
		got, _ := tools.ListTools(context.Background())
		return len(got) == 1 && got[0].Function.Name == "mcp__files__echo"
	})
	if managed, ask := tools.Policy("mcp__files__echo"); !managed || !ask {
		t.Fatalf("a new server asks first: managed=%v ask=%v", managed, ask)
	}
	if got, err := tools.CallTool(context.Background(), "mcp__files__echo", map[string]any{"text": "hi"}); err != nil || got != "echo: hi" {
		t.Fatalf("call: %#v %v", got, err)
	}

	// Allow is the owner's setting, saved through the stack and kept in the core's typed settings with every field.
	rec = postJSON(t, router, http.MethodPut, "/admin/api/mcp/servers",
		`{"name":"files","url":"`+ts.URL+`/mcp","trust":"allow","disabled_tools":["other"],"ca_cert":"/etc/ssl/own.pem","auth_kind":"header","auth_header":"X-Key","secret":"s3cret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change: %d %s", rec.Code, rec.Body.String())
	}
	saved := r.policy.Get().MCPServers
	if len(saved) != 1 || saved[0].Trust != "allow" || saved[0].CACert != "/etc/ssl/own.pem" || len(saved[0].DisabledTools) != 1 ||
		saved[0].Auth.Kind != "header" || saved[0].Auth.Header != "X-Key" || saved[0].Auth.Sealed == "" || strings.Contains(saved[0].Auth.Sealed, "s3cret") {
		t.Fatalf("the core's settings must keep every field of the stack's entry, with the value sealed: %+v", saved)
	}
	if rec.Body.Len() > 0 && strings.Contains(rec.Body.String(), "s3cret") {
		t.Error("the value must never come back")
	}

	// The module switch, as the core's registry flips it.
	if err := r.policy.Update(func(c *models.AgentsConfig) error { c.MCPOff = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.host.stack.ConfigChanged(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, _ := tools.ListTools(context.Background()); len(got) != 0 {
		t.Fatalf("a switched-off module offers nothing: %v", got)
	}
	if _, err := tools.CallTool(context.Background(), "mcp__files__echo", map[string]any{"text": "x"}); err == nil || !strings.Contains(err.Error(), "switched off") {
		t.Fatalf("a call is refused in words: %v", err)
	}
	if managed, ask := tools.Policy("mcp__files__echo"); !managed || !ask {
		t.Error("with the module off a name that looks like a stack's still asks and never passes")
	}
}

func waitUntil(t *testing.T, what string, ok func() bool) {
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

func TestMCPTools_WithNoStackAttachedNothingIsOfferedAndNothingCrashes(t *testing.T) {
	var s AppServices
	s.attachDynamicTools(nil)
	s.dynamicTools = toolregistry.NewStackTools()
	s.attachDynamicTools(nil)
	if got, err := s.dynamicTools.ListTools(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
}

// ---- the one-time move of the registry's servers ----

func TestMigrateMCPServers_CopiesOnceWithNamesTheStackAcceptsAndDropsTheOldList(t *testing.T) {
	mgr, _ := newTestDataManager(t)
	regPath := mgr.Paths().RegistryFile()
	old := `{"providers":{},"catalogue":[],"mcp_servers":[
		{"name":"My Server","url":"http://192.168.1.5:8931/mcp","enabled":true,"tls_ca_cert":"/etc/ca.pem"},
		{"name":"my-server","url":"http://x/mcp","enabled":false},
		{"name":"9lives","url":"http://y/mcp","enabled":true},
		{"name":"","url":"http://z/mcp","enabled":true},
		{"name":"averyveryveryverylongservernamethatdoesnotfit","url":"http://w/mcp","enabled":true}]}`
	if err := os.WriteFile(regPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	agents := mgr.Agents()
	if err := migrateMCPServers(mgr, agents); err != nil {
		t.Fatal(err)
	}
	got := agents.Get().MCPServers
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
		if s.Trust != "ask" {
			t.Errorf("%s: a copied server asks first, got %q", s.Name, s.Trust)
		}
		if len(s.Name) > 24 || s.Name == "" || s.Name[0] < 'a' || s.Name[0] > 'z' || strings.ContainsAny(s.Name, "_ ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			t.Errorf("name %q does not fit the stack's rule", s.Name)
		}
	}
	if len(got) != 5 {
		t.Fatalf("every entry is copied, even one that needs fixing: %v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("name %q is used twice: %v", n, names)
		}
		seen[n] = true
	}
	if got[0].URL != "http://192.168.1.5:8931/mcp" || !got[0].Enabled || got[0].CACert != "/etc/ca.pem" || got[1].Enabled {
		t.Errorf("address, switch and CA file are kept: %+v %+v", got[0], got[1])
	}
	rewritten, err := os.ReadFile(filepath.Clean(regPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), "mcp_servers") {
		t.Errorf("the registry must be written again without the old list: %s", rewritten)
	}

	// Again: nothing to do, and nothing duplicated.
	if err := migrateMCPServers(mgr, agents); err != nil {
		t.Fatal(err)
	}
	if again := agents.Get().MCPServers; len(again) != 5 {
		t.Errorf("a second start must not copy again: %d", len(again))
	}
}

func TestMigrateMCPServers_ARegistryWithNoOldListOrNoFileIsLeftAlone(t *testing.T) {
	mgr, _ := newTestDataManager(t)
	if err := migrateMCPServers(mgr, mgr.Agents()); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Agents().Get().MCPServers; len(got) != 0 {
		t.Errorf("%v", got)
	}
}

// newTestDataManager is a data manager over a seeded temporary folder, with its stores loaded.
func newTestDataManager(t *testing.T) (*storage.DataManager, paths.Paths) {
	t.Helper()
	p := seededPaths(t, t.TempDir())
	mgr, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	return mgr, p
}
