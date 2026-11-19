package reminders

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// ── the stack's host, in memory ──────────────────────────────

type fakeConfig struct {
	mu  sync.Mutex
	doc string
}

func (c *fakeConfig) Load(context.Context) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.RawMessage(c.doc), nil
}
func (c *fakeConfig) Save(_ context.Context, doc json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.doc = string(doc)
	return nil
}
func (c *fakeConfig) set(doc string) { c.mu.Lock(); c.doc = doc; c.mu.Unlock() }

type fakeNotify struct {
	mu       sync.Mutex
	channels []string
	sends    []sent
}

func (n *fakeNotify) Channels(context.Context) ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.channels...), nil
}
func (n *fakeNotify) Send(_ context.Context, channel, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sends = append(n.sends, sent{channel, text})
	return nil
}
func (n *fakeNotify) all() []sent {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]sent(nil), n.sends...)
}

type fakeLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *fakeLog) Log(_ stacksdk.Level, msg string, fields ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, msg+" "+strings.Join(fields, " "))
}
func (l *fakeLog) text() string { l.mu.Lock(); defer l.mu.Unlock(); return strings.Join(l.lines, "\n") }

type fileDir struct{ dir string }

func (d fileDir) Read(string) ([]byte, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no")
}
func (d fileDir) Write(string, []byte) error       { return nil }
func (d fileDir) Quarantine(string) error          { return nil }
func (d fileDir) Path(name string) (string, error) { return filepath.Join(d.dir, name), nil }

type env struct {
	t      *testing.T
	st     *Stack
	clock  *fakeClock
	cfg    *fakeConfig
	notify *fakeNotify
	log    *fakeLog
}

func newEnv(t *testing.T, doc string, channels ...string) *env {
	t.Helper()
	return newEnvStaged(t, doc, nil, channels...)
}

// newEnvStaged starts the stack with the rows of the old database already staged, as the core does before it starts the stack.
func newEnvStaged(t *testing.T, doc string, legacy []Reminder, channels ...string) *env {
	t.Helper()
	e := &env{t: t, clock: &fakeClock{now: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}, cfg: &fakeConfig{doc: doc}, notify: &fakeNotify{channels: channels}, log: &fakeLog{}}
	e.st = NewWithClock(e.clock)
	e.st.StageLegacy(legacy)
	if err := e.st.Start(context.Background(), stacksdk.Host{Config: e.cfg, Log: e.log, DataDir: fileDir{t.TempDir()}, Notify: e.notify, Schedule: testSchedule}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.st.Stop(context.Background()) })
	return e
}

func (e *env) do(method, path, body string, params map[string]string) (int, map[string]any) {
	e.t.Helper()
	req := stacksdk.Request{Method: method, Path: path, Params: params, ContentType: "application/json", Body: []byte(body)}
	if q := strings.Index(path, "?"); q >= 0 {
		req.Path, req.Query = path[:q], path[q+1:]
	}
	resp, err := e.st.Handle(context.Background(), req)
	if err != nil {
		e.t.Fatalf("Handle %s %s: %v", method, path, err)
	}
	var out map[string]any
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &out)
	}
	return resp.Status, out
}

func (e *env) tool(name string, args any) (string, error) {
	e.t.Helper()
	raw, _ := json.Marshal(args)
	out, err := e.st.CallTool(context.Background(), name, raw)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(out, &s); err != nil {
		e.t.Fatalf("tool %s answered %s, not a string: %v", name, out, err)
	}
	return s, nil
}

func codeOf(err error) stacksdk.Code { return stacksdk.CodeOf(err) }

// ── the manifest ─────────────────────────────────────────────

func TestStack_ManifestIsValidAndItsPortsArePinned(t *testing.T) {
	m := New().Manifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	want := []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortNotify, stacksdk.PortSchedule}
	if strings.Join(m.Requires, ",") != strings.Join(want, ",") {
		t.Errorf("requires = %v, want exactly %v: no dial, no fetch, no program, no vault, no key", m.Requires, want)
	}
	if m.Settings.Key != "reminders" || len(m.Storage) != 1 || m.Storage[0] != "reminders.db" {
		t.Errorf("settings key %q and storage %v must be the names the owner's install uses", m.Settings.Key, m.Storage)
	}
	if len(m.Modules) != 1 || m.Modules[0].Sends != stacksdk.SendsMessage {
		t.Errorf("one module that says it sends a message, got %+v", m.Modules)
	}
}

