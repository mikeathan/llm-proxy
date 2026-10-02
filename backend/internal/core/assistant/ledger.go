package assistant

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// The progress ledger is a deterministic, code-built record of what the run
// already did. When a sieve drops the middle of the history the ledger takes
// the place of the blind "history distilled" note's missing information, so a
// small model does not redo finished steps. No LLM call: it is built from the
// tool results the loop already holds.
const (
	ledgerShare         = 0.05
	ledgerMinChars      = 300
	ledgerMaxChars      = 2000
	ledgerFallbackChars = 1200

	ledgerMaxEntries     = 200 // bound memory in very long runs; older entries are counted as omitted
	ledgerArgMaxRunes    = 60
	ledgerResultMaxRunes = 80

	ledgerOK   = "✓"
	ledgerFail = "✗"
)

// ledgerKeyArgs are the argument names that identify what a call acted on, in
// priority order. File contents (content, new_block, …) are deliberately absent.
var ledgerKeyArgs = []string{"command", "path", "query", "url", "target", "name"}

// ledgerQuotedArgs are free-text arguments shown in double quotes.
var ledgerQuotedArgs = map[string]bool{"command": true, "query": true}

// ledgerEchoesResult lists tools whose successful output head is worth
// repeating (a command's output is the fact; a file's contents are noise).
var ledgerEchoesResult = map[string]bool{models.ToolTerminalExecute: true}

type ledgerEntry struct {
	line  string
	count int
}

// runLedger accumulates one run's tool outcomes. It is safe for concurrent
// use: tool calls in one batch record from parallel goroutines.
type runLedger struct {
	mu      sync.Mutex
	entries []ledgerEntry
	dropped int
}

// ledgerCharBudget sizes the rendered ledger from the model's context budget.
func ledgerCharBudget(contextBudget int) int {
	return contextShare(contextBudget, ledgerShare, ledgerMinChars, ledgerMaxChars, ledgerFallbackChars)
}

// record adds one finished tool call. raw is the JSON-marshalled result.
func (l *runLedger) record(call proxy.ToolCall, raw []byte) {
	line := formatLedgerLine(call, raw)
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.entries); n > 0 && l.entries[n-1].line == line {
		l.entries[n-1].count++
		return
	}
	l.entries = append(l.entries, ledgerEntry{line: line, count: 1})
	if len(l.entries) > ledgerMaxEntries {
		l.entries = l.entries[1:]
		l.dropped++
	}
}

// render returns the ledger lines (oldest first) within maxChars, newest kept.
// When older steps are cut it leads with a line saying how many.
func (l *runLedger) render(maxChars int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		return ""
	}
	lines := make([]string, len(l.entries))
	total := 0
	for i, e := range l.entries {
		lines[i] = e.line
		if e.count > 1 {
			lines[i] = fmt.Sprintf("%s ×%d", e.line, e.count)
		}
		total += len(lines[i]) + 1
	}
	if total <= maxChars && l.dropped == 0 {
		return strings.Join(lines, "\n")
	}

	reserve := len(fmt.Sprintf(prompts.SieveLedgerOmitted, l.dropped+len(lines)))
	used, first := reserve+1, len(lines)
	for first > 0 && used+len(lines[first-1])+1 <= maxChars {
		first--
		used += len(lines[first]) + 1
	}
	omitted := l.dropped + first
	if omitted == 0 {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf(prompts.SieveLedgerOmitted, omitted) + "\n" + strings.Join(lines[first:], "\n")
}

// formatLedgerLine builds "✓ tool arg [→ result head]" for one outcome.
func formatLedgerLine(call proxy.ToolCall, raw []byte) string {
	failed, head := classifyToolResult(raw)
	mark := ledgerOK
	if failed {
		mark = ledgerFail
	}
	parts := []string{mark, call.Function.Name}
	if arg := ledgerKeyArg(call.Function.Arguments); arg != "" {
		parts = append(parts, arg)
	}
	if (failed || ledgerEchoesResult[call.Function.Name]) && head != "" {
		parts = append(parts, "→", head)
	}
	return strings.Join(parts, " ")
}

// classifyToolResult reports whether the result is a failure — a JSON object
// carrying a non-empty "error" field, the shape the loop and tools use — and
// returns a compact one-line head of the payload. Plain-text output that merely
// mentions "error" is not a failure.
func classifyToolResult(raw []byte) (failed bool, head string) {
	if len(raw) > 0 && raw[0] == '{' {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) == nil {
			if v, ok := obj["error"]; ok && v != nil && v != "" {
				return true, compactLine(fmt.Sprint(v), ledgerResultMaxRunes)
			}
		}
	}
	text := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			text = s
		}
	}
	return false, compactLine(text, ledgerResultMaxRunes)
}

// ledgerKeyArg extracts the argument that identifies the call's target.
func ledgerKeyArg(arguments string) string {
	var args map[string]any
	if json.Unmarshal([]byte(arguments), &args) != nil {
		return ""
	}
	for _, k := range ledgerKeyArgs {
		if v, ok := args[k].(string); ok && v != "" {
			v = compactLine(v, ledgerArgMaxRunes)
			if ledgerQuotedArgs[k] {
				return `"` + v + `"`
			}
			return v
		}
	}
	return ""
}

// compactLine collapses whitespace to single spaces and clips to maxRunes.
func compactLine(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// recordLedger notes a finished tool call on the run's ledger. It is a no-op
// outside a run session.
func (a *Agent) recordLedger(call proxy.ToolCall, raw []byte) {
	if a.runS != nil {
		a.runS.ledger.record(call, raw)
	}
}

// ledgerMessage returns the progress-ledger message for a sieve to insert, or
// false when nothing has run yet. The text is built when the sieve fires and is
// not touched again until the next sieve, so the prefix stays stable between
// prunes.
func (a *Agent) ledgerMessage() (proxy.Message, bool) {
	if a.runS == nil {
		return proxy.Message{}, false
	}
	text := a.runS.ledger.render(ledgerCharBudget(a.config.ContextBudget))
	if text == "" {
		return proxy.Message{}, false
	}
	return proxy.Message{Role: proxy.UserRole, Content: prompts.SieveLedgerHeader + "\n" + text}, true
}
