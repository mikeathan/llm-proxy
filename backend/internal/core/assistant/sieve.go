// sieve.go — Context-pressure sieves that keep the LLM's input under the
// context window.  Three levels: physical (every turn, compress/drop oldest),
// reactive (first context overflow recovery), aggressive (second consecutive
// recovery attempt).  Exposed via the HistorySieve interface.
package assistant

import (
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

type HistorySieve interface {
	Sieve(history []proxy.Message) []proxy.Message
	Name() string
}

type physicalSieve struct {
	logger        logging.Logger
	contextBudget int
}

func (p *physicalSieve) Name() string { return "physical" }

func (p *physicalSieve) Sieve(history []proxy.Message) []proxy.Message {
	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}
	if totalChars <= p.contextBudget {
		return history
	}

	p.logger.Warn("critical context pressure - activating physical sieve", "chars", totalChars)

	compressStart := sieveLockedHead
	compressEnd := len(history) - sievePhysicalTail
	if compressStart < compressEnd {
		for i := compressStart; i < compressEnd; i++ {
			history[i].Content = truncateLongContent(history[i].Content, compressContentMax)
			history[i].ReasoningContent = truncateLongContent(history[i].ReasoningContent, compressReasoningMax)
		}
		totalChars = 0
		for _, m := range history {
			totalChars += len(m.Content)
		}
		if totalChars <= p.contextBudget {
			return history
		}
	}

	if len(history) <= sieveLockedHead+sievePhysicalTail {
		return history
	}

	newHistory := make([]proxy.Message, 0, len(history))
	newHistory = append(newHistory, history[:sieveLockedHead]...)
	newHistory = append(newHistory, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	newHistory = append(newHistory, history[len(history)-sievePhysicalTail:]...)
	newHistory = append(newHistory, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.ContextSieveWarning,
	})
	return newHistory
}

type reactiveSieve struct {
	logger logging.Logger
}

func (r *reactiveSieve) Name() string { return "reactive" }

func (r *reactiveSieve) Sieve(history []proxy.Message) []proxy.Message {
	r.logger.Warn("context size overflow detected, applying reactive sieve")
	if len(history) <= sieveLockedHead+sieveReactiveTail {
		return history
	}
	sieved := make([]proxy.Message, 0, len(history))
	sieved = append(sieved, history[:sieveLockedHead]...)
	sieved = append(sieved, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	tail := sieveReactiveTail
	if len(history) < tail+sieveLockedHead {
		tail = len(history) - sieveLockedHead
	}
	return append(sieved, history[len(history)-tail:]...)
}

type aggressiveSieve struct {
	logger logging.Logger
}

func (a *aggressiveSieve) Name() string { return "aggressive" }

func (a *aggressiveSieve) Sieve(history []proxy.Message) []proxy.Message {
	a.logger.Warn("aggressive sieve applied — model stuck after prior recovery attempt")
	sieved := make([]proxy.Message, 0, sieveLockedHead+sieveAggressiveTail+1)
	sieved = append(sieved, history[:sieveLockedHead]...)
	sieved = append(sieved, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	tail := sieveAggressiveTail
	if len(history) < tail+sieveLockedHead {
		tail = len(history) - sieveLockedHead
	}
	return append(sieved, history[len(history)-tail:]...)
}

// Sieve constants — shared between compression and message-dropping sieves.
//
// Locked head protects the first N messages from ever being dropped.
// Must be ≥3 to keep: [0]=system prompt, [1]=user task, [2]=any additional context.
const (
	sieveLockedHead      = 3
	sievePhysicalTail    = 10
	sieveReactiveTail    = 6
	sieveAggressiveTail  = 3
	compressContentMax   = 4000
	compressReasoningMax = 2000
)

// truncateLongContent preserves head+tail around a truncation marker so the
// model still sees the beginning and end of long messages, even when the
// middle is dropped under context pressure. It delegates to the shared
// proxy.TruncateResult with the concise marker used by the sieve layer.
func truncateLongContent(s string, limit int) string {
	return proxy.TruncateResult(s, limit, "\n...[Truncated]...\n")
}

func (a *Agent) applyPhysicalSieve(history []proxy.Message) []proxy.Message {
	s := &physicalSieve{logger: a.deps.Logger, contextBudget: a.config.ContextBudget}
	return s.Sieve(history)
}

func (a *Agent) applyReactiveSieve(history []proxy.Message) []proxy.Message {
	s := &reactiveSieve{logger: a.deps.Logger}
	return s.Sieve(history)
}

func (a *Agent) applyAggressiveSieve(history []proxy.Message) []proxy.Message {
	s := &aggressiveSieve{logger: a.deps.Logger}
	return s.Sieve(history)
}
