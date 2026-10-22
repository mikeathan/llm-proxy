// sieve.go — Context-pressure sieves that keep the LLM's input under the
// context window.  Three levels: physical (every turn, compress/drop oldest),
// reactive (first context overflow recovery), aggressive (second consecutive
// recovery attempt).  Exposed via the HistorySieve interface.
package assistant

import (
	"slices"
	"strings"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

type HistorySieve interface {
	Sieve(history []proxy.Message) []proxy.Message
	Name() string
}

// messageChars approximates a message's contribution to the prompt size:
// visible content + reasoning + native tool-call payloads. Tool-call
// ARGUMENTS must be counted — in native mode they ride in
// ToolCalls[].Function.Arguments (not Content), and a write_file/edit_file
// argument can be thousands of chars. A sieve that counts Content only
// under-measures the request and can let the prompt overflow the serving
// window (2026-08-30 smoke-test runs).
func messageChars(m proxy.Message) int {
	n := len(m.Content) + len(m.ReasoningContent)
	for _, tc := range m.ToolCalls {
		n += len(tc.Function.Arguments)
	}
	return n
}

type physicalSieve struct {
	logger        logging.Logger
	contextBudget int
	overhead      int            // chars the request carries beyond the history (system enrichment, tool schemas)
	ledger        *proxy.Message // progress ledger inserted after the note; nil when nothing has run
	warn          bool           // append the one-time "wrap up now" warning
	warned        bool           // set when the warning was actually appended
}

func (p *physicalSieve) Name() string { return "physical" }

func (p *physicalSieve) Sieve(history []proxy.Message) []proxy.Message {
	out, _ := shrinkHistory(p.prune(history), max(p.contextBudget-p.overhead, 0))
	return out
}

// prune drops and compresses old messages.
func (p *physicalSieve) prune(history []proxy.Message) []proxy.Message {
	// The history gets what the budget leaves after the request's fixed
	// overhead; judged against the whole budget a history that fits alone
	// was left untouched while the request overflowed.
	budget := max(p.contextBudget-p.overhead, 0)
	totalChars := 0
	for _, m := range history {
		totalChars += messageChars(m)
	}
	if totalChars <= budget {
		return history
	}

	p.logger.Warn("critical context pressure - activating physical sieve", "chars", totalChars)

	compressStart := sieveLockedHead
	compressEnd := len(history) - sievePhysicalTail
	task := taskIndex(history)
	if compressStart < compressEnd {
		for i := compressStart; i < compressEnd; i++ {
			if i == task {
				continue // the question being worked on is not log
			}
			history[i].Content = truncateLongContent(history[i].Content, compressContentMax)
			history[i].ReasoningContent = truncateLongContent(history[i].ReasoningContent, compressReasoningMax)
		}
		totalChars = 0
		for _, m := range history {
			totalChars += messageChars(m)
		}
		if totalChars <= budget {
			return history
		}
	}

	// A previous firing's note, ledger and warning are replaced, never stacked:
	// they would otherwise pile up in the kept tail, one more per firing, on a
	// small window where the sieve fires every turn.
	clean := withoutSieveMessages(history)
	if len(clean) <= sieveLockedHead+sievePhysicalTail {
		return history
	}

	head := keptHead(clean)
	from := tailStart(clean, len(head), sievePhysicalTail)
	newHistory := make([]proxy.Message, 0, len(clean))
	newHistory = append(newHistory, head...)
	newHistory = append(newHistory, keptQuestion(clean, len(head), from)...)
	newHistory = appendNoteAndLedger(newHistory, p.ledger)
	newHistory = append(newHistory, clean[from:]...)
	if p.warn {
		newHistory = append(newHistory, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.ContextSieveWarning,
		})
		p.warned = true
	}
	return newHistory
}

// sieveHistory keeps the locked head, inserts the sieve note, and retains the
// most recent tail messages. Both sieves share this shape; only the tail size
// and the reactive pre-check differ.
func sieveHistory(history []proxy.Message, tail int, ledger *proxy.Message) []proxy.Message {
	history = withoutSieveMessages(history)
	if len(history) < tail+sieveLockedHead {
		tail = len(history) - sieveLockedHead
	}
	head := keptHead(history)
	from := tailStart(history, len(head), tail)
	sieved := make([]proxy.Message, 0, len(history))
	sieved = append(sieved, head...)
	sieved = append(sieved, keptQuestion(history, len(head), from)...)
	sieved = appendNoteAndLedger(sieved, ledger)
	return append(sieved, history[from:]...)
}

// keptHead is the leading part of a history a sieve never drops: the system prompt, the task and any context
// before the first step, up to sieveLockedHead messages. It stops short of the tool loop: a call kept here
// without its result (dropped with the middle) is an orphan, so the first step is dropped, whole, with the rest
// and the progress ledger answers for it.
func keptHead(history []proxy.Message) []proxy.Message {
	end := min(sieveLockedHead, len(history))
	for end > sieveMinHead && inToolLoop(history[end-1]) {
		end--
	}
	return history[:end]
}

// taskIndex is where the question being worked on sits: the newest user message that is the user's own, not a nag,
// a sieve note or a ledger the agent injected. In a first question that is inside the locked head; in a later turn
// of a conversation the head holds the first turns instead, so the question sits in the middle a sieve drops. -1
// when there is none.
func taskIndex(history []proxy.Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.UserRole && !isAgentControlMessage(history[i]) {
			return i
		}
	}
	return -1
}

