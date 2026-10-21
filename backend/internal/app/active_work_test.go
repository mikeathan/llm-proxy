package app

import (
	"errors"
	"testing"
)

// Clearing runtime data while a run is executing or queued would silently drop it, so the guard answers "busy" for any
// sign of work and, when it cannot tell, fails closed.
func TestActiveWorkChecker(t *testing.T) {
	idle := workProbe{
		listWorkspaces: func() ([]string, error) { return []string{"a", "b"}, nil },
		assistantBusy:  func(string) bool { return false },
		automationBusy: func(string) bool { return false },
		laneQueued:     func() bool { return false },
	}
	cases := []struct {
		name   string
		change func(p *workProbe)
		want   bool
	}{
		{"nothing running", func(p *workProbe) {}, false},
		{"workspaces cannot be listed: fail closed", func(p *workProbe) {
			p.listWorkspaces = func() ([]string, error) { return nil, errors.New("disk") }
		}, true},
		{"an assistant run in the second workspace", func(p *workProbe) { p.assistantBusy = func(id string) bool { return id == "b" } }, true},
		{"an automation running", func(p *workProbe) { p.automationBusy = func(id string) bool { return id == "a" } }, true},
		{"a lane entry waiting", func(p *workProbe) { p.laneQueued = func() bool { return true } }, true},
		{"no workspaces at all", func(p *workProbe) { p.listWorkspaces = func() ([]string, error) { return nil, nil } }, false},
	}
	for _, c := range cases {
		p := idle
		c.change(&p)
		if got := activeWorkChecker(p)(); got != c.want {
			t.Errorf("%s: busy = %v, want %v", c.name, got, c.want)
		}
	}
}
