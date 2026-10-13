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

func TestReminderDeliverer(t *testing.T) {
	comm, conns := commWith("telegram")
	d := reminderDeliverer{comm: comm}
	if got := d.Connectors(); !slices.Equal(got, []string{"telegram"}) {
		t.Fatalf("Connectors() = %v, want the configured types", got)
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
	cfg := func() models.RemindersConfig {
		return models.RemindersConfig{Timezone: "Europe/London"}
	}
	svc := reminders.NewService(store, reminderDeliverer{comm: comm}, reminderNotice{}, reminders.SystemClock(),
		reminders.Config{Skipped: func() []string { return cfg().SkipConnectors }})
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

// A connector that is enabled gets reminders until it is ticked off; a reminder
// that names no channel then goes to the web UI only.
func TestReminderPort_TickedOffConnectorMeansInAppOnly(t *testing.T) {
	p, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.DB().Close() })
	store, _ := reminders.NewStore(p)
	comm, _ := commWith("telegram")
	cfg := func() models.RemindersConfig { return models.RemindersConfig{SkipConnectors: []string{"telegram"}} }
	svc := reminders.NewService(store, reminderDeliverer{comm: comm}, reminderNotice{}, reminders.SystemClock(),
		reminders.Config{Skipped: func() []string { return cfg().SkipConnectors }})
	port := reminderPort{svc: svc, settings: cfg}

	view, err := port.Schedule(context.Background(), tools.ReminderSpec{Message: "a", At: time.Now().Add(time.Hour), Zone: "UTC"})
	if err != nil || view.Channel != reminders.InAppChannel {
		t.Fatalf("channel = %q err=%v, want in-app only", view.Channel, err)
	}
	view, _ = port.Schedule(context.Background(), tools.ReminderSpec{Message: "b", At: time.Now().Add(time.Hour), Zone: "UTC", Origin: "telegram"})
	if view.Channel != "telegram" {
		t.Errorf("asked through telegram: channel = %q, want telegram", view.Channel)
	}
}
