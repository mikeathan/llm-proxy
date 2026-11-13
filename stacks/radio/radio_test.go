package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"llm-proxy-stacks/sdk"
)

type fakeConfig struct{ doc string }

func (c fakeConfig) Load(context.Context) (json.RawMessage, error) {
	return json.RawMessage(c.doc), nil
}
func (fakeConfig) Save(context.Context, json.RawMessage) error { return nil }

type fakeLog struct{}

func (fakeLog) Log(stacksdk.Level, string, ...string) {}

// recordingDir fails the test on any write: with no adapter, and in R1 always, the stack writes nothing.
type recordingDir struct {
	mu     sync.Mutex
	writes int
}

func (d *recordingDir) Read(string) ([]byte, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "none")
}
func (d *recordingDir) Write(string, []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes++
	return nil
}
func (d *recordingDir) Quarantine(string) error { return nil }

func host(doc string, dir stacksdk.DataDir) stacksdk.Host {
	return stacksdk.Host{Config: fakeConfig{doc}, Log: fakeLog{}, DataDir: dir}
}

func TestManifest_IsValidAndDeclaresNoPrivilege(t *testing.T) {
	m := New(fstest.MapFS{}).Manifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("the manifest must validate: %v", err)
	}
	if m.ID != ID || len(m.Modules) == 0 || m.Modules[0].ID != SensorsModuleID {
		t.Errorf("manifest = %+v", m)
	}
	for _, mod := range m.Modules {
		if mod.Sends != stacksdk.SendsNothing {
			t.Errorf("module %q sends %q; the Radio stack sends nothing", mod.ID, mod.Sends)
		}
	}
	for _, rt := range m.Routes {
		if rt.Action != "" || rt.Method != http.MethodGet {
			t.Errorf("R1 has only reads and no privileged action: %+v", rt)
		}
	}
}

// The idle rule: no adapter means no goroutine started and nothing written, even with the stack switched on.
func TestStart_WithNoAdapterStartsNoGoroutineAndWritesNothing(t *testing.T) {
	dir := &recordingDir{}
	s := New(fstest.MapFS{})
	before := runtime.NumGoroutine()
	if err := s.Start(context.Background(), host(`{"enabled":true}`, dir)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines went from %d to %d: the idle stack must start none", before, after)
	}
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath}); err != nil {
		t.Fatal(err)
	}
	if dir.writes != 0 {
		t.Errorf("the idle stack wrote %d times", dir.writes)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHandle_ListsTheSensorsWithTheirCapabilitiesAndTheSwitch(t *testing.T) {
	sys := fstest.MapFS{}
	usbWifi(sys, "wlx00", "mt7601u", "148f", "7601", "1", "down")
	s := New(sys)
	if err := s.Start(context.Background(), host(`{}`, &recordingDir{})); err != nil {
		t.Fatal(err)
	}
	resp, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("resp = %+v, %v", resp, err)
	}
	var v View
	if err := json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatal(err)
	}
	if v.Enabled {
		t.Errorf("the stack is off until the operator switches it on")
	}
	if len(v.Sensors) != 1 || v.Sensors[0].ID != "wlx00" || len(v.Sensors[0].Capabilities) == 0 {
		t.Errorf("view = %+v", v)
	}
	if v.Unused == 0 {
		t.Errorf("the count of capabilities not used yet is part of the view: %+v", v)
	}
}

func TestHandle_RefusesWhatItDoesNotKnowAndAStackThatIsNotRunning(t *testing.T) {
	s := New(fstest.MapFS{})
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath}); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("not running: %v", err)
	}
	_ = s.Start(context.Background(), host(`{}`, &recordingDir{}))
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: RoutePath}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an unknown route: %v", err)
	}
}

func TestStatus_ReportsTheSwitchAndStaysQuietWhenNotRunning(t *testing.T) {
	s := New(fstest.MapFS{})
	if st, _ := s.Status(context.Background()); len(st) != 0 {
		t.Errorf("not started: %+v", st)
	}
	_ = s.Start(context.Background(), host(`{"enabled":true}`, &recordingDir{}))
	st, _ := s.Status(context.Background())
	if len(st) != 1 || st[0].ID != SensorsModuleID || !st[0].Enabled {
		t.Errorf("status = %+v", st)
	}
}

func TestStack_StartTwiceIsRefusedAndNoToolsOrFindingsYet(t *testing.T) {
	s := New(fstest.MapFS{})
	if err := s.Start(context.Background(), host(`{}`, &recordingDir{})); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), host(`{}`, &recordingDir{})); err == nil {
		t.Error("a second Start must be refused")
	}
	if f, err := s.Findings(context.Background()); err != nil || len(f) != 0 {
		t.Errorf("findings = %v, %v", f, err)
	}
	if _, err := s.CallTool(context.Background(), "x", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("no tools: %v", err)
	}
}
