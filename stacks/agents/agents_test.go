package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// fakeHost is every port the manifest requires, in memory.
type fakeHost struct {
	mu       sync.Mutex
	files    map[string][]byte
	readErr  error
	settings string
	alerts   []stacksdk.Alert
	logged   []string
	hits     []stacksdk.SecretHit
	scanErr  error
	scans    int
	quarant  []string
}

func newFakeHost() *fakeHost { return &fakeHost{files: map[string][]byte{}, settings: `{}`} }

func (f *fakeHost) host() stacksdk.Host {
	return stacksdk.Host{Config: fakeConfig{f}, Events: fakeEvents{f}, Log: fakeLog{f}, DataDir: fakeDir{f}, SecretScan: fakeScan{f}}
}

type fakeConfig struct{ f *fakeHost }

func (c fakeConfig) Load(context.Context) (json.RawMessage, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return json.RawMessage(c.f.settings), nil
}
func (fakeConfig) Save(context.Context, json.RawMessage) error { return nil }

type fakeEvents struct{ f *fakeHost }

func (e fakeEvents) Publish(_ context.Context, ev stacksdk.Event) error {
	var a stacksdk.Alert
	if ev.Type != stacksdk.EventAlert || json.Unmarshal(ev.Data, &a) != nil || ev.Subject != a.ID {
		return errors.New("not an alert")
	}
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	e.f.alerts = append(e.f.alerts, a)
	return nil
}

type fakeLog struct{ f *fakeHost }

func (l fakeLog) Log(_ stacksdk.Level, msg string, fields ...string) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	l.f.logged = append(l.f.logged, msg+" "+strings.Join(fields, " "))
}

type fakeDir struct{ f *fakeHost }

func (d fakeDir) Read(name string) ([]byte, error) {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	if d.f.readErr != nil {
		return nil, d.f.readErr
	}
	b, ok := d.f.files[name]
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no such file")
	}
	return b, nil
}

func (d fakeDir) Write(name string, data []byte) error {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	d.f.files[name] = data
	return nil
}

func (d fakeDir) Quarantine(name string) error {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	d.f.quarant = append(d.f.quarant, name)
	delete(d.f.files, name)
	return nil
}

type fakeScan struct{ f *fakeHost }

func (s fakeScan) Scan(context.Context) ([]stacksdk.SecretHit, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.f.scans++
	return append([]stacksdk.SecretHit(nil), s.f.hits...), s.f.scanErr
}

func (f *fakeHost) record(t *testing.T) Record {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	var rec Record
	if b, ok := f.files[File]; ok {
		if err := json.Unmarshal(b, &rec); err != nil {
			t.Fatal(err)
		}
	}
	return rec
}

func (f *fakeHost) sent() []stacksdk.Alert {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]stacksdk.Alert(nil), f.alerts...)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestWatch() (*watch, *fakeHost, *clock) {
	f := newFakeHost()
	c := &clock{t: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}
	w := newWatch(c.now)
	w.bind(f.host())
	return w, f, c
}

func refused(automation, tool string) stacksdk.Observation {
	return stacksdk.Observation{Kind: stacksdk.ObservedRefused, Workspace: "ws", Automation: automation, Unattended: automation != "", Tool: tool}
}

func fetch(automation, host string) stacksdk.Observation {
	return stacksdk.Observation{Kind: stacksdk.ObservedFetch, Workspace: "ws", Automation: automation, Unattended: true, Host: host}
}

func TestWatch_ARefusalInAnAutomationIsRecordedAndAlertedOnceADay(t *testing.T) {
	w, f, c := newTestWatch()
	w.handle(t.Context(), refused("nightly", "read_file"))
	w.handle(t.Context(), refused("nightly", "read_file"))
	if got := f.record(t).Events; len(got) != 2 || got[0].Kind != KindToolRefused || got[0].Tool != "read_file" || got[0].Automation != "nightly" || got[0].Workspace != "ws" {
		t.Fatalf("events = %+v", got)
	}
	want := stacksdk.Alert{ID: AlertRefused, Workspace: "ws", Automation: "nightly", Tool: "read_file"}
	if got := f.sent(); len(got) != 1 || got[0] != want {
		t.Fatalf("alerts = %+v; want one %+v", got, want)
	}
	c.t = c.t.Add(25 * time.Hour)
	w.handle(t.Context(), refused("nightly", "read_file"))
	if len(f.sent()) != 2 {
		t.Error("a refusal on a later day alerts again")
	}
}

