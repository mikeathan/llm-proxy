package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeStarter starts the automations it knows, with the status it is given.
type fakeStarter struct {
	known  map[string]AutomationStart
	gotWS  string
	gotRun string
}

func (f *fakeStarter) Start(workspaceID, name string) (AutomationStart, error) {
	f.gotWS, f.gotRun = workspaceID, name
	start, ok := f.known[name]
	if !ok {
		return AutomationStart{}, errors.New("automation not found: " + workspaceID + "/" + name)
	}
	return start, nil
}

func TestRun_StartsTheAutomationInTheChatsWorkspace(t *testing.T) {
	starter := &fakeStarter{known: map[string]AutomationStart{"llm_news": {Status: AutomationStarted}}}
	reply, handled := New(Run(starter)).Execute(context.Background(), "/run llm_news", Env{WorkspaceID: "ws-1"})
	if !handled || reply.Failed || reply.Automation != "llm_news" || reply.Text != "Running 'llm_news'..." {
		t.Fatalf("reply = %+v", reply)
	}
	if starter.gotWS != "ws-1" || starter.gotRun != "llm_news" {
		t.Errorf("started %q in %q", starter.gotRun, starter.gotWS)
	}
}

func TestRun_SaysWhenTheRunWaits(t *testing.T) {
	cases := map[string]struct {
		start AutomationStart
		want  string
	}{
		"queued":  {AutomationStart{Status: AutomationQueued, Position: 2}, "'news' is queued (position 2)."},
		"skipped": {AutomationStart{Status: AutomationSkipped}, "'news' was skipped: it has nothing to run now."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reply, _ := New(Run(&fakeStarter{known: map[string]AutomationStart{"news": tc.start}})).Execute(context.Background(), "/run news", Env{WorkspaceID: "ws"})
			if reply.Failed || reply.Text != tc.want {
				t.Errorf("reply = %+v, want %q", reply, tc.want)
			}
			if wantFollow := tc.start.Status == AutomationQueued; (reply.Automation != "") != wantFollow {
				t.Errorf("follow the run = %q; a queued run is followed, a skipped one is not", reply.Automation)
			}
		})
	}
}

func TestRun_Refuses(t *testing.T) {
	starter := &fakeStarter{known: map[string]AutomationStart{}}
	cases := map[string]struct {
		message string
		env     Env
		want    string
	}{
		"no automation named":   {"/run", Env{WorkspaceID: "ws"}, "name an automation"},
		"an unknown automation": {"/run ghost", Env{WorkspaceID: "ws"}, `could not run "ghost"`},
		"no workspace":          {"/run news", Env{}, "no workspace"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reply, handled := New(Run(starter)).Execute(context.Background(), tc.message, tc.env)
			if !handled || !reply.Failed || reply.Automation != "" || !strings.Contains(reply.Text, tc.want) {
				t.Errorf("reply = %+v, want refused with %q", reply, tc.want)
			}
		})
	}
}

// A preview never starts anything.
func TestRun_PreviewStartsNothing(t *testing.T) {
	starter := &fakeStarter{known: map[string]AutomationStart{"news": {Status: AutomationStarted}}}
	p, matched := New(Run(starter)).Preview(context.Background(), "/run news", Env{WorkspaceID: "ws"})
	if !matched || p.Text != "Runs 'news' now" || starter.gotRun != "" {
		t.Errorf("preview = %+v, started %q", p, starter.gotRun)
	}
}