// keptQuestion is the question being worked on, as the one message a sieve keeps between the head and its note when
// it would otherwise be dropped (it lies after the head and before the kept tail). Nil when the head or the tail
// already holds it, so a first question is never kept twice.
func keptQuestion(history []proxy.Message, headEnd, tailFrom int) []proxy.Message {
	if i := taskIndex(history); i >= headEnd && i < tailFrom {
		return history[i : i+1]
	}
	return nil
}

// tailStart is where the kept tail of a history begins: n messages from the end, never inside the head, and never
// between a call and its results. A tail that would open on a result starts at the call that made it instead,
// so the newest step is whole; a result whose call lies in the head cannot be completed and is skipped.
func tailStart(history []proxy.Message, headEnd, n int) int {
	start := max(len(history)-n, headEnd)
	for start > headEnd && history[start].Role == proxy.ToolRole {
		start--
	}
	for start < len(history) && history[start].Role == proxy.ToolRole {
		start++
	}
	return start
}

// inToolLoop reports whether a message belongs to a tool step: a call or a result.
func inToolLoop(m proxy.Message) bool {
	return m.Role == proxy.ToolRole || len(m.ToolCalls) > 0
}

// appendNoteAndLedger appends the byte-exact sieve note followed by the
// progress ledger (when there is one). The note stays its own message so the
// control-message allowlist matches it exactly.
func appendNoteAndLedger(history []proxy.Message, ledger *proxy.Message) []proxy.Message {
	history = append(history, proxy.Message{Role: proxy.UserRole, Content: prompts.SieveSystemNote})
	if ledger != nil {
		history = append(history, *ledger)
	}
	return history
}

// withoutSieveMessages drops the messages an earlier sieve inserted — the sieve
// note, the progress ledger and the context warning — so a new prune replaces
// them instead of stacking beside them. It returns history unchanged (no copy)
// when there are none.
func withoutSieveMessages(history []proxy.Message) []proxy.Message {
	if !slices.ContainsFunc(history, isSieveMessage) {
		return history
	}
	return slices.DeleteFunc(slices.Clone(history), isSieveMessage)
}

// isSieveMessage reports the agent-injected messages a sieve adds to a history.
func isSieveMessage(m proxy.Message) bool {
	if m.Role != proxy.UserRole {
		return false
	}
	return m.Content == prompts.SieveSystemNote ||
		m.Content == prompts.ContextSieveWarning ||
		strings.HasPrefix(m.Content, prompts.SieveLedgerHeader)
}

// sieveLedger returns the ledger message for a sieve, or nil when nothing ran.
func (a *Agent) sieveLedger() *proxy.Message {
	msg, ok := a.ledgerMessage()
	if !ok {
		return nil
	}
	return &msg
}

type reactiveSieve struct {
	logger logging.Logger
	ledger *proxy.Message
}

func (r *reactiveSieve) Name() string { return "reactive" }

func (r *reactiveSieve) Sieve(history []proxy.Message) []proxy.Message {
	r.logger.Warn("context size overflow detected, applying reactive sieve")
	if len(history) <= sieveLockedHead+sieveReactiveTail {
		return history
	}
	return sieveHistory(history, sieveReactiveTail, r.ledger)
}

type aggressiveSieve struct {
	logger logging.Logger
	ledger *proxy.Message
}

func (a *aggressiveSieve) Name() string { return "aggressive" }

func (a *aggressiveSieve) Sieve(history []proxy.Message) []proxy.Message {
	a.logger.Warn("aggressive sieve applied — model stuck after prior recovery attempt")
	return sieveHistory(history, sieveAggressiveTail, a.ledger)
}

// Sieve constants — shared between compression and message-dropping sieves.
//
// Locked head protects the first N messages from ever being dropped.
// Must be ≥3 to keep: [0]=system prompt, [1]=user task, [2]=any additional context.
const (
	sieveLockedHead      = 3
	sieveMinHead         = 2 // the system prompt and the task
	sievePhysicalTail    = 10
	sieveReactiveTail    = 6
	sieveAggressiveTail  = 3
	compressContentMax   = 4000
	compressReasoningMax = 2000

	sieveTruncationMarker = "\n...[Truncated]...\n"
)

// truncateLongContent preserves head+tail around a truncation marker so the
// model still sees the beginning and end of long messages, even when the
// middle is dropped under context pressure. It delegates to the shared
// proxy.TruncateResult with the concise marker used by the sieve layer.
func truncateLongContent(s string, limit int) string {
	return proxy.TruncateResult(s, limit, sieveTruncationMarker)
}

// applyPhysicalSieve prunes an over-budget history. The "wrap up now" warning is
// a one-time cue per run: on a small window the sieve fires on later turns too,
// and a run that keeps working must not be told to stop every turn (the note and
// the progress ledger are refreshed each time regardless).
func (a *Agent) applyPhysicalSieve(history []proxy.Message, overhead int) []proxy.Message {
	warn := a.runS == nil || !a.runS.sieve.contextWarned
	s := &physicalSieve{logger: a.deps.Logger, contextBudget: a.requestLimit(), overhead: overhead, ledger: a.sieveLedger(), warn: warn}
	pruned := s.Sieve(history)
	if s.warned && a.runS != nil {
		a.runS.sieve.contextWarned = true
	}
	return pruned
}

func (a *Agent) applyReactiveSieve(history []proxy.Message) []proxy.Message {
	s := &reactiveSieve{logger: a.deps.Logger, ledger: a.sieveLedger()}
	return s.Sieve(history)
}

func (a *Agent) applyAggressiveSieve(history []proxy.Message) []proxy.Message {
	s := &aggressiveSieve{logger: a.deps.Logger, ledger: a.sieveLedger()}
	return s.Sieve(history)
}