func TestWatch_ChatRefusalsAndApprovalsAreRecordedNotAlerted(t *testing.T) {
	w, f, _ := newTestWatch()
	w.handle(t.Context(), stacksdk.Observation{Kind: stacksdk.ObservedRefused, Workspace: "ws", Tool: "execute_terminal_command"})
	w.handle(t.Context(), stacksdk.Observation{Kind: stacksdk.ObservedApproval, Workspace: "ws", Automation: "nightly", Unattended: true, Tool: "write_file"})
	got := f.record(t).Events
	if len(got) != 2 || got[1].Kind != KindApprovalAsked || got[1].Tool != "write_file" || got[1].Automation != "nightly" {
		t.Fatalf("events = %+v", got)
	}
	if len(f.sent()) != 0 {
		t.Errorf("alerts = %+v; a person is at the chat, and an approval is a question already asked", f.sent())
	}
}

func TestWatch_DestinationsAreLearnedForAWeekThenANewHostIsReported(t *testing.T) {
	w, f, c := newTestWatch()
	w.handle(t.Context(), fetch("weather", "api.weather.example"))
	c.t = c.t.Add(6 * 24 * time.Hour)
	w.handle(t.Context(), fetch("weather", "cdn.weather.example"))
	if rec := f.record(t); len(rec.Events) != 0 || len(f.sent()) != 0 {
		t.Fatalf("the learning week records hosts only: %+v %+v", rec.Events, f.sent())
	}
	if d := f.record(t).Destinations["ws/weather"]; len(d.Hosts) != 2 || d.Hosts[0] != "api.weather.example" {
		t.Fatalf("destinations = %+v", d)
	}
	c.t = c.t.Add(2 * 24 * time.Hour)
	w.handle(t.Context(), fetch("weather", "api.weather.example"))
	w.handle(t.Context(), fetch("weather", "203.0.113.9"))
	if got := f.record(t).Events; len(got) != 1 || got[0].Kind != KindNewDestination || got[0].Host != "203.0.113.9" {
		t.Fatalf("events = %+v; want one new destination with the host only", got)
	}
	want := stacksdk.Alert{ID: AlertDestination, Workspace: "ws", Automation: "weather", Host: "203.0.113.9"}
	if got := f.sent(); len(got) != 1 || got[0] != want {
		t.Fatalf("alerts = %+v; want one %+v", got, want)
	}
	w.handle(t.Context(), fetch("weather", "203.0.113.9"))
	if len(f.record(t).Events) != 1 {
		t.Error("a host is new once")
	}
}

func TestWatch_AnAutomationThatBrowsesManyHostsOnlyRecords(t *testing.T) {
	w, f, c := newTestWatch()
	for _, h := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		w.handle(t.Context(), fetch("news", h+".news.example"))
	}
	if !f.record(t).Destinations["ws/news"].Many {
		t.Fatal("nine hosts in the learning week marks the automation as one that browses")
	}
	c.t = c.t.Add(8 * 24 * time.Hour)
	w.handle(t.Context(), fetch("news", "z.news.example"))
	if len(f.record(t).Events) != 1 || len(f.sent()) != 0 {
		t.Errorf("events %+v alerts %+v; a browsing automation's new host is recorded, never alerted", f.record(t).Events, f.sent())
	}
}

func TestWatch_AFetchWithoutAnAutomationOrAHostIsNotLearned(t *testing.T) {
	w, f, _ := newTestWatch()
	w.handle(t.Context(), fetch("", "a.example"))
	w.handle(t.Context(), fetch("news", ""))
	if len(f.files) != 0 {
		t.Errorf("a record was written for nothing: %v", f.files)
	}
}

func TestWatch_UnreadableRecordRecordsAndAlertsNothingAndIsReported(t *testing.T) {
	w, f, _ := newTestWatch()
	f.readErr = errors.New("disk")
	w.handle(t.Context(), refused("nightly", "read_file"))
	f.readErr = nil
	if len(f.sent()) != 0 || len(f.files) != 0 {
		t.Errorf("alerts %+v files %v; nothing is recorded or alerted from a record that could not be read", f.sent(), f.files)
	}
	if len(f.logged) != 1 || !strings.Contains(f.logged[0], "agent security record") {
		t.Errorf("log = %q; the failure is reported", f.logged)
	}
}

func TestWatch_ARecordThatCannotBeDecodedIsSetAsideAndTheNextOneStartsEmpty(t *testing.T) {
	w, f, _ := newTestWatch()
	f.files[File] = []byte("{not json")
	w.handle(t.Context(), refused("nightly", "read_file"))
	if len(f.quarant) != 1 || f.quarant[0] != File || len(f.sent()) != 0 {
		t.Fatalf("quarantined %v alerts %+v", f.quarant, f.sent())
	}
	w.handle(t.Context(), refused("nightly", "read_file"))
	if len(f.record(t).Events) != 1 || len(f.sent()) != 1 {
		t.Errorf("the next observation starts a fresh record: %+v %+v", f.record(t).Events, f.sent())
	}
}

