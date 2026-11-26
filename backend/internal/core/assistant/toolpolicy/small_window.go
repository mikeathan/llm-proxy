// small_window.go — the tools a run on a small local window keeps. The full tool set's descriptions and schemas (26
// tools, about 20,000 characters on 2026-11-26) fill an 8192-token window before the question is read, so such a run
// is offered the everyday core only: files, search, fetch, memory and the always-on run tools.
package toolpolicy

import (
	"context"
	"slices"

	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// smallWindowTokens is the smallest serving window that keeps every tool: a run whose context budget is below what
// this window derives is on a small window. 16384 is the smallest window the full tool set was seen to work in.
const smallWindowTokens = 16384

// SmallWindowCategories are the tool categories a small-window run keeps. A tool outside the built-in catalog (a
// stack's or an MCP server's) is not among them.
var SmallWindowCategories = []string{
	models.CategoryFileSystem, models.CategorySearch, models.CategoryNetwork, models.CategoryMemory,
	models.CategorySystem, models.CategoryGlobal, models.CategoryAutomation,
}

// smallWindowBudget is the context budget a smallWindowTokens window derives under the local budget policy.
var smallWindowBudget = func() int {
	b, _ := orchestrator.LocalBudgetPolicy{}.Derive(models.ModelConfig{}, orchestrator.ContextResolution{ServingContext: smallWindowTokens})
	return b.ContextBudget
}()

// IsSmallWindow reports whether a run with this workload and context budget is on a small window. Only a local
// budget is a window; a cloud budget is a cost allowance and never trims the tools. An unknown budget (0) is not
// judged.
func IsSmallWindow(workload models.WorkloadClass, contextBudget int) bool {
	return workload == models.WorkloadLocal && contextBudget > 0 && contextBudget < smallWindowBudget
}

// KeptOnSmallWindow reports whether a small-window run keeps the named tool.
func KeptOnSmallWindow(name string) bool {
	return slices.Contains(SmallWindowCategories, models.CategoryOf(name))
}

// SmallWindowTools keeps only the small-window tools of base. A run decides once, when it is built, whether it is on
// a small window (its budget is then the server's own window: the model's metadata is reconciled with the running
// server before a run's options are read), so its tool list and the prompt prefix stay the same turn after turn.
func SmallWindowTools(base Provider) Provider {
	return &windowFit{inner: base}
}

type windowFit struct {
	inner Provider
}

func (w *windowFit) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	tools, err := w.inner.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	kept := make([]proxy.Tool, 0, len(tools))
	for _, t := range tools {
		if KeptOnSmallWindow(t.Function.Name) {
			kept = append(kept, t)
		}
	}
	return kept, nil
}

func (w *windowFit) UseNativeTools() bool { return w.inner.UseNativeTools() }