func TestStack_RoutesAreTheOnesTheScreenAlwaysRead(t *testing.T) {
	var got []string
	for _, r := range New().Manifest().Routes {
		got = append(got, r.Method+" "+r.Path)
	}
	want := []string{
		"GET /admin/api/reminders", "GET /admin/api/reminders/events", "POST /admin/api/reminders", "PUT /admin/api/reminders/{id}",
		"DELETE /admin/api/reminders/{id}", "POST /admin/api/reminders/{id}/pause", "POST /admin/api/reminders/{id}/resume", "POST /admin/api/reminders/{id}/skip",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("routes = %v, want %v", got, want)
	}
}

// ── the routes ───────────────────────────────────────────────

func TestAPI_CreateThenListShowsWhatTheScreenNeeds(t *testing.T) {
	e := newEnv(t, "", "telegram")
	status, body := e.do("POST", "/admin/api/reminders", `{"message":"call bank","fire_at":"2026-10-12T09:00:00Z","timezone":"Europe/London"}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, body)
	}
	for k, want := range map[string]any{"message": "call bank", "status": "scheduled", "timezone": "Europe/London", "channel": "telegram", "connector": "", "fire_at": "2026-10-12T09:00:00Z"} {
		if body[k] != want {
			t.Errorf("created %s = %v, want %v", k, body[k], want)
		}
	}
	status, list := e.do("GET", "/admin/api/reminders", "", nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d", status)
	}
	open, _ := list["open"].([]any)
	recent, ok := list["recent"].([]any)
	if len(open) != 1 || !ok || len(recent) != 0 {
		t.Fatalf("open %v and recent %v: a list with nothing finished must be an empty array, not null", open, list["recent"])
	}
}

func TestAPI_EmptyListsAreArrays(t *testing.T) {
	e := newEnv(t, "")
	_, list := e.do("GET", "/admin/api/reminders", "", nil)
	if open, ok := list["open"].([]any); !ok || len(open) != 0 {
		t.Fatalf("open = %#v, want []", list["open"])
	}
}

func TestAPI_CreateErrors(t *testing.T) {
	e := newEnv(t, "")
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"bad json":        {`{`, http.StatusBadRequest},
		"bad fire_at":     {`{"message":"x","fire_at":"tomorrow"}`, http.StatusBadRequest},
		"past":            {`{"message":"x","fire_at":"2026-10-11T09:00:00Z"}`, http.StatusBadRequest},
		"empty message":   {`{"message":" ","fire_at":"2026-10-12T09:00:00Z"}`, http.StatusBadRequest},
		"unknown zone":    {`{"message":"x","fire_at":"2026-10-12T09:00:00Z","timezone":"Mars/Base"}`, http.StatusBadRequest},
		"unknown channel": {`{"message":"x","fire_at":"2026-10-12T09:00:00Z","connector":"nope"}`, http.StatusBadRequest},
	} {
		if status, body := e.do("POST", "/admin/api/reminders", tc.body, nil); status != tc.want || body["error"] == nil {
			t.Errorf("%s: %d %v, want %d with an error", name, status, body, tc.want)
		}
	}
	resp, _ := e.st.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: "/admin/api/reminders", ContentType: "text/plain", Body: []byte("{}")})
	if resp.Status != http.StatusUnsupportedMediaType {
		t.Errorf("a body that is not application/json = %d, want 415", resp.Status)
	}
}

func TestAPI_EditPauseResumeSkipDelete(t *testing.T) {
	e := newEnv(t, "")
	_, c := e.do("POST", "/admin/api/reminders", `{"message":"standup","cron":"0 9 * * 1-5","timezone":"UTC"}`, nil)
	id := int64(c["id"].(float64))
	p := map[string]string{"id": itoa(id)}
	item := "/admin/api/reminders/" + itoa(id)

	if status, b := e.do("PUT", item, `{"message":"standup!","cron":"0 10 * * 1-5"}`, p); status != http.StatusOK || b["message"] != "standup!" || b["cron"] != "0 10 * * 1-5" {
		t.Fatalf("edit = %d %v", status, b)
	}
	if status, b := e.do("POST", item+"/pause", "", p); status != http.StatusOK || b["status"] != "paused" {
		t.Fatalf("pause = %d %v", status, b)
	}
	if status, b := e.do("POST", item+"/resume", "", p); status != http.StatusOK || b["status"] != "scheduled" {
		t.Fatalf("resume = %d %v", status, b)
	}
	if status, b := e.do("POST", item+"/skip", "", p); status != http.StatusOK || b["status"] != "scheduled" {
		t.Fatalf("skip = %d %v", status, b)
	}
	if status, _ := e.do("DELETE", item, "", p); status != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", status)
	}
	if status, b := e.do("DELETE", item, "", p); status != http.StatusNotFound || b["error"] != "reminder not found" {
		t.Fatalf("delete again = %d %v, want 404", status, b)
	}
	if status, b := e.do("PUT", "/admin/api/reminders/x", `{}`, map[string]string{"id": "x"}); status != http.StatusBadRequest || b["error"] != "invalid id" {
		t.Fatalf("a bad id = %d %v", status, b)
	}
}

func TestAPI_EventsAreFetchedByCursorAndOnlyAlertTheScreenWhenTheyShould(t *testing.T) {
	e := newEnv(t, "", "telegram")
	e.do("POST", "/admin/api/reminders", `{"message":"tea","fire_at":"2026-10-12T08:30:00Z"}`, nil)
	_, first := e.do("GET", "/admin/api/reminders/events", "", nil)
	if first["cursor"].(float64) != 0 || len(first["items"].([]any)) != 0 {
		t.Fatalf("with nothing fired: %v", first)
	}
	e.advance(40 * time.Minute)
	waitFor(t, func() bool { return len(e.notify.all()) == 1 })
	_, after0 := e.do("GET", "/admin/api/reminders/events?after=0", "", nil)
	items, _ := after0["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["message"] != "tea" {
		t.Fatalf("events after 0 = %v", after0)
	}
	if status, b := e.do("GET", "/admin/api/reminders/events?after=nope", "", nil); status != http.StatusBadRequest || b["error"] == nil {
		t.Fatalf("a bad cursor = %d %v", status, b)
	}
}

// advance moves the clock once the delivery loop has armed its timer for what was just created, as the service's own tests do.
func (e *env) advance(d time.Duration) {
	e.t.Helper()
	waitFor(e.t, e.clock.armed)
	e.clock.Advance(d)
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}

// ── delivery ─────────────────────────────────────────────────

func TestDelivery_TheOwnersWordsAreSentUnchangedAndNeverLogged(t *testing.T) {
	e := newEnv(t, "", "telegram")
	secret := "ring Dr Smith about the biopsy <b>not markup</b>"
	e.do("POST", "/admin/api/reminders", `{"message":"`+secret+`","fire_at":"2026-10-12T09:00:00Z"}`, nil)
	e.advance(time.Hour)
	waitFor(t, func() bool { return len(e.notify.all()) == 1 })
	if got := e.notify.all()[0]; got.connector != "telegram" || got.message != secret {
		t.Fatalf("sent %+v, want the text unchanged to telegram", got)
	}
	if strings.Contains(e.log.text(), "biopsy") {
		t.Fatalf("the reminder's text reached the log: %s", e.log.text())
	}
}

func TestDelivery_SkippedConnectorsAreNotUsedAndReadLive(t *testing.T) {
	e := newEnv(t, `{"skip_connectors":["slack"]}`, "telegram", "slack")
	e.do("POST", "/admin/api/reminders", `{"message":"x","fire_at":"2026-10-12T09:00:00Z"}`, nil)
	e.advance(time.Hour)
	waitFor(t, func() bool { return len(e.notify.all()) == 1 })
	if got := e.notify.all(); len(got) != 1 || got[0].connector != "telegram" {
		t.Fatalf("sends = %+v, want telegram only", got)
	}
}

// ── the switch ───────────────────────────────────────────────

func TestSwitchedOff_RefusesChangesDeliversNothingAndCatchesUpWhenOn(t *testing.T) {
	e := newEnv(t, "", "telegram")
	e.do("POST", "/admin/api/reminders", `{"message":"x","fire_at":"2026-10-12T09:00:00Z"}`, nil)
	e.cfg.set(`{"disabled":true}`)
	_ = e.st.ConfigChanged(context.Background())

	if status, b := e.do("POST", "/admin/api/reminders", `{"message":"y","fire_at":"2026-10-12T10:00:00Z"}`, nil); status != http.StatusConflict || !strings.Contains(b["error"].(string), "switched off") {
		t.Fatalf("create while off = %d %v, want 409 saying it is switched off", status, b)
	}
	if _, err := e.tool(ToolSchedule, ScheduleArgs{Message: "z", When: "2026-10-12 11:00"}); codeOf(err) != stacksdk.CodeUnavailable {
		t.Fatalf("the tool while off = %v, want unavailable", err)
	}
	e.clock.Advance(2 * time.Hour) // past the fire time by a clear margin, so it is late and not on time
	time.Sleep(50 * time.Millisecond)
	if n := len(e.notify.all()); n != 0 {
		t.Fatalf("%d sent while off", n)
	}
	if st, _ := e.st.Status(context.Background()); len(st) != 1 || st[0].Enabled {
		t.Fatalf("status = %+v, want the module off", st)
	}

	e.cfg.set("")
	_ = e.st.ConfigChanged(context.Background())
	waitFor(t, func() bool { return len(e.notify.all()) == 1 })
	if got := e.notify.all()[0].message; got != "(late) x" {
		t.Fatalf("after switching on it sent %q, want the late form", got)
	}
}

// ── the tools ────────────────────────────────────────────────

func TestTool_ScheduleEchoesTimeZoneAndChannel(t *testing.T) {
	e := newEnv(t, `{"timezone":"Europe/London"}`, "telegram")
	got, err := e.tool(ToolSchedule, ScheduleArgs{Message: "call bank", When: "2026-10-12 20:00"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Reminder 1 set for Mon 12 Oct 2026 20:00 Europe/London via telegram"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTool_ScheduleUsesTheConversationZoneThenTheNamedOne(t *testing.T) {
	e := newEnv(t, "")
	if got, _ := e.tool(ToolSchedule, ScheduleArgs{Message: "a", When: "2026-10-12 20:00", ChatZone: "Asia/Tokyo"}); !strings.Contains(got, "Asia/Tokyo") {
		t.Errorf("chat zone not used: %q", got)
	}
	if got, _ := e.tool(ToolSchedule, ScheduleArgs{Message: "b", When: "2026-10-12 20:00", ChatZone: "Asia/Tokyo", Timezone: "Europe/Paris"}); !strings.Contains(got, "Europe/Paris") {
		t.Errorf("a zone the user named must win: %q", got)
	}
}

func TestTool_ScheduleRecurringNamedConnectorAndOrigin(t *testing.T) {
	e := newEnv(t, "", "telegram", "slack")
	got, err := e.tool(ToolSchedule, ScheduleArgs{Message: "standup", Cron: "0 9 * * 1", Connector: " Slack "})
	if err != nil || !strings.HasPrefix(got, "Reminder 1 set: repeats (0 9 * * 1) starting ") || !strings.HasSuffix(got, "via slack") {
		t.Fatalf("got %q, %v", got, err)
	}
	got, _ = e.tool(ToolSchedule, ScheduleArgs{Message: "from telegram", When: "2026-10-12 20:00", Origin: "telegram"})
	if !strings.HasSuffix(got, "via telegram") {
		t.Fatalf("a reminder asked for through telegram replies there: %q", got)
	}
}

func TestTool_ScheduleInputErrorsSayWhatToFix(t *testing.T) {
	e := newEnv(t, "")
	for name, tc := range map[string]struct {
		args ScheduleArgs
		want string
	}{
		"no message": {ScheduleArgs{When: "2026-10-12 20:00"}, "message is required"},
		"neither":    {ScheduleArgs{Message: "x"}, "give exactly one of 'when'"},
		"both":       {ScheduleArgs{Message: "x", When: "2026-10-12 20:00", Cron: "0 9 * * *"}, "give exactly one of 'when'"},
		"bad when":   {ScheduleArgs{Message: "x", When: "soon"}, "when must be a local date and time"},
		"bad until":  {ScheduleArgs{Message: "x", Cron: "0 9 * * *", Until: "later"}, "until must be a local date and time"},
		"bad zone":   {ScheduleArgs{Message: "x", When: "2026-10-12 20:00", Timezone: "Mars/Base"}, "unknown timezone"},
		"too often":  {ScheduleArgs{Message: "x", Cron: "*/5 * * * *"}, "more often than every 15 minutes"},
	} {
		if _, err := e.tool(ToolSchedule, tc.args); err == nil || codeOf(err) != stacksdk.CodeInvalid || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want invalid containing %q", name, err, tc.want)
		}
	}
}

func TestTool_ListAndCancel(t *testing.T) {
	e := newEnv(t, "", "telegram")
	if got, _ := e.tool(ToolList, struct{}{}); got != "No open reminders." {
		t.Fatalf("empty list = %q", got)
	}
	e.tool(ToolSchedule, ScheduleArgs{Message: "tea", When: "2026-10-12 20:00", Timezone: "UTC"})
	e.tool(ToolSchedule, ScheduleArgs{Message: "standup", Cron: "0 9 * * 1", Timezone: "UTC"})
	got, _ := e.tool(ToolList, struct{}{})
	if !strings.Contains(got, `1: "tea" at Mon 12 Oct 2026 20:00 UTC via telegram`) || !strings.Contains(got, "(repeats: 0 9 * * 1)") {
		t.Fatalf("list = %q", got)
	}
	if got, err := e.tool(ToolCancel, CancelArgs{ID: 1}); err != nil || got != "Reminder 1 cancelled." {
		t.Fatalf("cancel = %q, %v", got, err)
	}
	if _, err := e.tool(ToolCancel, CancelArgs{ID: 1}); codeOf(err) != stacksdk.CodeNotFound {
		t.Fatalf("cancel again = %v, want not found", err)
	}
	if _, err := e.st.CallTool(context.Background(), "nope", nil); codeOf(err) != stacksdk.CodeNotFound {
		t.Fatalf("an unknown tool = %v", err)
	}
}

// ── /remind ──────────────────────────────────────────────────

func TestCommand_SetsInThePersonsZoneAndPreviewStoresNothing(t *testing.T) {
	e := newEnv(t, "", "telegram")
	now := e.clock.Now()
	args := CommandArgs{Text: "in 2h check the oven", Zone: "Europe/London", Now: now}
	args.Preview = true
	got, err := e.tool(ToolCommand, args)
	if err != nil || !strings.Contains(got, "check the oven") || strings.HasPrefix(got, "Reminder") {
		t.Fatalf("preview = %q, %v", got, err)
	}
	if list, _ := e.tool(ToolList, struct{}{}); list != "No open reminders." {
		t.Fatalf("a preview stored something: %q", list)
	}
	args.Preview = false
	got, err = e.tool(ToolCommand, args)
	if err != nil || !strings.HasPrefix(got, "Reminder 1 set for ") || !strings.Contains(got, "Europe/London via telegram: check the oven") {
		t.Fatalf("set = %q, %v", got, err)
	}
}

func TestCommand_UnreadableAndRefusedReadPlainly(t *testing.T) {
	e := newEnv(t, "")
	if _, err := e.tool(ToolCommand, CommandArgs{Text: "blah", Zone: "UTC", Now: e.clock.Now()}); codeOf(err) != stacksdk.CodeInvalid {
		t.Fatalf("unreadable = %v, want invalid", err)
	}
	_, err := e.tool(ToolCommand, CommandArgs{Text: "every 5 minutes x", Zone: "UTC", Now: e.clock.Now()})
	if err == nil || strings.Contains(err.Error(), "invalid reminder: ") {
		t.Fatalf("a refusal should read plainly, without the package prefix: %v", err)
	}
}

// ── the rows from the old database ───────────────────────────

func legacyRows(e *env) []Reminder {
	at := e.clock.Now().Add(time.Hour)
	until := at.Add(48 * time.Hour)
	return []Reminder{
		{ID: 7, Message: "pay rent", FireAt: at, Timezone: "Europe/London", Status: StatusScheduled, Rev: 2, CreatedBy: "ui", CreatedAt: at.Add(-time.Hour)},
		{ID: 9, Message: "standup", FireAt: at, Cron: "0 9 * * 1", Until: &until, Timezone: "UTC", Status: StatusPaused, Rev: 5, Attempts: 1, LastError: "x", CreatedBy: "assistant", CreatedAt: at.Add(-2 * time.Hour), Event: "delivered", EventAt: at.Add(-time.Minute)},
	}
}

func TestStageLegacy_RowsKeepIdsStatusAndRevisionAndArriveBeforeAnythingCanBeSet(t *testing.T) {
	probe := &env{clock: &fakeClock{now: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}}
	e := newEnvStaged(t, "", legacyRows(probe))
	_, list := e.do("GET", "/admin/api/reminders", "", nil)
	open := list["open"].([]any)
	if len(open) != 2 {
		t.Fatalf("open = %v: the copy must be done before the first request is answered", open)
	}
	byID := map[float64]map[string]any{}
	for _, o := range open {
		m := o.(map[string]any)
		byID[m["id"].(float64)] = m
	}
	if byID[7]["message"] != "pay rent" || byID[9]["status"] != "paused" || byID[9]["cron"] != "0 9 * * 1" || byID[9]["until"] == nil {
		t.Fatalf("imported rows changed: %v", byID)
	}
	_, made := e.do("POST", "/admin/api/reminders", `{"message":"new","fire_at":"2026-10-12T12:00:00Z"}`, nil)
	if made["id"].(float64) <= 9 {
		t.Fatalf("a new reminder got id %v: ids must continue after the imported ones", made["id"])
	}
	got, _ := e.st.store.Get(context.Background(), 9)
	if got.Rev != 5 || got.Attempts != 1 || got.Event != "delivered" {
		t.Fatalf("revision, attempts and outcome must survive: %+v", got)
	}
}

// A second start with the same old rows staged copies nothing: the marker lives in the stack's file.
func TestStageLegacy_AStartAfterTheCopyDoesNotCopyAgain(t *testing.T) {
	probe := &env{clock: &fakeClock{now: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}}
	rows := legacyRows(probe)
	dir := t.TempDir()
	start := func() *Stack {
		st := NewWithClock(probe.clock)
		st.StageLegacy(rows)
		if err := st.Start(context.Background(), stacksdk.Host{Config: &fakeConfig{}, Log: &fakeLog{}, DataDir: fileDir{dir}, Notify: &fakeNotify{}, Schedule: testSchedule}); err != nil {
			t.Fatal(err)
		}
		return st
	}
	first := start()
	if err := first.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := start()
	defer second.Stop(context.Background())
	open, err := second.store.ListOpen(context.Background())
	if err != nil || len(open) != 2 {
		t.Fatalf("after a second start: %v, %v (the rows must not be copied twice)", open, err)
	}
}

// If the file already holds a reminder under an id the old rows use, the old reminder is kept under a new id and is not lost.
func TestImportLegacy_ATakenIdKeepsTheOldReminderUnderANewOne(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Add(ctx, Reminder{Message: "set first", FireAt: base.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	n, err := s.ImportLegacy(ctx, []Reminder{{ID: 1, Message: "from the old database", FireAt: base.Add(2 * time.Hour), Status: StatusScheduled, CreatedAt: base}})
	if err != nil || n != 1 {
		t.Fatalf("import = %d, %v: a taken id must not stop the copy", n, err)
	}
	open, _ := s.ListOpen(ctx)
	if len(open) != 2 || open[0].Message != "set first" || open[1].Message != "from the old database" || open[1].ID == 1 {
		t.Fatalf("open = %+v, want both reminders, the old one under a new id", open)
	}
}

// ── the switch does not spin the loop ────────────────────────

// While the module is off a reminder that fell due is overdue: a timer for it would fire at once and the loop would run a query in a tight circle.
func TestSwitchedOff_ArmsNoTimerForAnOverdueReminder(t *testing.T) {
	e := newEnv(t, "", "telegram")
	e.do("POST", "/admin/api/reminders", `{"message":"x","fire_at":"2026-10-12T09:00:00Z"}`, nil)
	waitFor(t, e.clock.armed)
	e.cfg.set(`{"disabled":true}`)
	_ = e.st.ConfigChanged(context.Background())
	e.clock.Advance(2 * time.Hour)
	time.Sleep(100 * time.Millisecond) // the loop has been woken and has settled
	if e.clock.armed() {
		t.Fatal("a timer is armed while the module is off: an overdue reminder would fire it at once and the loop would spin")
	}
	e.cfg.set("")
	_ = e.st.ConfigChanged(context.Background())
	waitFor(t, func() bool { return len(e.notify.all()) == 1 })
}

func TestEraseAll_RemovesEveryReminderAndLeavesTheCopyMarkerSoNothingComesBack(t *testing.T) {
	probe := &env{clock: &fakeClock{now: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}}
	e := newEnvStaged(t, "", legacyRows(probe), "telegram")
	e.do("POST", "/admin/api/reminders", `{"message":"x","fire_at":"2026-10-12T12:00:00Z"}`, nil)
	if err := e.st.EraseAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.tool(ToolList, struct{}{}); list != "No open reminders." {
		t.Fatalf("after erase: %q", list)
	}
	if n, err := e.st.store.ImportLegacy(context.Background(), legacyRows(probe)); err != nil || n != 0 {
		t.Fatalf("the old rows must not be copied back after an erase: %d, %v", n, err)
	}
}
