package app

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/db"
	"llm-proxy/models"
)

type sinkConnector struct{ sent []string }

func (c *sinkConnector) Send(_ context.Context, m string) error {
	c.sent = append(c.sent, m)
	return nil
}
func (c *sinkConnector) Name() string { return "rec" }

func commWith(types ...string) (*tools.CommunicationTools, map[string]*sinkConnector) {
	comm := tools.NewCommunicationTools()
	conns := map[string]*sinkConnector{}
	for _, ty := range types {
		conns[ty] = &sinkConnector{}
		comm.AddConnector("my-"+ty, ty, conns[ty])
	}
	return comm, conns
}

func TestDefaultReminderConnector(t *testing.T) {
	cases := []struct {
		name    string
		setting string
		types   []string
		want    string
	}{
		{"explicit setting wins", "slack", []string{"telegram", "slack"}, "slack"},
		{"the only connector", "", []string{"discord"}, "discord"},
		{"several and no setting: no guess", "", []string{"slack", "telegram"}, ""},
		{"none configured", "", nil, ""},
		{"setting names an unconfigured type: the only one", "slack", []string{"telegram"}, "telegram"},
		{"setting names an unconfigured type among several: no guess", "slack", []string{"telegram", "discord"}, ""},
	}
	for _, tc := range cases {
		comm, _ := commWith(tc.types...)
		got := defaultReminderConnector(comm, func() models.RemindersConfig {
			return models.RemindersConfig{DefaultConnector: tc.setting}
		})
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestReminderDeliverer(t *testing.T) {
	comm, conns := commWith("telegram")
	d := reminderDeliverer{comm: comm}
	if !d.HasConnector("telegram") || d.HasConnector("slack") {
		t.Fatal("HasConnector must reflect the configured types")
	}
	if err := d.Deliver(context.Background(), "telegram", "hello"); err != nil || !slices.Equal(conns["telegram"].sent, []string{"hello"}) {
		t.Fatalf("err=%v sent=%v", err, conns["telegram"].sent)
	}
}

func TestReminderPort_ScheduleListCancel(t *testing.T) {
	p, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.DB().Close() })
	store, err := reminders.NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	comm, _ := commWith("telegram")
	cfg := func() models.RemindersConfig { return models.RemindersConfig{Timezone: "Europe/London"} }
	svc := reminders.NewService(store, reminderDeliverer{comm: comm}, reminderNotice{}, reminders.SystemClock(),
		reminders.Config{DefaultConnector: func() string { return defaultReminderConnector(comm, cfg) }})
	port := reminderPort{svc: svc, settings: cfg}

	loc, err := port.Zone("")
	if err != nil || loc.String() != "Europe/London" {
		t.Fatalf("owner zone fallback: %v %v", loc, err)
	}
	if _, err := port.Zone("Mars/Base"); err == nil {
		t.Fatal("invalid zone must error")
	}

	at := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	view, err := port.Schedule(context.Background(), tools.ReminderSpec{Message: "call bank", At: at, Zone: "Europe/London"})
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if view.ID == 0 || view.Channel != "telegram" || view.Zone != "Europe/London" || !view.At.Equal(at) {
		t.Fatalf("view = %+v", view)
	}
	list, err := port.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != view.ID {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := port.Cancel(context.Background(), view.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := port.List(context.Background()); len(list) != 0 {
		t.Fatalf("after cancel: %+v", list)
	}
}

// A reminder asked for in a chat app goes back to that app, whatever the default is.
func TestReminderPort_RepliesWhereAsked(t *testing.T) {
	p, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.DB().Close() })
	store, _ := reminders.NewStore(p)
	comm, _ := commWith("telegram", "slack")
	cfg := func() models.RemindersConfig { return models.RemindersConfig{DefaultConnector: "telegram"} }
	svc := reminders.NewService(store, reminderDeliverer{comm: comm}, reminderNotice{}, reminders.SystemClock(),
		reminders.Config{DefaultConnector: func() string { return defaultReminderConnector(comm, cfg) }})
	port := reminderPort{svc: svc, settings: cfg}
	at := time.Now().Add(time.Hour)

	view, err := port.Schedule(context.Background(), tools.ReminderSpec{Message: "a", At: at, Zone: "UTC", Origin: "slack"})
	if err != nil || view.Channel != "slack" {
		t.Fatalf("asked in slack: channel=%q err=%v, want slack", view.Channel, err)
	}
	view, _ = port.Schedule(context.Background(), tools.ReminderSpec{Message: "b", At: at, Zone: "UTC", Origin: "slack", Connector: "telegram"})
	if view.Channel != "telegram" {
		t.Errorf("a named channel beats the origin, got %q", view.Channel)
	}
	view, _ = port.Schedule(context.Background(), tools.ReminderSpec{Message: "c", At: at, Zone: "UTC", Origin: "discord"})
	if view.Channel != "telegram" {
		t.Errorf("an origin that is no longer configured falls back to the default, got %q", view.Channel)
	}
	view, _ = port.Schedule(context.Background(), tools.ReminderSpec{Message: "d", At: at, Zone: "UTC"})
	if view.Channel != "telegram" {
		t.Errorf("web chat uses the default, got %q", view.Channel)
	}
}
