package app

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/commands"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/db"
	"llm-proxy/models"
	"time"
)

// The registry (core) and the manifest (stack) both describe the Reminders module. The manifest is the source of what a module is, and the registry
// must not drift from it, including the words the module's card shows.
func TestRemindersStack_TheRegistryModuleMatchesTheManifestSpec(t *testing.T) {
	reg := allModules(t, newTestStacks(t).reg)
	specs := remindersStack().Manifest().Modules
	if len(specs) != 1 {
		t.Fatalf("the manifest has %d modules, the registry's Reminders stack one", len(specs))
	}
	spec := specs[0]
	for _, m := range reg {
		if m.ID != spec.ID {
			continue
		}
		if m.Title != spec.Title || m.Purpose != spec.Purpose || m.Layer != spec.Layer || string(m.Sends) != string(spec.Sends) || m.StepUp != spec.StepUp || m.Stack != stack.Reminders {
			t.Errorf("registry %+v differs from manifest %+v", m, spec)
		}
		if m.About.Watches != spec.About.Watches || m.About.Never != spec.About.Never || m.About.ShowsUp != spec.About.ShowsUp || m.About.Settings != spec.About.Settings {
			t.Errorf("the card's words differ: registry %+v, manifest %+v", m.About, spec.About)
		}
		return
	}
	t.Fatal("the Reminders module is not in the registry")
}

// ── a Reminders stack over the real adapters ─────────────────

type memConnectors struct {
	mu    sync.Mutex
	types []string
	sent  []string
}

func (c *memConnectors) Types() []string { return c.types }
func (c *memConnectors) Deliver(_ context.Context, channel, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, channel+":"+text)
	return nil
}

type remindersRig struct {
	port       remindersPort
	host       *stackHost
	policy     *memReminders
	connectors *memConnectors
	dir        string
}

// newRemindersRig hosts the real stack over the real adapters the core builds: the settings document, the data folder, the notify port and the
// schedule port over the one parser automations use. Only the connectors and the settings store are in memory.
func newRemindersRig(t *testing.T, channels ...string) *remindersRig {
	t.Helper()
	return newRemindersRigIn(t, t.TempDir(), nil, channels...)
}

// newRemindersRigIn hosts the stack over the folder dir, with the rows of the old database staged before it starts, as the app does.
func newRemindersRigIn(t *testing.T, dir string, legacy []legacyReminder, channels ...string) *remindersRig {
	t.Helper()
	st := remindersStack()
	policy := &memReminders{}
	conns := &memConnectors{types: channels}
	host, err := newStackHost(st, stacksdk.Host{
		Config:   stackport.Config[models.RemindersConfig]{Store: policy},
		Log:      stackport.Log{Stack: "reminders"},
		DataDir:  stackport.DataDir{Dir: dir, Files: st.Manifest().Storage},
		Notify:   stackport.Notify{Types: conns.Types, Deliver: conns.Deliver},
		Schedule: stackport.Schedule{Parse: func(expr string) (interface{ Next(time.Time) time.Time }, error) { return automation.ParseCron(expr) }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) > 0 {
		if err := stageLegacyReminders(host, legacy); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	host.Run(ctx)
	t.Cleanup(func() { cancel(); _ = st.Stop(context.Background()) })
	return &remindersRig{port: remindersPort{h: host}, host: host, policy: policy, connectors: conns, dir: dir}
}

func TestRemindersStack_OverTheRealGrammarAcceptsWhatAutomationsAcceptAndRefusesTheRest(t *testing.T) {
	r := newRemindersRig(t, "telegram")
	ctx := context.Background()
	got, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "stand-up", Cron: "0 9 * * 1-5", Timezone: "Europe/London"})
	if err != nil || !strings.HasPrefix(got, "Reminder 1 set: repeats (0 9 * * 1-5) starting ") || !strings.HasSuffix(got, "Europe/London via telegram") {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "x", Cron: "@daily", Timezone: "UTC"}); err != nil || !strings.Contains(got, "repeats (@daily)") {
		t.Fatalf("a descriptor, which automations accept, was refused: %q, %v", got, err)
	}
	// Asked for with a cron and no start time (the assistant's way), a schedule that does not parse never fires, as it always said.
	for name, cron := range map[string]string{"seconds are refused, as for an automation": "*/5 * * * * *", "not a schedule": "nope"} {
		if _, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "x", Cron: cron, Timezone: "UTC"}); err == nil || !strings.Contains(err.Error(), "recurrence never fires again") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "x", Cron: "*/5 * * * *", Timezone: "UTC"}); err == nil || !strings.Contains(err.Error(), "more often than every 15 minutes") {
		t.Errorf("too often: %v", err)
	}
	// With a start time (the screen's way) the grammar's own words say what is wrong.
	for name, tc := range map[string]struct{ cron, want string }{
		"seconds": {"*/5 * * * * *", "expected exactly 5 fields, found 6"},
		"words":   {"nope", "expected exactly 5 fields, found 1: [nope]"},
	} {
		resp, err := r.host.stack.Handle(ctx, stacksdk.Request{Method: http.MethodPost, Path: "/admin/api/reminders", ContentType: "application/json",
			Body: []byte(`{"message":"x","fire_at":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `","cron":"` + tc.cron + `","timezone":"UTC"}`)})
		if err != nil || resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), tc.want) {
			t.Errorf("%s: %d %s %v, want 400 saying %q", name, resp.Status, resp.Body, err, tc.want)
		}
	}
	if list, _ := r.port.List(ctx); !strings.Contains(list, "(repeats: 0 9 * * 1-5)") {
		t.Fatalf("list = %q", list)
	}
}

