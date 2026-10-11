package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeReminderService struct {
	specs   []ReminderSpec
	cancels []int64
	views   []ReminderView
	err     error
}

func (f *fakeReminderService) Schedule(_ context.Context, s ReminderSpec) (ReminderView, error) {
	if f.err != nil {
		return ReminderView{}, f.err
	}
	f.specs = append(f.specs, s)
	return ReminderView{ID: 7, Message: s.Message, At: s.At, Cron: s.Cron, Zone: s.Zone, Channel: channelOrDefault(s.Connector)}, nil
}

func channelOrDefault(c string) string {
	if c == "" {
		return "telegram"
	}
	return c
}

func (f *fakeReminderService) Cancel(_ context.Context, id int64) error {
	if f.err != nil {
		return f.err
	}
	f.cancels = append(f.cancels, id)
	return nil
}

func (f *fakeReminderService) List(context.Context) ([]ReminderView, error) { return f.views, f.err }

func (f *fakeReminderService) Zone(requested string) (*time.Location, error) {
	if requested == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(requested)
}

func TestScheduleReminder_EchoesAbsoluteTimeZoneChannel(t *testing.T) {
	svc := &fakeReminderService{}
	got, err := NewReminderTools(svc).Schedule(context.Background(), ScheduleReminderArgs{
		Message: "Call the bank", When: "2026-10-12T09:00:00+01:00", Timezone: "Europe/London"})
	if err != nil {
		t.Fatal(err)
	}
	out := got.(string)
	for _, want := range []string{"Mon 12 Oct 2026 09:00", "Europe/London", "telegram", "7"} {
		if !strings.Contains(out, want) {
			t.Errorf("result %q missing %q", out, want)
		}
	}
	if len(svc.specs) != 1 || !svc.specs[0].At.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("spec = %+v, want 08:00Z", svc.specs)
	}
}

func TestScheduleReminder_NamedConnectorLowercased(t *testing.T) {
	svc := &fakeReminderService{}
	_, err := NewReminderTools(svc).Schedule(context.Background(), ScheduleReminderArgs{
		Message: "x", When: "2026-10-12T09:00:00Z", Connector: " Telegram "})
	if err != nil || svc.specs[0].Connector != "telegram" {
		t.Fatalf("err=%v spec=%+v", err, svc.specs)
	}
}

func TestScheduleReminder_Recurring(t *testing.T) {
	svc := &fakeReminderService{}
	got, err := NewReminderTools(svc).Schedule(context.Background(), ScheduleReminderArgs{
		Message: "standup", Cron: "0 9 * * 1", Timezone: "Europe/London", Until: "2027-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.(string), "0 9 * * 1") {
		t.Errorf("result should state the recurrence: %q", got)
	}
	if svc.specs[0].Until == nil {
		t.Error("until not passed through")
	}
}

func TestScheduleReminder_InputErrors(t *testing.T) {
	cases := map[string]ScheduleReminderArgs{
		"blank message":      {Message: " ", When: "2026-10-12T09:00:00Z"},
		"neither when/cron":  {Message: "x"},
		"both when and cron": {Message: "x", When: "2026-10-12T09:00:00Z", Cron: "0 9 * * *"},
		"bad when":           {Message: "x", When: "tomorrow morning"},
		"bad until":          {Message: "x", Cron: "0 9 * * *", Until: "soon"},
		"bad timezone":       {Message: "x", When: "2026-10-12T09:00:00Z", Timezone: "Mars/Base"},
	}
	for name, args := range cases {
		svc := &fakeReminderService{}
		if _, err := NewReminderTools(svc).Schedule(context.Background(), args); err == nil {
			t.Errorf("%s: expected error", name)
		}
		if len(svc.specs) != 0 {
			t.Errorf("%s: nothing should be scheduled", name)
		}
	}
}

func TestScheduleReminder_ServiceErrorSurfaces(t *testing.T) {
	svc := &fakeReminderService{err: errors.New("reminder time is in the past")}
	_, err := NewReminderTools(svc).Schedule(context.Background(), ScheduleReminderArgs{
		Message: "x", When: "2020-01-01T00:00:00Z"})
	if err == nil || !strings.Contains(err.Error(), "in the past") {
		t.Fatalf("err = %v", err)
	}
}

func TestListReminders_FormatsOpenOnly(t *testing.T) {
	svc := &fakeReminderService{views: []ReminderView{
		{ID: 3, Message: "Call bank", At: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC), Zone: "Europe/London", Channel: "telegram"},
		{ID: 4, Message: "Standup", At: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC), Cron: "0 9 * * 1", Zone: "Europe/London", Channel: "in-app"},
	}}
	got, err := NewReminderTools(svc).List(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	out := got.(string)
	for _, want := range []string{"3", "Call bank", "09:00", "4", "Standup", "0 9 * * 1", "in-app"} {
		if !strings.Contains(out, want) {
			t.Errorf("list %q missing %q", out, want)
		}
	}
	empty, _ := NewReminderTools(&fakeReminderService{}).List(context.Background(), struct{}{})
	if !strings.Contains(empty.(string), "No open reminders") {
		t.Errorf("empty list = %q", empty)
	}
}

func TestCancelReminder(t *testing.T) {
	svc := &fakeReminderService{}
	if _, err := NewReminderTools(svc).Cancel(context.Background(), CancelReminderArgs{ID: 9}); err != nil || svc.cancels[0] != 9 {
		t.Fatalf("err=%v cancels=%v", err, svc.cancels)
	}
	bad := &fakeReminderService{err: errors.New("reminder not found")}
	if _, err := NewReminderTools(bad).Cancel(context.Background(), CancelReminderArgs{ID: 99}); err == nil {
		t.Fatal("unknown id must error")
	}
}

func TestReminderManifest_LoadsEveryTool(t *testing.T) {
	for _, name := range []string{"schedule_reminder", "list_reminders", "cancel_reminder"} {
		params, desc, err := LoadManifestAsTool("reminders", name)
		if err != nil || desc == "" || params["type"] != "object" {
			t.Errorf("%s: params=%v desc=%q err=%v", name, params, desc, err)
		}
	}
	params, _, _ := LoadManifestAsTool("reminders", "schedule_reminder")
	if _, ok := params["properties"].(map[string]any)["when"]; !ok {
		t.Error("schedule_reminder must declare 'when'")
	}
}
