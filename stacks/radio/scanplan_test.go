package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"llm-proxy-stacks/sdk"
)

// memConfig keeps the document the way the core does: a saved document is merged over the stored one.
type memConfig struct {
	mu  sync.Mutex
	doc map[string]json.RawMessage
}

func (c *memConfig) Load(context.Context) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Marshal(c.doc)
}

func (c *memConfig) Save(_ context.Context, d json.RawMessage) error {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(d, &in); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range in {
		c.doc[k] = v
	}
	return nil
}

func scanRig(t *testing.T) (*Stack, *memConfig) {
	t.Helper()
	cfg := &memConfig{doc: map[string]json.RawMessage{"enabled": json.RawMessage("true")}}
	s := New(fstest.MapFS{})
	h := host("", &pathDir{dir: t.TempDir()})
	h.Config = cfg
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s, cfg
}

func scanCall(t *testing.T, s *Stack, method, body string) (stacksdk.Response, ScanView) {
	t.Helper()
	resp, err := s.Handle(context.Background(), stacksdk.Request{Method: method, Path: ScanPath, Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	var v ScanView
	_ = json.Unmarshal(resp.Body, &v)
	return resp, v
}

func TestScanPlanDefaultsThenAnOperatorPlanThenAReset(t *testing.T) {
	s, cfg := scanRig(t)
	_, v := scanCall(t, s, http.MethodGet, "")
	if v.Custom || len(v.Plan.Channels) != 6 || v.Plan.WindowSeconds != 20 || v.Plan.CycleSeconds != 300 || v.WindowsPerHour != 72 {
		t.Fatalf("default plan: %+v", v)
	}
	resp, v := scanCall(t, s, http.MethodPut, `{"channels":[36,1,11,6],"window_seconds":10,"cycle_seconds":120}`)
	if resp.Status != http.StatusOK || !v.Custom || len(v.Plan.Channels) != 4 || v.Plan.Channels[0] != 1 || v.Plan.WindowSeconds != 10 || v.WindowsPerHour != 120 {
		t.Fatalf("set plan: %d %+v", resp.Status, v)
	}
	if string(cfg.doc["enabled"]) != "true" {
		t.Error("saving a plan must not touch the switch")
	}
	_, v = scanCall(t, s, http.MethodPut, `{"channels":[],"window_seconds":0,"cycle_seconds":0}`)
	if v.Custom || len(v.Plan.Channels) != 6 {
		t.Errorf("empty fields reset to the default: %+v", v)
	}
}

func TestScanPlanRefusesWhatCannotRunAndSaysWhy(t *testing.T) {
	s, cfg := scanRig(t)
	cases := map[string]string{
		`{"channels":[1,99]}`:   "channel 99 cannot be listened on",
		`{"channels":[1,1]}`:    "listed twice",
		`{"window_seconds":31}`: "a window lasts 1 to 30 seconds",
		`{"cycle_seconds":30}`:  "at least 60 seconds",
		`{"channels":[1,6,11,36],"window_seconds":30,"cycle_seconds":100}`:   "do not fit in one cycle",
		`{"channels":[1,2,3,4,5,6,7,8,9,10,11,12,13,36],"cycle_seconds":60}`: "windows an hour",
		`{"channels":[1],"speed":3}`:                                         "send channels",
	}
	for body, want := range cases {
		resp, _ := scanCall(t, s, http.MethodPut, body)
		if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), want) {
			t.Errorf("%s: %d %s, want a refusal saying %q", body, resp.Status, resp.Body, want)
		}
	}
	if _, ok := cfg.doc["channels"]; ok {
		t.Error("a refused plan must save nothing")
	}
}

func TestAStartedRunUsesTheSavedPlan(t *testing.T) {
	s, _ := scanRig(t)
	scanCall(t, s, http.MethodPut, `{"channels":[6,1],"window_seconds":5,"cycle_seconds":90}`)
	resp, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: PlanPath, Query: "adapter=wlan9"})
	if err != nil {
		t.Fatal(err)
	}
	var pv PlanView
	_ = json.Unmarshal(resp.Body, &pv)
	if len(pv.Plan.Channels) != 2 || pv.Plan.Channels[0] != 1 || pv.Plan.WindowSeconds != 5 || len(pv.Requests) < 2 {
		t.Errorf("the session must approve the saved plan: %+v", pv)
	}
}