func TestRemindersStack_TheAssistantAndTheCommandReachItAsTheyAlwaysDid(t *testing.T) {
	r := newRemindersRig(t, "telegram")
	ctx := context.Background()
	got, err := r.port.Remind(ctx, commands.RemindRequest{Text: "every weekday 9am stand-up", Zone: "Europe/London", Now: time.Now(), Preview: true})
	if err != nil || !strings.Contains(got, "repeats (0 9 * * 1-5)") || !strings.HasSuffix(got, "stand-up") {
		t.Fatalf("preview = %q, %v", got, err)
	}
	if _, err := r.port.Remind(ctx, commands.RemindRequest{Text: "blah 9am call", Zone: "UTC", Now: time.Now()}); err == nil || !strings.Contains(err.Error(), `"blah"`) {
		t.Fatalf("an unreadable time should name the word: %v", err)
	}
	if got, err := r.port.Cancel(ctx, 99); err == nil {
		t.Fatalf("cancel of nothing = %q, want a refusal", got)
	}
}

func TestRemindersStack_ItsRoutesAnswerAsTheScreenAlwaysReadThem(t *testing.T) {
	r := newRemindersRig(t, "telegram")
	st := r.host.stack
	resp, err := st.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: "/admin/api/reminders"})
	if err != nil || resp.Status != http.StatusOK || resp.ContentType != "application/json" || !strings.Contains(string(resp.Body), `"open":[]`) || !strings.Contains(string(resp.Body), `"recent":[]`) {
		t.Fatalf("%d %s %v", resp.Status, resp.Body, err)
	}
}

// ── the switch ───────────────────────────────────────────────

func TestRemindersSwitch_IsAnOffFlagThatTheSettingsFormNeverChanges(t *testing.T) {
	r := newRemindersRig(t, "telegram")
	sw := remindersSwitch{store: r.policy, host: r.host}
	if !sw.Enabled() {
		t.Fatal("an install that has always had reminders keeps them on")
	}
	if err := sw.Set(false); err != nil || sw.Enabled() || !r.policy.Get().Disabled {
		t.Fatalf("Set(false): %v, %+v", err, r.policy.Get())
	}
	if _, err := r.port.Schedule(context.Background(), tools.ReminderRequest{Message: "x", When: soon(), Timezone: "UTC"}); err == nil || !strings.Contains(err.Error(), "switched off") {
		t.Fatalf("while off: %v, want it to say it is switched off", err)
	}
	// The form saves the owner's zone and connectors; it must not turn the module back on.
	set := models.UserSettings{Reminders: &models.RemindersConfig{Disabled: true, Timezone: "UTC"}}
	applySettingsFields(&set, models.SystemUpdatePayload{Reminders: &models.RemindersConfig{Timezone: "Europe/London", SkipConnectors: []string{"slack"}}})
	if !set.Reminders.Disabled || set.Reminders.Timezone != "Europe/London" || len(set.Reminders.SkipConnectors) != 1 {
		t.Fatalf("settings after the form: %+v", set.Reminders)
	}
	if err := sw.Set(true); err != nil || !sw.Enabled() {
		t.Fatalf("Set(true): %v", err)
	}
}

// ── the reminders the service kept in its main database ──────

const legacySchema = `
CREATE TABLE reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT, message TEXT NOT NULL, fire_at INTEGER NOT NULL, cron TEXT NOT NULL DEFAULT '', until_at INTEGER,
    timezone TEXT NOT NULL DEFAULT '', connector TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'scheduled', attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL,
    rev INTEGER NOT NULL DEFAULT 0, event TEXT NOT NULL DEFAULT '', event_at INTEGER NOT NULL DEFAULT 0);`

