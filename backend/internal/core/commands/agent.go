package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"llm-proxy/internal/core/assistant/profile"
)

const (
	researchName  = "research"
	researchAgent = "research"
	researchUsage = "/research <question>"
	agentName     = "agent"
	agentUsage    = "/agent <agent> <question>"
)

// AgentCatalog is the person's agents: the prompt-profile catalog a chat's agent is chosen from.
type AgentCatalog interface {
	Get(id string) (profile.Profile, bool)
	List() []profile.Profile
}

// Agents is what the turn commands need: the agents, and why helpers cannot run ("" when they can; nil means they
// cannot, for a reason nobody gave).
type Agents struct {
	Catalog    AgentCatalog
	HelpersOff func() string
}

// helpersOff says why helpers cannot run, "" when they can.
func (a Agents) helpersOff() string {
	if a.HelpersOff == nil {
		return "helpers are not set up"
	}
	return a.HelpersOff()
}

// start is the turn of one message under one agent: with helpers when the agent names some and they can run, and a
// note when it names some that cannot.
func (a Agents) start(p profile.Profile, question string) TurnStart {
	spec := TurnSpec{Message: question, Agent: p.ID}
	if len(p.Delegates) == 0 {
		return TurnStart{Spec: spec, Preview: "Runs under " + p.Name}
	}
	if why := a.helpersOff(); why != "" {
		return TurnStart{
			Spec:    spec,
			Note:    fmt.Sprintf("Helpers are off (%s), so %s answers on its own.", why, p.Name),
			Preview: fmt.Sprintf("Runs under %s without helpers: %s", p.Name, why),
		}
	}
	spec.UseHelpers = true
	return TurnStart{Spec: spec, Preview: fmt.Sprintf("Runs under %s with helpers", p.Name)}
}

// Research is the /research command: one message asked of the Research agent, with helpers when they can run. The
// conversation's own agent is unchanged afterwards.
func Research(agents Agents) Command {
	return Command{
		Name:     researchName,
		Usage:    researchUsage,
		Summary:  "Ask the Research agent, with helpers when they can run",
		Examples: []string{"compare the latest Go, Rust and Zig releases"},
		Turn: func(_ context.Context, req Request) (TurnStart, error) {
			if req.Args == "" {
				return TurnStart{}, errors.New("ask a question after /research")
			}
			p, ok := agents.Catalog.Get(researchAgent)
			if !ok {
				return TurnStart{}, errors.New("there is no Research agent")
			}
			return agents.start(p, req.Args), nil
		},
	}
}

// Agent is the /agent command: one message asked of any of the person's agents, with helpers when it names some and
// they can run. An agent for the whole chat is not a command (a connector message starts a fresh conversation every
// time, so there is nothing for it to stick to): it is chosen in the chat itself, or as the workspace's default.
func Agent(agents Agents) Command {
	return Command{
		Name:     agentName,
		Usage:    agentUsage,
		Summary:  "Ask one of your agents, for this message only",
		Examples: []string{"research compare the latest Go, Rust and Zig releases"},
		Turn: func(_ context.Context, req Request) (TurnStart, error) {
			id, question, _ := strings.Cut(req.Args, " ")
			question = strings.TrimSpace(question)
			if id == "" {
				return TurnStart{}, fmt.Errorf("name an agent: %s", agents.ids())
			}
			p, ok := agents.Catalog.Get(id)
			if !ok {
				return TurnStart{}, fmt.Errorf("unknown agent %q: choose one of %s", id, agents.ids())
			}
			if question == "" {
				return TurnStart{}, fmt.Errorf("ask a question after the agent (%s)", p.Name)
			}
			return agents.start(p, question), nil
		},
	}
}

// ids lists the agents' ids, in the catalog's order.
func (a Agents) ids() string {
	list := a.Catalog.List()
	ids := make([]string, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.ID)
	}
	return strings.Join(ids, ", ")
}
