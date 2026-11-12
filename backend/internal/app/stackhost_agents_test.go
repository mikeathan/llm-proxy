package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/agentwatch"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/stackport"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// The registry (core) and the manifest (stack) both describe the Agents modules. The manifest is the source of what a module is, and the registry
// must not drift from it, including the words the module's card shows.
func TestAgentsStack_TheRegistryModulesMatchTheManifestSpecs(t *testing.T) {
	reg := allModules(t, newTestStacks(t).reg)
	for _, spec := range agentsStack().Manifest().Modules {
		var found bool
		for _, m := range reg {
			if m.ID != spec.ID {
				continue
			}
			found = true
			if m.Title != spec.Title || m.Purpose != spec.Purpose || m.Layer != spec.Layer || string(m.Sends) != string(spec.Sends) || m.StepUp != spec.StepUp || m.Stack != stack.Agents {
				t.Errorf("registry %+v differs from manifest %+v", m, spec)
			}
			if m.About.Watches != spec.About.Watches || m.About.Never != spec.About.Never || m.About.ShowsUp != spec.About.ShowsUp {
				t.Errorf("%s: the card's words differ: registry %+v, manifest %+v", spec.ID, m.About, spec.About)
			}
		}
		if !found {
			t.Errorf("module %s is not in the registry", spec.ID)
		}
	}
	if got := len(agentsStack().Manifest().Modules); got != 2 {
		t.Errorf("the manifest has %d modules, the registry's Agents stack two", got)
	}
}

// The ids of the alerts the stack publishes and the ids the core has words for are one list.
func TestAgentsStack_TheCoreHasWordsForEveryAlertTheStackPublishes(t *testing.T) {
	core, stk := agentwatch.AlertIDs(), agentAlertIDs()
	slices.Sort(core)
	slices.Sort(stk)
	if !slices.Equal(core, stk) {
		t.Fatalf("the core words %v, the stack publishes %v", core, stk)
	}
}

// A file a stack lists in its manifest is the stack's, and two stacks never claim one.
func TestAgentsStack_NoCoreStoreUsesTheFileItClaimsAndNoOtherStackDoes(t *testing.T) {
	files := agentsStack().Manifest().Storage
	if len(files) != 1 || files[0] != "agent-watch.json" {
		t.Fatalf("the stack claims %v; the file keeps the name it always had", files)
	}
	if _, err := claimedFiles(networkStack().Manifest(), agentsStack().Manifest()); err != nil {
		t.Error(err)
	}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), `"agent-watch.json"`) {
			t.Errorf("%s names agent-watch.json, a file the Agents stack's manifest claims", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The Agents stack's one route is a route like any other: registered with the core's router, and a read that needs no passphrase.
func TestAgentsStack_ItsRouteIsRegisteredAndNeedsNoPassphrase(t *testing.T) {
	routes := allRoutes(t)
	for _, rt := range agentsStack().Manifest().Routes {
		if !slices.Contains(routes, rt.Method+" "+rt.Path) {
			t.Errorf("the stack declares %s %s but it is not registered", rt.Method, rt.Path)
		}
	}
	if steps := stackStepUpRoutes([]stacksdk.Stack{agentsStack()}); len(steps) != 0 {
		t.Errorf("reading the record needs no passphrase: %v", steps)
	}
}

// ---- the whole path, with the real adapters ----

type sentMessages struct {
	mu   sync.Mutex
	msgs []string
}

func (s *sentMessages) notify(_ context.Context, m string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	return nil
}

func (s *sentMessages) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.msgs)
}

type agentsRig struct {
	host   *stackHost
	feed   agentwatch.Feed
	policy *memAgents
	dir    string
	sent   *sentMessages
	cancel context.CancelFunc
}

// newAgentsRig hosts the stack over the same adapters the service uses: the typed settings behind the Config port, the data folder behind
// DataDir, the core's wording behind Events, and the core's folder walk behind SecretScan.
func newAgentsRig(t *testing.T) *agentsRig {
	t.Helper()
	r := &agentsRig{policy: &memAgents{}, dir: t.TempDir(), sent: &sentMessages{}}
	st := agentsStack()
	host := stacksdk.Host{
		Config:     stackport.Config[models.AgentsConfig]{Store: r.policy},
		Events:     agentwatch.Alerts{Notify: r.sent.notify},
		Log:        nopLog{},
		DataDir:    stackport.DataDir{Dir: r.dir, Files: st.Manifest().Storage},
		SecretScan: agentwatch.Scanner{},
	}
	h, err := newStackHost(st, host)
	if err != nil {
		t.Fatal(err)
	}
	r.host = h
	r.feed = agentwatch.Feed{Push: h.Observe, ActivityOn: agentActivitySwitch(r.policy).Enabled}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	h.Run(ctx)
	t.Cleanup(cancel)
	return r
}

func (r *agentsRig) waitForMessages(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(r.sent.all()) < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return r.sent.all()
}