func TestWatch_ObserveNeverBlocksAndRunDrains(t *testing.T) {
	w, f, _ := newTestWatch()
	for range queueSize + 10 {
		w.observe(refused("nightly", "read_file"))
	}
	if w.dropped.Load() != 10 {
		t.Errorf("dropped = %d, want 10", w.dropped.Load())
	}
	s := New()
	s.w, s.started = w, true
	if st, _ := s.Status(t.Context()); len(st) != 2 || st[0].Problem != ProblemDropped {
		t.Errorf("status = %+v; an event that did not fit the queue is reported", st)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { w.run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(f.record(t).Events) < queueSize && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if n := len(f.record(t).Events); n != queueSize {
		t.Errorf("events = %d; want the %d queued observations drained", n, queueSize)
	}
}

func TestWatch_AKindItDoesNotUseIsNeverQueued(t *testing.T) {
	w, _, _ := newTestWatch()
	w.observe(stacksdk.Observation{Kind: "something_else", Workspace: "ws"})
	if len(w.queue) != 0 {
		t.Fatalf("queued %d", len(w.queue))
	}
}

func TestRecord_AnEarlierVersionsFileReadsBackAndWritesBackWithTheSameKeys(t *testing.T) {
	// The file as the service wrote it before the stack existed (models.AgentWatch): every kind, every field.
	const before = `{
  "events": [
    {"at": "2026-11-05T09:00:00Z", "kind": "tool_refused", "workspace": "ws", "automation": "nightly", "tool": "read_file"},
    {"at": "2026-11-05T09:01:00Z", "kind": "approval_asked", "workspace": "ws", "tool": "write_file"},
    {"at": "2026-11-05T09:02:00Z", "kind": "new_destination", "workspace": "ws", "automation": "weather", "host": "203.0.113.9"},
    {"at": "2026-11-05T09:03:00Z", "kind": "secret_found", "workspace": "ws", "where": "reports/news.md", "detail": "a script token of this program"}
  ],
  "destinations": {"ws/weather": {"since": "2026-10-01T00:00:00Z", "hosts": ["a.example", "b.example"], "many": true}},
  "alerted": {"ws/nightly/refused": "2026-11-05T09:00:00Z"},
  "secrets_seen": {"ws/reports/news.md|a script token of this program": "2026-11-05T09:03:00Z"}
}`
	var rec Record
	if err := json.Unmarshal([]byte(before), &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Events) != 4 || rec.Events[3].Kind != KindSecretFound || !rec.Destinations["ws/weather"].Many {
		t.Fatalf("record = %+v", rec)
	}
	again, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	_ = json.Unmarshal([]byte(before), &a)
	_ = json.Unmarshal(again, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("the record changed shape:\n before %s\n after  %s", ja, jb)
	}
	for k, want := range map[Kind]string{KindToolRefused: "tool_refused", KindApprovalAsked: "approval_asked", KindNewDestination: "new_destination", KindSecretFound: "secret_found"} {
		if string(k) != want {
			t.Errorf("kind %q is not the stored string %q", k, want)
		}
	}
}

func TestSettings_NeitherKeyKeepsBothPartsOn(t *testing.T) {
	w, f, _ := newTestWatch()
	if s := w.settings(t.Context()); s.ActivityOff || s.SecretsPassOff {
		t.Fatalf("settings = %+v", s)
	}
	f.settings = `{"activity_off":true,"secrets_pass_off":false}`
	if s := w.settings(t.Context()); !s.ActivityOff || s.SecretsPassOff {
		t.Fatalf("settings = %+v", s)
	}
}

func TestStack_ManifestIsValidAndItsPortsArePinned(t *testing.T) {
	m := New().Manifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Settings.Key != "agents" || len(m.Storage) != 1 || m.Storage[0] != "agent-watch.json" {
		t.Errorf("the settings key and the file are unchanged from before the stack: %q %v", m.Settings.Key, m.Storage)
	}
	want := []string{stacksdk.PortConfig, stacksdk.PortEvents, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortSecretScan}
	if strings.Join(m.Requires, ",") != strings.Join(want, ",") {
		t.Errorf("requires %v; want exactly %v (no dial, no fetch, no program)", m.Requires, want)
	}
	if len(m.Routes) != 1 || m.Routes[0].Path != "/admin/api/agent-watch" || m.Routes[0].Method != http.MethodGet {
		t.Errorf("routes = %+v", m.Routes)
	}
	ids := map[string]bool{}
	for _, mod := range m.Modules {
		ids[mod.ID] = true
	}
	if !ids["agent_activity"] || !ids["secrets_pass"] || len(ids) != 2 {
		t.Errorf("modules = %v", ids)
	}
}

func TestStack_AnObservationBeforeStartWaitsInTheQueueAndIsHandledOnceRunning(t *testing.T) {
	var _ stacksdk.Observer = (*Stack)(nil)
	f := newFakeHost()
	s := New()
	s.Observe(refused("nightly", "read_file")) // the service attaches the bus before it starts the loops
	if len(f.sent()) != 0 {
		t.Fatal("nothing can be handled before the ports are bound")
	}
	if err := s.Start(t.Context(), f.host()); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), f.host()); err == nil {
		t.Error("a second Start must be refused")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(f.sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if err := s.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(t.Context()); err != nil {
		t.Errorf("Stop twice is harmless: %v", err)
	}
	if len(f.sent()) != 1 || len(f.record(t).Events) != 1 {
		t.Errorf("alerts %+v events %+v; the observation made before Start is not lost", f.sent(), f.record(t).Events)
	}
}

func TestStack_StatusSaysWhichPartsAreOnAndWhenThePassLastRan(t *testing.T) {
	f := newFakeHost()
	s := New()
	if got, _ := s.Status(t.Context()); got != nil {
		t.Fatalf("status before start = %+v", got)
	}
	if err := s.Start(t.Context(), f.host()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(t.Context()) //nolint:errcheck // the stack's Stop never fails
	f.settings = `{"activity_off":true}`
	got, _ := s.Status(t.Context())
	if len(got) != 2 || got[0].ID != ActivityModuleID || got[0].Enabled || got[1].ID != SecretsModuleID || !got[1].Enabled || !got[1].LastRun.IsZero() {
		t.Fatalf("status = %+v", got)
	}
	s.w.scanSecrets(t.Context())
	if got, _ = s.Status(t.Context()); got[1].LastRun.IsZero() {
		t.Error("the last run of the pass is reported")
	}
}

func TestHandle_NewestFirstWithDestinationsAndNoInternals(t *testing.T) {
	t0 := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	f := newFakeHost()
	rec := Record{
		Events: []Event{
			{At: t0, Kind: KindToolRefused, Workspace: "ws", Automation: "nightly", Tool: "read_file"},
			{At: t0.Add(time.Hour), Kind: KindNewDestination, Workspace: "ws", Automation: "weather", Host: "203.0.113.9"},
		},
		Destinations: map[string]Destinations{"ws/weather": {Since: t0.Add(-10 * 24 * time.Hour), Hosts: []string{"a", "b"}}},
		Alerted:      map[string]time.Time{"ws/nightly/refused": t0},
		SecretsSeen:  map[string]time.Time{"ws/x|y": t0},
	}
	data, _ := json.Marshal(rec)
	f.files[File] = data
	s := New()
	s.now = func() time.Time { return t0 }
	if err := s.Start(t.Context(), f.host()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(t.Context()) //nolint:errcheck // the stack's Stop never fails
	resp, err := s.Handle(t.Context(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath})
	if err != nil || resp.Status != http.StatusOK || resp.ContentType != "application/json" {
		t.Fatalf("resp %+v err %v", resp, err)
	}
	var got view
	if err := json.Unmarshal(resp.Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 2 || got.Events[0].Kind != "new_destination" {
		t.Fatalf("events = %+v; want newest first", got.Events)
	}
	if len(got.Automations) != 1 || got.Automations[0].Hosts != 2 || got.Automations[0].Learning || got.Automations[0].Automation != "weather" {
		t.Fatalf("automations = %+v", got.Automations)
	}
	var raw map[string]any
	_ = json.Unmarshal(resp.Body, &raw)
	if _, ok := raw["alerted"]; ok {
		t.Error("the alert bookkeeping is not shown")
	}
}

func TestHandle_AnUnreadableRecordIsAnErrorNeverAnEmptyRecord(t *testing.T) {
	f := newFakeHost()
	s := New()
	if err := s.Start(t.Context(), f.host()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(t.Context()) //nolint:errcheck // the stack's Stop never fails
	f.readErr = errors.New("disk")
	resp, err := s.Handle(t.Context(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath})
	if err != nil || resp.Status != http.StatusInternalServerError || !strings.Contains(string(resp.Body), "failed to read the agent security record") {
		t.Errorf("resp %+v err %v", resp, err)
	}
}

func TestHandle_AnotherRouteOrAStoppedStackIsRefused(t *testing.T) {
	s := New()
	if _, err := s.Handle(t.Context(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath}); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("before start: %v", err)
	}
	if err := s.Start(t.Context(), newFakeHost().host()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(t.Context()) //nolint:errcheck // the stack's Stop never fails
	for _, req := range []stacksdk.Request{{Method: http.MethodPut, Path: RoutePath}, {Method: http.MethodGet, Path: "/admin/api/other"}} {
		if _, err := s.Handle(t.Context(), req); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
			t.Errorf("%s %s: %v", req.Method, req.Path, err)
		}
	}
	if _, err := s.CallTool(t.Context(), "x", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a tool that is not declared: %v", err)
	}
}

func TestSecrets_AHitIsRecordedByPlaceAndKindAndAlertedOncePerWorkspace(t *testing.T) {
	w, f, _ := newTestWatch()
	f.hits = []stacksdk.SecretHit{
		{Workspace: "ws", Where: "reports/news.md", Kind: "a script token of this program"},
		{Workspace: "other", Where: "a.txt", Kind: "a key"},
		{Workspace: "ws", Where: "notes.md", Kind: "a key"},
	}
	w.scanSecrets(t.Context())
	got := f.record(t).Events
	if len(got) != 3 || got[0].Kind != KindSecretFound || got[0].Where != "reports/news.md" || got[0].Detail != "a script token of this program" {
		t.Fatalf("events = %+v", got)
	}
	wantAlerts := []stacksdk.Alert{{ID: AlertSecrets, Workspace: "ws", Count: 2}, {ID: AlertSecrets, Workspace: "other", Count: 1}}
	if a := f.sent(); len(a) != 2 || a[0] != wantAlerts[0] || a[1] != wantAlerts[1] {
		t.Fatalf("alerts = %+v; want one per workspace in the order first seen, %+v", a, wantAlerts)
	}
	w.scanSecrets(t.Context())
	if len(f.record(t).Events) != 3 || len(f.sent()) != 2 {
		t.Error("a secret already reported is not reported again")
	}
}

func TestSecrets_DoesNothingWhileThePassIsSwitchedOff(t *testing.T) {
	w, f, _ := newTestWatch()
	f.hits = []stacksdk.SecretHit{{Workspace: "ws", Where: "notes.md", Kind: "a key"}}
	f.settings = `{"secrets_pass_off":true}`
	w.scanSecrets(t.Context())
	if f.scans != 0 || len(f.record(t).Events) != 0 || len(f.sent()) != 0 {
		t.Fatalf("the pass ran while switched off: %d scans, %+v", f.scans, f.record(t).Events)
	}
	f.settings = `{}`
	w.scanSecrets(t.Context())
	if len(f.record(t).Events) != 1 {
		t.Fatalf("events = %+v; the pass runs once it is switched on", f.record(t).Events)
	}
}

func TestSecrets_AFailedScanIsReportedAndRecordsNothing(t *testing.T) {
	w, f, _ := newTestWatch()
	f.scanErr = errors.New("walk")
	w.scanSecrets(t.Context())
	if len(f.logged) != 1 || len(f.record(t).Events) != 0 || w.lastRun.Load() != nil {
		t.Errorf("log %q events %+v", f.logged, f.record(t).Events)
	}
}

func TestSecrets_ForgetOldestKeepsTheNewest(t *testing.T) {
	t0 := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	seen := map[string]time.Time{"a": t0, "b": t0.Add(time.Hour), "c": t0.Add(2 * time.Hour)}
	forgetOldest(seen, 2)
	if _, ok := seen["a"]; ok || len(seen) != 2 {
		t.Errorf("seen = %v; want the oldest gone", seen)
	}
}

func TestEvents_TheRecordKeepsTheNewestFiveHundred(t *testing.T) {
	w, f, _ := newTestWatch()
	for range maxEvents + 5 {
		w.handle(t.Context(), stacksdk.Observation{Kind: stacksdk.ObservedApproval, Workspace: "ws", Tool: "write_file"})
	}
	if n := len(f.record(t).Events); n != maxEvents {
		t.Errorf("events = %d, want %d", n, maxEvents)
	}
}

func TestAlertIDs_AreThoseThePassAndTheLoopPublish(t *testing.T) {
	if got := strings.Join(AlertIDs(), ","); got != "agent_tool_refused,agent_new_destination,agent_secrets_found" {
		t.Errorf("alert ids = %s", got)
	}
}
