package assistant

import (
	"testing"

	"llm-proxy/internal/core/proxy"
)

func TestParseLoopStrategy(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want LoopStrategyName
	}{
		{"empty defaults to react", "", LoopReact},
		{"react passthrough", "react", LoopReact},
		{"plan_execute passthrough", "plan_execute", LoopPlanExecute},
		{"unknown defaults to react", "nonsense", LoopReact},
		{"deferred archetype defaults to react", "map_reduce", LoopReact},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseLoopStrategy(LoopStrategyName(tc.in)); got != tc.want {
				t.Errorf("ParseLoopStrategy(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoopStrategyName_Valid(t *testing.T) {
	cases := []struct {
		in   LoopStrategyName
		want bool
	}{
		{LoopReact, true},
		{LoopPlanExecute, true},
		{LoopEvaluatorOptimizer, true},
		{"", false},
		{"react", true},
		{"plan_execute", true},
		{"evaluator_optimizer", true},
		{"map_reduce", false},
		{"REACT", false},
	}
	for _, tc := range cases {
		if got := tc.in.Valid(); got != tc.want {
			t.Errorf("LoopStrategyName(%q).Valid() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLoopStrategyRegistry_Build(t *testing.T) {
	t.Run("registered strategy builds", func(t *testing.T) {
		s, err := loopStrategies.Build(LoopReact)
		if err != nil {
			t.Fatalf("Build(react) failed: %v", err)
		}
		if s.Name() != LoopReact {
			t.Errorf("expected react strategy, got %s", s.Name())
		}
		if _, err := loopStrategies.Build(LoopPlanExecute); err != nil {
			t.Fatalf("Build(plan_execute) failed: %v", err)
		}
	})

	t.Run("unregistered strategy errors", func(t *testing.T) {
		_, err := loopStrategies.Build("map_reduce")
		if err == nil {
			t.Fatal("expected error for unregistered strategy")
		}
	})

	t.Run("fresh registry starts empty", func(t *testing.T) {
		r := NewLoopStrategyRegistry()
		if _, err := r.Build(LoopReact); err == nil {
			t.Fatal("expected error from empty registry")
		}
	})
}

func TestRegisteredLoopStrategyNames(t *testing.T) {
	names := RegisteredLoopStrategyNames()
	want := []string{"evaluator_optimizer", "plan_execute", "react"}
	if len(names) != len(want) {
		t.Fatalf("expected %d registered strategies, got %v", len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q (sorted, registry-derived)", i, names[i], want[i])
		}
	}
}

func TestResolveLoopStrategyName(t *testing.T) {
	client := &MockClient{}
	provider := &MockProvider{}
	engine := &MockEngine{}

	t.Run("explicit per-model config wins", func(t *testing.T) {
		agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: LoopPlanExecute})
		if got := resolveLoopStrategyName(agent); got != LoopPlanExecute {
			t.Errorf("expected plan_execute, got %s", got)
		}
		s := resolveLoopStrategy(agent)
		if s.Name() != LoopPlanExecute {
			t.Errorf("expected resolved strategy plan_execute, got %s", s.Name())
		}
	})

	t.Run("no explicit config falls back to provider default", func(t *testing.T) {
		agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
		if got := resolveLoopStrategyName(agent); got != LoopReact {
			t.Errorf("expected provider default react, got %s", got)
		}
	})

	t.Run("unknown configured value falls back to react with log", func(t *testing.T) {
		agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: "nonsense"})
		// ParseLoopStrategy already normalized at config-apply time; the resolver
		// must not panic and must resolve to react.
		s := resolveLoopStrategy(agent)
		if s.Name() != LoopReact {
			t.Errorf("expected react fallback, got %s", s.Name())
		}
	})
}

// TestLastUserMessage pins the history scan helper reused by PlanExecuteStrategy.
func TestLastUserMessage(t *testing.T) {
	cases := []struct {
		name    string
		history []proxy.Message
		want    string
	}{
		{"empty history", nil, ""},
		{"no user message", []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}}, ""},
		{"single user message", []proxy.Message{{Role: proxy.UserRole, Content: "first"}}, "first"},
		{"last user message wins", []proxy.Message{
			{Role: proxy.UserRole, Content: "first"},
			{Role: proxy.AssistantRole, Content: "thinking"},
			{Role: proxy.UserRole, Content: "last"},
		}, "last"},
		{"assistant after last user", []proxy.Message{
			{Role: proxy.UserRole, Content: "task"},
			{Role: proxy.AssistantRole, Content: "response"},
		}, "task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastUserMessage(tc.history); got != tc.want {
				t.Errorf("lastUserMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}
