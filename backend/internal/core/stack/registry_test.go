package stack

import (
	"errors"
	"testing"
)

// fakeSwitch is a Switch over a bool, so a test can see what the registry did.
type fakeSwitch struct {
	on     bool
	sets   int
	setErr error
}

func (f *fakeSwitch) Enabled() bool { return f.on }

func (f *fakeSwitch) Set(enabled bool) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.on = enabled
	return nil
}

func testStacks() []Stack {
	return []Stack{
		{ID: Network, Title: "Network", Layers: []Layer{{ID: "discover", Title: "Discover"}, {ID: "listen", Title: "Listen"}, {ID: "tell", Title: "Tell"}}},
		{ID: Agents, Title: "Agents", Layers: []Layer{{ID: "watch", Title: "Watch"}}, Actions: []Action{{ID: "lockdown", Title: "Lock down"}}},
	}
}

func module(id string, layer string, sw Switch, needs ...string) Module {
	return Module{ID: id, Title: id, Purpose: "does " + id, Stack: Network, Layer: layer, Sends: SendsNothing, Needs: needs, Switch: sw}
}

func TestNew_RefusesAnInconsistentRegistry(t *testing.T) {
	sw := &fakeSwitch{}
	cases := map[string][]Module{
		"duplicate id":          {module("a", "discover", sw), module("a", "listen", sw)},
		"unknown layer":         {module("a", "nowhere", sw)},
		"unknown need":          {module("a", "discover", sw, "ghost")},
		"need on a higher tier": {module("a", "discover", sw, "b"), module("b", "listen", sw)},
		"need is itself":        {module("a", "discover", sw, "a")},
		"no switch":             {module("a", "discover", nil)},
		"no id":                 {module("", "discover", sw)},
		"unknown stack":         {{ID: "a", Title: "a", Stack: "mars", Layer: "discover", Sends: SendsNothing, Switch: sw}},
		"need in another stack": {module("a", "discover", sw), {ID: "b", Title: "b", Stack: Agents, Layer: "watch", Sends: SendsNothing, Switch: sw, Needs: []string{"a"}}},
	}
	for name, mods := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(testStacks(), mods); err == nil {
				t.Fatal("New accepted an inconsistent registry")
			}
		})
	}
}

