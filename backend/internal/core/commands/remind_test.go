package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeReminders struct {
	got   []RemindRequest
	reply string
	err   error
}

func (f *fakeReminders) Remind(_ context.Context, r RemindRequest) (string, error) {
	f.got = append(f.got, r)
	return f.reply, f.err
}

func newRemind(t *testing.T) (*Registry, *fakeReminders, Env) {
	t.Helper()
	svc := &fakeReminders{reply: "Reminder 1 set for the sentence the stack worded"}
	env := Env{Zone: "Europe/London", Now: time.Date(2026, 10, 12, 20, 29, 0, 0, time.UTC)}
	return New(Remind(svc)), svc, env
}

func TestRemind_SendsTheTextTheZoneAndTheTimeAndShowsTheStacksSentence(t *testing.T) {
	r, svc, env := newRemind(t)
	got, handled := r.Execute(context.Background(), "/remind sunday 8pm eye test", env)
	if !handled || got.Text != svc.reply {
		t.Fatalf("handled=%v reply=%q", handled, got.Text)
	}
	if len(svc.got) != 1 || svc.got[0].Text != "sunday 8pm eye test" || svc.got[0].Zone != "Europe/London" || !svc.got[0].Now.Equal(env.Now) || svc.got[0].Preview {
		t.Fatalf("request = %+v", svc.got)
	}
}

func TestRemind_PassesTheConnectorTheMessageArrivedThrough(t *testing.T) {
	r, svc, env := newRemind(t)
	r.Execute(models.WithOriginConnector(context.Background(), "telegram"), "/remind tomorrow 9am x", env)
	if svc.got[0].Origin != "telegram" {
		t.Fatalf("origin = %q", svc.got[0].Origin)
	}
}

func TestRemind_PreviewAsksOnlyForTheDescription(t *testing.T) {
	r, svc, env := newRemind(t)
	p, ok := r.Preview(context.Background(), "/remind tomorrow 9am call the bank", env)
	if !ok || p.Error != "" || p.Text != svc.reply || !svc.got[0].Preview {
		t.Fatalf("preview %+v request %+v", p, svc.got)
	}
}

func TestRemind_AnErrorShowsTheStacksWordsAndTheUsage(t *testing.T) {
	r, svc, env := newRemind(t)
	svc.err = errors.New(`I could not read "blah" as a time`)
	got, handled := r.Execute(context.Background(), "/remind blah 9am call", env)
	if !handled || !got.Failed || !strings.Contains(got.Text, `"blah"`) || !strings.Contains(got.Text, remindUsage) {
		t.Fatalf("reply %+v should name the word and the usage", got)
	}
}
