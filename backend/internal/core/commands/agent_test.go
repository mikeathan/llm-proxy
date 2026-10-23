package commands

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/profile"
)

// fakeCatalog is a profile catalog with a Research agent that names helpers and a General one that does not.
type fakeCatalog struct{ profiles []profile.Profile }

func (c fakeCatalog) Get(id string) (profile.Profile, bool) {
	for _, p := range c.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return profile.Profile{}, false
}
func (c fakeCatalog) List() []profile.Profile { return c.profiles }

var testCatalog = fakeCatalog{profiles: []profile.Profile{
	{ID: "general", Name: "General"},
	{ID: "research", Name: "Research", Delegates: []string{"web-researcher"}},
}}

func agents(helpersOff string) Agents {
	return Agents{Catalog: testCatalog, HelpersOff: func() string { return helpersOff }}
}

func TestResearch_StartsATurnUnderResearchWithHelpers(t *testing.T) {
	r := New(Research(agents("")))
	reply, handled := r.Execute(context.Background(), "/research compare Go and Rust", Env{})
	if !handled || reply.Failed || reply.Turn == nil {
		t.Fatalf("reply = %+v, handled %v; want a turn", reply, handled)
	}
	want := TurnSpec{Message: "compare Go and Rust", Agent: "research", UseHelpers: true}
	if *reply.Turn != want {
		t.Errorf("turn = %+v, want %+v", *reply.Turn, want)
	}
	if reply.Text != "" {
		t.Errorf("nothing to say when helpers can run, got %q", reply.Text)
	}
}

// With helpers off, the turn still runs under Research, and the person is told why it has no helpers.
func TestResearch_HelpersOffRunsWithoutThemAndSaysSo(t *testing.T) {
	r := New(Research(agents("no Helper model is chosen")))
	reply, _ := r.Execute(context.Background(), "/research compare Go and Rust", Env{})
	if reply.Turn == nil || reply.Turn.UseHelpers || reply.Turn.Agent != "research" {
		t.Fatalf("turn = %+v, want Research without helpers", reply.Turn)
	}
	if !strings.Contains(reply.Text, "no Helper model is chosen") {
		t.Errorf("the note should say why there are no helpers: %q", reply.Text)
	}
}

func TestResearch_Refuses(t *testing.T) {
	cases := map[string]struct {
		agents  Agents
		message string
		want    string
	}{
		"no question":       {agents(""), "/research", "ask a question"},
		"no Research agent": {Agents{Catalog: fakeCatalog{profiles: []profile.Profile{{ID: "general", Name: "General"}}}}, "/research q", "no Research agent"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reply, handled := New(Research(tc.agents)).Execute(context.Background(), tc.message, Env{})
			if !handled || !reply.Failed || reply.Turn != nil || !strings.Contains(reply.Text, tc.want) {
				t.Errorf("reply = %+v, want refused with %q", reply, tc.want)
			}
		})
	}
}

func TestAgent_StartsATurnUnderTheNamedAgent(t *testing.T) {
	r := New(Agent(agents("")))
	cases := map[string]TurnSpec{
		"/agent research what changed in Go 1.27?": {Message: "what changed in Go 1.27?", Agent: "research", UseHelpers: true},
		"/agent@MyBot general  hello there":        {Message: "hello there", Agent: "general"},
	}
	for message, want := range cases {
		reply, handled := r.Execute(context.Background(), message, Env{})
		if !handled || reply.Failed || reply.Turn == nil || *reply.Turn != want {
			t.Errorf("%q: reply = %+v, want turn %+v", message, reply, want)
		}
	}
}

func TestAgent_Refuses(t *testing.T) {
	r := New(Agent(agents("")))
	cases := map[string]string{
		"/agent":                  "name an agent",
		"/agent ghost a question": `unknown agent "ghost": choose one of general, research`,
		// D3: an agent for the whole chat is not a command (a connector message is a fresh conversation every time).
		"/agent research": "ask a question after the agent",
	}
	for message, want := range cases {
		reply, handled := r.Execute(context.Background(), message, Env{})
		if !handled || !reply.Failed || reply.Turn != nil || !strings.Contains(reply.Text, want) {
			t.Errorf("%q: reply = %+v, want refused with %q", message, reply, want)
		}
	}
}

// The preview says how the message would run, without starting anything.
func TestTurnCommands_Preview(t *testing.T) {
	cases := []struct {
		name, message string
		agents        Agents
		cmd           func(Agents) Command
		text, err     string
	}{
		{"research with helpers", "/research q", agents(""), Research, "Runs under Research with helpers", ""},
		{"research, helpers off", "/research q", agents("no Helper model is chosen"), Research, "Runs under Research without helpers: no Helper model is chosen", ""},
		{"an agent without helpers", "/agent general q", agents(""), Agent, "Runs under General", ""},
		{"no question yet", "/agent research", agents(""), Agent, "", "ask a question after the agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, matched := New(tc.cmd(tc.agents)).Preview(context.Background(), tc.message, Env{})
			if !matched || p.Text != tc.text || !strings.Contains(p.Error, tc.err) {
				t.Errorf("preview = %+v, want text %q, error containing %q", p, tc.text, tc.err)
			}
		})
	}
}

// A command that answers in text has no turn; only a turn command starts one.
func TestExecute_TextCommandHasNoTurn(t *testing.T) {
	reply, _ := New(echoCommand("echo")).Execute(context.Background(), "/echo hi", Env{})
	if reply.Turn != nil || reply.Text != "ran:hi" {
		t.Errorf("reply = %+v", reply)
	}
}