func TestNew_AcceptsNeedsOnTheSameOrAnEarlierTier(t *testing.T) {
	sw := &fakeSwitch{}
	_, err := New(testStacks(), []Module{
		module("presence", "discover", sw),
		module("names", "discover", sw, "presence"),
		module("summary", "tell", sw, "presence"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
}

func build(t *testing.T, mods ...Module) *Registry {
	t.Helper()
	r, err := New(testStacks(), mods)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestView_DrawsTheTopTierFirstAndKeepsEmptyOnesOut(t *testing.T) {
	r := build(t, module("presence", "discover", &fakeSwitch{}), module("summary", "tell", &fakeSwitch{}, "presence"))
	v, err := r.View(Network, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range v.Layers {
		got = append(got, l.ID)
	}
	if len(got) != 2 || got[0] != "tell" || got[1] != "discover" {
		t.Fatalf("layers = %v, want [tell discover] (the empty listen tier is left out)", got)
	}
}

func TestView_StateHealthAndWhatBlocksAModule(t *testing.T) {
	presence, names := &fakeSwitch{on: true}, &fakeSwitch{on: false}
	summary := &fakeSwitch{on: true}
	mods := []Module{
		{ID: "presence", Title: "Presence check", Stack: Network, Layer: "discover", Sends: SendsLAN, Switch: presence, Monitors: []string{"presence"}},
		module("names", "discover", names),
		{ID: "summary", Title: "Weekly summary", Stack: Network, Layer: "tell", Sends: SendsNothing, Switch: summary, Needs: []string{"presence"}},
		{ID: "router", Title: "Router log", Stack: Network, Layer: "listen", Sends: Listens, Switch: &fakeSwitch{on: true}, Monitors: []string{"router_log", "router_log_quiet"},
			Ready: func() (bool, string) { return false, "Guardrails are off" }},
	}
	r := build(t, mods...)

	v, err := r.View(Network, map[string]bool{"router_log_quiet": true})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ModuleView{}
	for _, l := range v.Layers {
		for _, m := range l.Modules {
			byID[m.ID] = m
		}
	}
	if m := byID["presence"]; !m.Enabled || m.Health != HealthWorking || m.Blocked != "" {
		t.Errorf("presence = %+v, want on, working, not blocked", m)
	}
	if m := byID["names"]; m.Enabled || m.Health != "" {
		t.Errorf("names is off and must show no health, got %+v", m)
	}
	if m := byID["router"]; m.Health != HealthStopped || m.Blocked != "Guardrails are off" {
		t.Errorf("router = %+v, want stopped (one of its two parts stalled) and blocked by its readiness", m)
	}
	if v.Summary.On != 3 || v.Summary.Total != 4 || v.Summary.Stopped != 1 || v.Summary.Blocked != 1 {
		t.Errorf("summary = %+v, want 3 of 4 on, 1 stopped, 1 blocked", v.Summary)
	}

	presence.on = false
	v, _ = r.View(Network, nil)
	for _, l := range v.Layers {
		for _, m := range l.Modules {
			if m.ID == "summary" && m.Blocked != "needs Presence check" {
				t.Errorf("summary blocked = %q, want \"needs Presence check\"", m.Blocked)
			}
		}
	}
}

func TestView_AnUnknownStackIsNotFound(t *testing.T) {
	if _, err := build(t).View("mars", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestView_ACurrentActionIsListedOnItsStack(t *testing.T) {
	v, err := build(t).View(Agents, nil)
	if err != nil || len(v.Actions) != 1 || v.Actions[0].ID != "lockdown" {
		t.Fatalf("view = %+v, %v; want the lock down action", v, err)
	}
}

func TestSwitch_TurnsAModuleOnAndOff(t *testing.T) {
	sw := &fakeSwitch{}
	r := build(t, module("presence", "discover", sw))
	if _, err := r.Switch(Network, "presence", true); err != nil || !sw.on {
		t.Fatalf("on: err = %v, on = %v", err, sw.on)
	}
	if _, err := r.Switch(Network, "presence", false); err != nil || sw.on {
		t.Fatalf("off: err = %v, on = %v", err, sw.on)
	}
}

func TestSwitch_RefusesAModuleWhoseNeedIsOffAndNamesIt(t *testing.T) {
	need, dependent := &fakeSwitch{}, &fakeSwitch{}
	needMod := module("presence", "discover", need)
	needMod.Title = "Presence check"
	r := build(t, needMod, module("summary", "tell", dependent, "presence"))

	_, err := r.Switch(Network, "summary", true)
	var ne *NeedError
	if !errors.As(err, &ne) || !errors.Is(err, ErrNeedOff) || ne.Need != "Presence check" {
		t.Fatalf("err = %v, want a NeedError naming the Presence check", err)
	}
	if dependent.sets != 0 || dependent.on {
		t.Fatal("the switch was changed although the module was refused")
	}
	need.on = true
	if _, err := r.Switch(Network, "summary", true); err != nil {
		t.Fatalf("with the need on: %v", err)
	}
}

func TestSwitch_SwitchingOffAModuleOthersNeedIsAllowedAndNamesTheDependents(t *testing.T) {
	need, dependent := &fakeSwitch{on: true}, &fakeSwitch{on: true}
	r := build(t, module("presence", "discover", need), func() Module {
		m := module("summary", "tell", dependent, "presence")
		m.Title = "Weekly summary"
		return m
	}())

	out, err := r.Switch(Network, "presence", false)
	if err != nil || need.on {
		t.Fatalf("err = %v, on = %v; switching off must succeed", err, need.on)
	}
	if len(out.Blocked) != 1 || out.Blocked[0] != "Weekly summary" {
		t.Fatalf("blocked = %v, want [Weekly summary]", out.Blocked)
	}
	if !dependent.on {
		t.Fatal("the dependent's own setting must not be changed")
	}
}

func TestSwitch_AFailedWriteIsReportedAndLeavesTheModuleAsItWas(t *testing.T) {
	boom := errors.New("disk full")
	sw := &fakeSwitch{setErr: boom}
	r := build(t, module("presence", "discover", sw))
	if _, err := r.Switch(Network, "presence", true); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the write error", err)
	}
	if sw.on {
		t.Fatal("module reads as on after a failed write")
	}
}

func TestSwitch_UnknownIsNotFound(t *testing.T) {
	r := build(t, module("presence", "discover", &fakeSwitch{}))
	for _, c := range []struct {
		s  StackID
		id string
	}{{"mars", "presence"}, {Network, "ghost"}, {Agents, "presence"}} {
		if _, err := r.Switch(c.s, c.id, true); !errors.Is(err, ErrNotFound) {
			t.Errorf("Switch(%s, %s) = %v, want ErrNotFound", c.s, c.id, err)
		}
	}
}

func TestStacks_SummarisesEachStack(t *testing.T) {
	r := build(t, module("presence", "discover", &fakeSwitch{on: true}), module("names", "discover", &fakeSwitch{}))
	sums := r.Stacks(nil)
	if len(sums) != 2 || sums[0].ID != Network || sums[0].On != 1 || sums[0].Total != 2 {
		t.Fatalf("summaries = %+v", sums)
	}
}

func TestView_AsksEachModulesReadinessOnce(t *testing.T) {
	calls := 0
	m := module("presence", "discover", &fakeSwitch{})
	m.Ready = func() (bool, string) { calls++; return true, "" }
	r := build(t, m, module("summary", "tell", &fakeSwitch{}))
	if _, err := r.View(Network, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("readiness asked %d times for one module in one view: it may read the disk", calls)
	}
}