func legacyDB(t *testing.T) *sql.DB {
	t.Helper()
	p, err := db.Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.DB().Close() })
	return p.DB()
}

func TestLegacyReminders_AnInstallWithoutTheTableHasNothingToCopy(t *testing.T) {
	rows, err := readLegacyReminders(context.Background(), legacyDB(t))
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}

func TestLegacyReminders_AreCopiedOnceKeepingIdsStatusAndRevision(t *testing.T) {
	old := legacyDB(t)
	if _, err := old.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	fire := time.Now().Add(48 * time.Hour).Unix()
	until := time.Now().Add(96 * time.Hour).Unix()
	for _, q := range []string{
		`INSERT INTO reminders (id, message, fire_at, timezone, connector, status, created_by, created_at, rev) VALUES (7, 'pay rent', ` + unixStr(fire) + `, 'Europe/London', 'telegram', 'scheduled', 'ui', 1, 2)`,
		`INSERT INTO reminders (id, message, fire_at, cron, until_at, timezone, status, attempts, last_error, created_by, created_at, rev, event, event_at) VALUES (9, 'stand-up', ` + unixStr(fire) + `, '0 9 * * 1', ` + unixStr(until) + `, 'UTC', 'paused', 1, 'down', 'assistant', 1, 5, 'failed', 1700000000123)`,
		`INSERT INTO reminders (id, message, fire_at, timezone, status, created_by, created_at) VALUES (11, 'done one', 1700000000, 'UTC', 'done', 'ui', 1)`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := readLegacyReminders(context.Background(), old)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	r := newRemindersRigIn(t, dir, rows, "telegram")
	list, _ := r.port.List(ctx)
	if !strings.Contains(list, `7: "pay rent"`) || !strings.Contains(list, `9: "stand-up"`) || !strings.Contains(list, "(paused)") || strings.Contains(list, "done one") {
		t.Fatalf("the open reminders after the copy: %q", list)
	}
	got, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "new", When: soon(), Timezone: "UTC"})
	if err != nil || !strings.HasPrefix(got, "Reminder 12 set") {
		t.Fatalf("a new reminder after the copy: %q, %v (ids continue after the highest copied)", got, err)
	}
	var n int
	if err := old.QueryRow(`SELECT COUNT(*) FROM reminders`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("the old table is left as it was: %d rows, %v", n, err)
	}
	// A later start over the same folder with the same old rows still staged (the old table is kept) copies nothing again, and does not bring back
	// what the owner deleted in between.
	if _, err := r.port.Cancel(ctx, 7); err != nil {
		t.Fatal(err)
	}
	_ = r.host.stack.Stop(ctx)
	again := newRemindersRigIn(t, dir, rows, "telegram")
	list, _ = again.port.List(ctx)
	if strings.Contains(list, "pay rent") || !strings.Contains(list, `9: "stand-up"`) || !strings.Contains(list, `12: "new"`) {
		t.Fatalf("after a restart: %q (the copy must happen once; a cancelled reminder must not come back)", list)
	}
}

func unixStr(n int64) string { return strconv.FormatInt(n, 10) }

// soon is a local time a few days ahead, in the form the assistant writes it.
func soon() string { return time.Now().UTC().Add(72 * time.Hour).Format("2006-01-02 15:04") }

func TestEraseReminders_EmptiesTheStackAndTheOldTableSoNothingComesBack(t *testing.T) {
	old := legacyDB(t)
	if _, err := old.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO reminders (id, message, fire_at, timezone, status, created_by, created_at) VALUES (7, 'pay rent', ` + unixStr(time.Now().Add(48*time.Hour).Unix()) + `, 'UTC', 'scheduled', 'ui', 1)`); err != nil {
		t.Fatal(err)
	}
	r := newRemindersRig(t, "telegram")
	ctx := context.Background()
	if _, err := r.port.Schedule(ctx, tools.ReminderRequest{Message: "x", When: soon(), Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if err := eraseStackReminders(ctx, r.host); err != nil {
		t.Fatal(err)
	}
	if err := clearLegacyReminders(ctx, old); err != nil {
		t.Fatal(err)
	}
	if list, _ := r.port.List(ctx); list != "No open reminders." {
		t.Fatalf("after the erase: %q", list)
	}
	rows, err := readLegacyReminders(ctx, old)
	if err != nil || len(rows) != 0 {
		t.Fatalf("old rows left: %v %v", rows, err)
	}
	if err := clearLegacyReminders(ctx, legacyDB(t)); err != nil {
		t.Fatalf("a database without the old table has nothing to clear: %v", err)
	}
}
