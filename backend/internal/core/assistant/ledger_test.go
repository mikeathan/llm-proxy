package assistant

import (
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

func tc(name, args string) proxy.ToolCall {
	return proxy.ToolCall{ID: "id", Type: "function", Function: proxy.FunctionCall{Name: name, Arguments: args}}
}

func TestRunLedger_FormatsOutcomeKeyArgAndTerminalResult(t *testing.T) {
	var l runLedger
	l.record(tc(models.ToolTerminalExecute, `{"command":"uname -a"}`), []byte(`"Darwin mac 25.0.0\nextra"`))
	l.record(tc("write_file", `{"path":"smoke/hello.txt","content":"SECRET BODY"}`), []byte(`"File written successfully"`))
	l.record(tc(models.ToolTerminalExecute, `{"command":"npx tsc"}`), []byte(`{"error":"shell execution failed: exit status 1"}`))
	l.record(tc("internet_search", `{"query":"llm news"}`), []byte(`[{"title":"x"}]`))

	got := l.render(10_000)
	for _, want := range []string{
		`✓ execute_terminal_command "uname -a" → Darwin mac 25.0.0 extra`,
		"✓ write_file smoke/hello.txt",
		`✗ execute_terminal_command "npx tsc" → shell execution failed: exit status 1`,
		`✓ internet_search "llm news"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "SECRET BODY") {
		t.Error("file content arguments must never reach the ledger")
	}
	if strings.Contains(got, "File written successfully") {
		t.Error("only terminal results are echoed on success")
	}
}

// Real terminal output is plain text and may mention "error"; only a JSON
// object carrying an error field is a failure.
func TestRunLedger_PlainTextMentioningErrorIsNotAFailure(t *testing.T) {
	var l runLedger
	l.record(tc(models.ToolTerminalExecute, `{"command":"grep error log"}`), []byte(`"error: none found"`))
	if got := l.render(1000); !strings.HasPrefix(got, "✓") {
		t.Errorf("plain-text output must be a success, got %q", got)
	}
}

func TestRunLedger_CollapsesConsecutiveIdenticalSteps(t *testing.T) {
	var l runLedger
	for range 3 {
		l.record(tc("read_file", `{"path":"a.txt"}`), []byte(`"data"`))
	}
	l.record(tc("read_file", `{"path":"b.txt"}`), []byte(`"data"`))
	l.record(tc("read_file", `{"path":"a.txt"}`), []byte(`"data"`))

	got := l.render(1000)
	if !strings.Contains(got, "✓ read_file a.txt ×3") {
		t.Errorf("expected collapsed ×3, got:\n%s", got)
	}
	if strings.Count(got, "a.txt") != 2 {
		t.Errorf("non-consecutive repeats stay separate, got:\n%s", got)
	}
}

func TestRunLedger_IsBoundedNewestKeptAndSaysWhatWasOmitted(t *testing.T) {
	var l runLedger
	for i := range 100 {
		l.record(tc("read_file", fmt.Sprintf(`{"path":"file-%03d.txt"}`, i)), []byte(`"x"`))
	}
	const budget = 400
	got := l.render(budget)

	if len(got) > budget {
		t.Errorf("ledger is %d chars, budget %d", len(got), budget)
	}
	if !strings.Contains(got, "file-099.txt") {
		t.Error("newest step must be kept")
	}
	if strings.Contains(got, "file-000.txt") {
		t.Error("oldest step should have been cut")
	}
	if !strings.Contains(got, "earlier steps omitted") {
		t.Errorf("expected an omitted-steps line, got:\n%s", got)
	}
}

func TestRunLedger_EmptyRendersNothing(t *testing.T) {
	var l runLedger
	if got := l.render(1000); got != "" {
		t.Errorf("empty ledger must render nothing, got %q", got)
	}
}

func TestRunLedger_BoundsLongArgumentsAndResults(t *testing.T) {
	var l runLedger
	l.record(tc(models.ToolTerminalExecute, `{"command":"`+strings.Repeat("c", 500)+`"}`), []byte(`"`+strings.Repeat("r", 500)+`"`))
	if got := l.render(10_000); len(got) > 250 {
		t.Errorf("one entry must stay compact, got %d chars", len(got))
	}
}

func TestLedgerCharBudget_ScalesWithContext(t *testing.T) {
	cases := []struct {
		name   string
		budget int
		want   int
	}{
		{"8K window", 21848, 1092},
		{"16K window", 57344, ledgerMaxChars},
		{"tiny window clamps up", 1000, ledgerMinChars},
		{"unset falls back", 0, ledgerFallbackChars},
	}
	for _, c := range cases {
		if got := ledgerCharBudget(c.budget); got != c.want {
			t.Errorf("%s: ledgerCharBudget(%d) = %d, want %d", c.name, c.budget, got, c.want)
		}
	}
}

// Plan Phase 2 acceptance: building the ledger adds no meaningful latency
// (target < 1 ms per tool result). Run: go test ./internal/core/assistant/ -run xxx -bench RunLedger
func BenchmarkRunLedger_RecordAndRender(b *testing.B) {
	call := tc(models.ToolTerminalExecute, `{"command":"go build ./... && go test ./internal/core/assistant/"}`)
	raw := []byte(`"` + strings.Repeat("output line\n", 200) + `"`)
	var l runLedger
	for i := range 100 { // a long run's worth of history
		l.record(tc("read_file", fmt.Sprintf(`{"path":"f%d"}`, i)), raw)
	}
	b.ResetTimer()
	for range b.N {
		l.record(call, raw)
		_ = l.render(ledgerMaxChars)
	}
}