func (r *agentsRig) get(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	router := api.NewRouter()
	if err := api.MountStackRoutes(router, r.host.stack, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/agent-watch", nil))
	return rec
}

// A refused tool call on the event bus becomes the same record entry and the same sentence as before the stack existed, the record is in the file
// where it always was, and the screen's route shows it.
func TestAgentsStack_ARefusalOnTheBusIsTheSameAlertAndRecordAsBefore(t *testing.T) {
	r := newAgentsRig(t)
	r.feed.Observe("ws", assistant.AgentEvent{Type: assistant.EventGuardrailViolation, Channel: assistant.ChannelAutomation, Automation: "nightly",
		Payload: map[string]string{"tool": "read_file", "error": "path /etc/shadow is outside the workspace"}})
	got := r.waitForMessages(t, 1)
	const want = `Agent watch: the automation "nightly" in workspace "ws" tried a tool call the safety rules refused (read_file). Open its last run to see what it was doing.`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("alerts = %q, want [%q]", got, want)
	}
	raw, err := os.ReadFile(filepath.Join(r.dir, "agent-watch.json"))
	if err != nil {
		t.Fatalf("the record is not where it always was: %v", err)
	}
	if strings.Contains(string(raw), "shadow") {
		t.Fatalf("an error text reached the record: %s", raw)
	}
	var rec struct {
		Events []struct {
			Kind, Workspace, Automation, Tool string
		}
	}
	if err := json.Unmarshal(raw, &rec); err != nil || len(rec.Events) != 1 || rec.Events[0].Kind != "tool_refused" || rec.Events[0].Tool != "read_file" {
		t.Fatalf("record = %s (%v)", raw, err)
	}
	resp := r.get(t)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"kind":"tool_refused"`) || !strings.Contains(resp.Body.String(), `"automation":"nightly"`) {
		t.Fatalf("GET: %d %s", resp.Code, resp.Body.String())
	}
}

// Switching the activity part off in the core's typed settings silences the record at once, because the feed reads the switch on every event.
func TestAgentsStack_SwitchingActivityOffStopsTheRecordAndBackOnResumesIt(t *testing.T) {
	r := newAgentsRig(t)
	ev := assistant.AgentEvent{Type: assistant.EventGuardrailBlocked, Channel: assistant.ChannelAssistant, Payload: assistant.GuardrailBlockedPayload{Tool: "write_file"}}
	if err := r.policy.Update(func(c *models.AgentsConfig) error { c.ActivityOff = true; return nil }); err != nil {
		t.Fatal(err)
	}
	r.feed.Observe("ws", ev)
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(r.dir, "agent-watch.json")); err == nil {
		t.Fatal("an observation was recorded while the part is off")
	}
	if err := r.policy.Update(func(c *models.AgentsConfig) error { c.ActivityOff = false; return nil }); err != nil {
		t.Fatal(err)
	}
	r.feed.Observe("ws", ev)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(r.dir, "agent-watch.json")); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the record did not resume once the part was switched back on")
}

// A factory reset removes the record file and a copy set aside as corrupt.
func TestAgentsStack_AFactoryResetRemovesTheRecordAndItsQuarantinedCopy(t *testing.T) {
	dir := t.TempDir()
	files := agentsStack().Manifest().Storage
	for _, name := range []string{"agent-watch.json", "agent-watch.json.corrupt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := stackport.Remove(dir, files); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("left after a reset: %v", left)
	}
}

// Only the kinds the manifest lists are pushed to the stack, and a stack that was not hosted takes nothing and does not crash.
func TestStackHost_ObserveHoldsTheStackToWhatItAnnouncedAndIsSafeWhenNotHosted(t *testing.T) {
	var nilHost *stackHost
	nilHost.Observe(stacksdk.Observation{Kind: stacksdk.ObservedRefused})
	r := newAgentsRig(t)
	r.host.Observe(stacksdk.Observation{Kind: "something_else", Workspace: "ws"})
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(r.dir, "agent-watch.json")); err == nil {
		t.Error("an observation of a kind the manifest did not announce reached the stack")
	}
}

// reporting is a stack that says how one module is doing.
type reporting struct {
	portsOnly
	status []stacksdk.ModuleStatus
}

func (r reporting) Status(context.Context) ([]stacksdk.ModuleStatus, error) { return r.status, nil }

// The registry asks the hosted stacks how their modules are doing, and a module no stack reports adds nothing.
func TestObservedModule_ComesFromTheHostedStackThatRunsIt(t *testing.T) {
	ran := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	host, err := newStackHost(reporting{portsOnly: portsOnly{manifest: baseManifest(stacksdk.PortLog)}, status: []stacksdk.ModuleStatus{
		{ID: "m", Enabled: true, LastRun: ran, Problem: "the queue was full"},
		{ID: "never", Enabled: true},
	}}, stacksdk.Host{Log: nopLog{}})
	if err != nil {
		t.Fatal(err)
	}
	n := &netauditServices{Agents: host}
	if got, ok := n.observedModule("m"); !ok || !got.LastRun.Equal(ran) || got.Problem != "the queue was full" {
		t.Errorf("observed = %+v %v", got, ok)
	}
	if got, ok := n.observedModule("never"); !ok || !got.LastRun.IsZero() {
		t.Errorf("a module that has not run: %+v %v", got, ok)
	}
	if _, ok := n.observedModule("unknown"); ok {
		t.Error("no stack runs this module")
	}
	if _, ok := (&netauditServices{}).observedModule("m"); ok {
		t.Error("with no stack hosted there is nothing to ask, and no crash")
	}
}

// The real Agents stack answers for both of its modules, and says nothing is wrong while it runs.
func TestObservedModule_TheAgentsStackAnswersForBothOfItsParts(t *testing.T) {
	r := newAgentsRig(t)
	n := &netauditServices{Agents: r.host}
	for _, id := range []string{"agent_activity", "secrets_pass"} {
		if got, ok := n.observedModule(id); !ok || got.Problem != "" {
			t.Errorf("%s: %+v %v", id, got, ok)
		}
	}
}
