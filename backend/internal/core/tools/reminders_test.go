package tools

import (
	"context"
	"errors"
	"testing"

	"llm-proxy/models"
)

type fakeReminderService struct {
	got     ReminderRequest
	cancels []int64
	err     error
}

func (f *fakeReminderService) Schedule(_ context.Context, r ReminderRequest) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.got = r
	return "Reminder 7 set for the time the stack resolved", nil
}

func (f *fakeReminderService) List(context.Context) (string, error) {
	return "No open reminders.", f.err
}

func (f *fakeReminderService) Cancel(_ context.Context, id int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.cancels = append(f.cancels, id)
	return "Reminder cancelled.", nil
}

func TestScheduleReminder_ReturnsTheStacksSentenceAndPassesWhatTheModelAsked(t *testing.T) {
	svc := &fakeReminderService{}
	got, err := NewReminderTools(svc).Schedule(context.Background(), ScheduleReminderArgs{
		Message: "call bank", When: "2026-10-12 20:00", Timezone: "Europe/London", Connector: "slack", Until: "u", Cron: "c",
	})
	if err != nil || got != "Reminder 7 set for the time the stack resolved" {
		t.Fatalf("got %v, %v", got, err)
	}
	want := ReminderRequest{Message: "call bank", When: "2026-10-12 20:00", Timezone: "Europe/London", Connector: "slack", Until: "u", Cron: "c"}
	if svc.got != want {
		t.Fatalf("passed %+v, want %+v", svc.got, want)
	}
}

// The origin connector and the conversation's zone come from the run's context, which the model cannot write.
func TestScheduleReminder_AddsTheOriginAndTheConversationZoneFromTheContext(t *testing.T) {
	svc := &fakeReminderService{}
	ctx := models.WithChatTimezone(models.WithOriginConnector(context.Background(), "telegram"), "Asia/Tokyo")
	if _, err := NewReminderTools(svc).Schedule(ctx, ScheduleReminderArgs{Message: "x", When: "2026-10-12 20:00"}); err != nil {
		t.Fatal(err)
	}
	if svc.got.Origin != "telegram" || svc.got.ChatZone != "Asia/Tokyo" {
		t.Fatalf("request = %+v", svc.got)
	}
}

func TestReminderTools_RefusalsSurfaceInTheStacksWords(t *testing.T) {
	svc := &fakeReminderService{err: errors.New("invalid reminder: reminder time is in the past")}
	tools := NewReminderTools(svc)
	if _, err := tools.Schedule(context.Background(), ScheduleReminderArgs{Message: "x"}); err == nil || err.Error() != svc.err.Error() {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := tools.List(context.Background(), struct{}{}); err == nil {
		t.Fatal("list must surface an error")
	}
	if _, err := tools.Cancel(context.Background(), CancelReminderArgs{ID: 99}); err == nil {
		t.Fatal("cancel must surface an error")
	}
}

func TestReminderTools_ListAndCancelReturnTheStacksSentence(t *testing.T) {
	svc := &fakeReminderService{}
	tools := NewReminderTools(svc)
	if got, _ := tools.List(context.Background(), struct{}{}); got != "No open reminders." {
		t.Fatalf("list = %v", got)
	}
	if got, _ := tools.Cancel(context.Background(), CancelReminderArgs{ID: 3}); got != "Reminder cancelled." || len(svc.cancels) != 1 || svc.cancels[0] != 3 {
		t.Fatalf("cancel = %v %v", got, svc.cancels)
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
	if props, _ := models.SchemaProperties(params); props["when"] == nil {
		t.Error("schedule_reminder must declare 'when'")
	}
}
