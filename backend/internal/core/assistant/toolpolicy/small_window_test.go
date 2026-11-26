package toolpolicy

import (
	"context"
	"slices"
	"testing"

	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// localBudget is the context budget the local policy derives for a serving window, as a run is given it.
func localBudget(t *testing.T, window int) int {
	t.Helper()
	b, err := orchestrator.LocalBudgetPolicy{}.Derive(models.ModelConfig{}, orchestrator.ContextResolution{ServingContext: window})
	if err != nil {
		t.Fatal(err)
	}
	return b.ContextBudget
}

func TestIsSmallWindow(t *testing.T) {
	cases := []struct {
		name     string
		workload models.WorkloadClass
		budget   int
		want     bool
	}{
		{"a local 8k window", models.WorkloadLocal, localBudget(t, 8192), true},
		{"a local 12k window", models.WorkloadLocal, localBudget(t, 12288), true},
		{"a local 16k window keeps every tool", models.WorkloadLocal, localBudget(t, 16384), false},
		{"a local 32k window", models.WorkloadLocal, localBudget(t, 32768), false},
		{"an unknown budget is not judged", models.WorkloadLocal, 0, false},
		{"a cloud budget is a cost allowance, not a window", models.WorkloadCloud, 20000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSmallWindow(c.workload, c.budget); got != c.want {
				t.Errorf("IsSmallWindow(%s, %d) = %v, want %v", c.workload, c.budget, got, c.want)
			}
		})
	}
}

type staticProvider []proxy.Tool

func (p staticProvider) ListTools(context.Context) ([]proxy.Tool, error) { return p, nil }
func (staticProvider) UseNativeTools() bool                              { return true }

func toolsNamed(names ...string) staticProvider {
	out := make(staticProvider, len(names))
	for i, n := range names {
		out[i] = proxy.Tool{Type: "function", Function: proxy.FunctionSchema{Name: n}}
	}
	return out
}

func names(t *testing.T, p Provider) []string {
	t.Helper()
	list, err := p.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(list))
	for i, tool := range list {
		out[i] = tool.Function.Name
	}
	return out
}

func TestSmallWindowTools(t *testing.T) {
	all := toolsNamed(models.ToolFileRead, models.ToolInternetSearch, models.ToolNetworkFetch, models.ToolMemorySearch,
		models.ToolMemoryUpdate, models.ToolSystemError, models.ToolApplyGuardrails, models.ToolAutomationJournal,
		models.ToolScheduleReminder, models.ToolNewsroomFollow, models.ToolNetworkScan, models.ToolRadioDigest,
		models.ToolTerminalExecute, models.ToolNotifyUser, models.ToolDelegate, "stack_offered_tool")
	core := []string{models.ToolFileRead, models.ToolInternetSearch, models.ToolNetworkFetch, models.ToolMemorySearch,
		models.ToolMemoryUpdate, models.ToolSystemError, models.ToolApplyGuardrails, models.ToolAutomationJournal}

	t.Run("it keeps the core tools, in their order", func(t *testing.T) {
		if got := names(t, SmallWindowTools(all)); !slices.Equal(got, core) {
			t.Errorf("got %v, want %v", got, core)
		}
	})
	t.Run("the native-tools mode is the inner provider's", func(t *testing.T) {
		if !SmallWindowTools(all).UseNativeTools() {
			t.Error("UseNativeTools must pass through")
		}
	})
}
