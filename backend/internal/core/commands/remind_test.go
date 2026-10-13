package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/reminders"
	"llm-proxy/models"
)

type fakePlanner struct {
	created []reminders.Reminder
	refuse  error
}

func (f *fakePlanner) Plan(r reminders.Reminder) (reminders.Reminder, error) {
	if f.refuse != nil {
		return reminders.Reminder{}, f.refuse
	}
	if r.Connector == "" {
		r.Connector = r.Origin
	}
	return r, nil
}

func (f *fakePlanner) Create(_ context.Context, r reminders.Reminder) (reminders.Reminder, error) {
	f.created = append(f.created, r)
	r.ID = int64(len(f.created))
	return r, nil
}

func (f *fakePlanner) Channel(stored string) string {
	if stored == "" {
		return "in-app"
	}
	return stored
}

func newRemind(t *testing.T) (*Registry, *fakePlanner, Env) {
	t.Helper()
	svc := &fakePlanner{}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	env := Env{Zone: "Europe/London", Now: time.Date(2026, 10, 12, 20, 29, 0, 0, london)}
	return New(Remind(svc)), svc, env
}

func TestRemind_CreatesInTheCallersZone(t *testing.T) {
	r, svc, env := newRemind(t)
	got, handled := r.Execute(context.Background(), "/remind sunday 8pm eye test monday 11:35", env)
	reply := got.Text
	if !handled {
		t.Fatal("not handled")
	}
	if want := "Reminder 1 set for Sun 18 Oct 2026 20:00 Europe/London via in-app: eye test monday 11:35"; reply != want {
		t.Errorf("reply = %q, want %q", reply, want)
	}
	stored := svc.created[0]
	if want := time.Date(2026, 10, 18, 19, 0, 0, 0, time.UTC); !stored.FireAt.Equal(want) {
		t.Errorf("FireAt = %v, want %v (20:00 BST)", stored.FireAt, want)
	}
	if stored.Timezone != "Europe/London" || stored.Message != "eye test monday 11:35" || stored.CreatedBy != createdByUser {
		t.Errorf("stored %+v", stored)
	}
}

func TestRemind_AnswersOnTheOriginChannel(t *testing.T) {
	r, svc, env := newRemind(t)
	ctx := models.WithOriginConnector(context.Background(), "telegram")
	got, _ := r.Execute(ctx, "/remind tomorrow 9am call the bank", env)
	reply := got.Text
	if !strings.Contains(reply, "via telegram") || svc.created[0].Origin != "telegram" {
		t.Errorf("reply %q origin %q: want the request's own channel", reply, svc.created[0].Origin)
	}
}

func TestRemind_Recurring(t *testing.T) {
	r, svc, env := newRemind(t)
	got, _ := r.Execute(context.Background(), "/remind every weekday 9am stand-up", env)
	reply := got.Text
	if svc.created[0].Cron != "0 9 * * 1-5" || !strings.Contains(reply, "repeats (0 9 * * 1-5)") {
		t.Errorf("cron %q reply %q", svc.created[0].Cron, reply)
	}
}

func TestRemind_UnreadableTimeStoresNothingAndShowsUsage(t *testing.T) {
	r, svc, env := newRemind(t)
	got, handled := r.Execute(context.Background(), "/remind blah 9am call", env)
	reply := got.Text
	if !handled || len(svc.created) != 0 {
		t.Fatalf("handled=%v created=%d", handled, len(svc.created))
	}
	if !strings.Contains(reply, `"blah"`) || !strings.Contains(reply, remindUsage) {
		t.Errorf("reply %q should name the word and the usage", reply)
	}
}

func TestRemind_RefusedReminderReadsPlainly(t *testing.T) {
	r, svc, env := newRemind(t)
	svc.refuse = fmt.Errorf("%w: that time has passed", reminders.ErrInvalid)
	got, _ := r.Execute(context.Background(), "/remind today 8am x", env)
	reply := got.Text
	if strings.Contains(reply, "invalid reminder:") || !strings.Contains(reply, "that time has passed") {
		t.Errorf("reply %q", reply)
	}
}

func TestRemind_PreviewStoresNothing(t *testing.T) {
	r, svc, env := newRemind(t)
	p, ok := r.Preview(context.Background(), "/remind tomorrow 9am call the bank", env)
	if !ok || p.Error != "" || !strings.Contains(p.Text, "Tue 13 Oct 2026 09:00 Europe/London") || len(svc.created) != 0 {
		t.Errorf("preview %+v created=%d", p, len(svc.created))
	}
	p, _ = r.Preview(context.Background(), "/remind ", env)
	if p.Error == "" {
		t.Errorf("empty args should explain, got %+v", p)
	}
}

func TestRemind_UnknownZoneFallsBackToUTC(t *testing.T) {
	r, svc, env := newRemind(t)
	env.Zone = "Mars/Base"
	r.Execute(context.Background(), "/remind tomorrow 9am x", env)
	if svc.created[0].Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", svc.created[0].Timezone)
	}
}
