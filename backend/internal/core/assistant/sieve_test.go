package assistant

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

// In-place compression runs on every turn while the prompt is over budget. A
// message that was already truncated must come back byte-identical, otherwise
// its bytes shift each turn and the llama.cpp prefix cache never survives.
func TestTruncateLongContent_IsIdempotent(t *testing.T) {
	for _, limit := range []int{compressContentMax, compressReasoningMax, 10} {
		once := truncateLongContent(strings.Repeat("abcdefghij", 1000), limit)
		twice := truncateLongContent(once, limit)
		if once != twice {
			t.Errorf("limit %d: second truncation changed the text (%d -> %d bytes)", limit, len(once), len(twice))
		}
	}
}

func ledgerMessagesIn(msgs []proxy.Message) []proxy.Message {
	var out []proxy.Message
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, prompts.SieveLedgerHeader) {
			out = append(out, m)
		}
	}
	return out
}

func longHistory(n int) []proxy.Message {
	h := []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}, {Role: proxy.UserRole, Content: "task"}, {Role: proxy.AssistantRole, Content: "ack"}}
	for i := range n {
		h = append(h, proxy.Message{Role: proxy.AssistantRole, Content: fmt.Sprintf("a%d", i)}, proxy.Message{Role: proxy.ToolRole, Content: fmt.Sprintf("r%d", i)})
	}
	return h
}

func ledgerAgent(t *testing.T) *Agent {
	t.Helper()
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5, ContextBudget: 21848})
	agent.runS = newRunSession(agent, context.Background(), nil)
	agent.recordLedger(tc("read_file", `{"path":"first-call.txt"}`), []byte(`"x"`))
	return agent
}

func TestIsAgentControlMessage_RecognisesLedger(t *testing.T) {
	agent := ledgerAgent(t)
	msg, ok := agent.ledgerMessage()
	if !ok {
		t.Fatal("expected a ledger message")
	}
	if !IsAgentControlMessage(msg) {
		t.Error("the ledger is injected by the agent and must not be mistaken for user text")
	}
}

func TestSieves_InsertLedgerAfterNote(t *testing.T) {
	sieves := map[string]func(*Agent, []proxy.Message) []proxy.Message{
		"reactive":   (*Agent).applyReactiveSieve,
		"aggressive": (*Agent).applyAggressiveSieve,
	}
	for name, apply := range sieves {
		t.Run(name, func(t *testing.T) {
			got := apply(ledgerAgent(t), longHistory(20))
			if got[sieveLockedHead].Content != prompts.SieveSystemNote {
				t.Fatalf("note must stay byte-exact at index %d, got %q", sieveLockedHead, got[sieveLockedHead].Content)
			}
			led := ledgerMessagesIn(got)
			if len(led) != 1 || got[sieveLockedHead+1].Content != led[0].Content {
				t.Fatalf("want exactly one ledger message right after the note, got %d", len(led))
			}
			if !strings.Contains(led[0].Content, "✓ read_file first-call.txt") {
				t.Errorf("ledger must list the earlier call, got %q", led[0].Content)
			}
		})
	}
}

// A second prune must replace the first ledger, not stack beside it.
func TestSieves_DoNotStackLedgers(t *testing.T) {
	agent := ledgerAgent(t)
	history := agent.applyReactiveSieve(longHistory(20))
	history = append(history, longHistory(20)[3:]...)
	history = agent.applyAggressiveSieve(history)
	if n := len(ledgerMessagesIn(history)); n != 1 {
		t.Errorf("want exactly 1 ledger after two prunes, got %d", n)
	}
}

func TestSieves_NoLedgerMessageWhenNothingRan(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5})
	agent.runS = newRunSession(agent, context.Background(), nil)
	if n := len(ledgerMessagesIn(agent.applyReactiveSieve(longHistory(20)))); n != 0 {
		t.Errorf("an empty ledger must not add a message, got %d", n)
	}
}

// probeRun drives a streaming run of toolTurns tool calls (each returning a
// 1,500-char result) against a small context budget, so the physical sieve fires
// mid-run, and returns every request the model saw. pathFor names the argument
// of each turn's call.
func probeRun(t *testing.T, toolTurns, contextBudget int, pathFor func(turn int) string) [][]proxy.Message {
	t.Helper()
	var requests [][]proxy.Message
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			requests = append(requests, append([]proxy.Message(nil), req.Messages...))
			ch := make(chan *proxy.ChatResponse, 1)
			if n := len(requests); n <= toolTurns {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
					ID: fmt.Sprintf("call_%d", n), Type: "function",
					Function: proxy.FunctionCall{Name: "probe_tool", Arguments: fmt.Sprintf(`{"path":%q}`, pathFor(n))},
				}}}}}}
			} else {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "# Done\nTask finished successfully"}}}}
			}
			close(ch)
			return ch, nil
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "probe_tool"}}}}
	engine := &MockEngine{Result: strings.Repeat("r", 1500)}
	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: toolTurns * 2, ContextBudget: contextBudget})

	if _, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.SystemRole, Content: "sys"},
		{Role: proxy.UserRole, Content: "do the task"},
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return requests
}

func countContent(msgs []proxy.Message, content string) int {
	n := 0
	for _, m := range msgs {
		if m.Role == proxy.UserRole && m.Content == content {
			n++
		}
	}
	return n
}

// End to end on the streaming path: a small window forces the physical sieve
// to drop the middle of the history mid-run. Once a call's own messages are
// gone, the model must still be told it was done — in exactly one ledger message.
func TestPhysicalSieve_LedgerCarriesDroppedSteps(t *testing.T) {
	const (
		toolTurns   = 16
		trackedTurn = 3
		trackedPath = "dropped-call.txt"
	)
	requests := probeRun(t, toolTurns, 5000, func(n int) string {
		if n == trackedTurn {
			return trackedPath
		}
		return fmt.Sprintf("file-%02d.txt", n)
	})
	carries := func(msgs []proxy.Message) bool {
		for _, m := range msgs {
			for _, c := range m.ToolCalls {
				if strings.Contains(c.Function.Arguments, trackedPath) {
					return true
				}
			}
		}
		return false
	}
	var droppedAt = -1
	for i, msgs := range requests {
		led := ledgerMessagesIn(msgs)
		if len(led) > 1 {
			t.Errorf("request %d carries %d ledger messages, want at most 1", i+1, len(led))
		}
		if droppedAt < 0 && i >= trackedTurn && !carries(msgs) {
			droppedAt = i
		}
	}
	if droppedAt < 0 {
		t.Fatalf("test premise broken: the tracked call was never dropped across %d requests", len(requests))
	}
	led := ledgerMessagesIn(requests[droppedAt])
	if len(led) != 1 || !strings.Contains(led[0].Content, "✓ probe_tool "+trackedPath) {
		t.Fatalf("request %d dropped the call's messages, so the ledger must list it; ledger messages: %v", droppedAt+1, led)
	}
	for i, msgs := range requests[droppedAt:] {
		if msgs[0].Content != requests[0][0].Content || msgs[1].Content != requests[0][1].Content {
			t.Errorf("request %d: locked head (system prompt, task) changed", droppedAt+i+1)
		}
	}
}

// With a small window the sieve fires again on later turns. Each firing must
// replace the previous note and warning, not stack beside them: a pile of
// "deliver your final answer NOW" lines is noise that eats the small window and
// shouts at the model.
func TestPhysicalSieve_KeepsOneNoteAndOneWarning(t *testing.T) {
	requests := probeRun(t, 16, 5000, func(n int) string { return fmt.Sprintf("file-%02d.txt", n) })

	firings := 0
	for i, msgs := range requests {
		notes := countContent(msgs, prompts.SieveSystemNote)
		warnings := countContent(msgs, prompts.ContextSieveWarning)
		if notes > 1 || warnings > 1 {
			t.Errorf("request %d carries %d sieve notes and %d warnings, want at most 1 of each", i+1, notes, warnings)
		}
		if warnings > 0 {
			firings++
		}
	}
	if firings == 0 {
		t.Fatal("test premise broken: the sieve never fired")
	}
}

// The "deliver your final answer NOW" warning is a one-time wrap-up cue, not a
// per-turn nag: a run that keeps working through later prunes is not told to
// stop each turn, but still gets the refreshed note and progress ledger.
func TestPhysicalSieve_WarnsToWrapUpOncePerRun(t *testing.T) {
	requests := probeRun(t, 16, 5000, func(n int) string { return fmt.Sprintf("file-%02d.txt", n) })

	withWarning, withNoteAfterWarning := 0, 0
	warnedAt := -1
	for i, msgs := range requests {
		if countContent(msgs, prompts.ContextSieveWarning) > 0 {
			withWarning++
			if warnedAt < 0 {
				warnedAt = i
			}
		} else if warnedAt >= 0 && countContent(msgs, prompts.SieveSystemNote) == 1 && len(ledgerMessagesIn(msgs)) == 1 {
			withNoteAfterWarning++
		}
	}
	if withWarning != 1 {
		t.Errorf("the warning must reach the model once per run, but %d requests carried it", withWarning)
	}
	if withNoteAfterWarning == 0 {
		t.Error("later prunes must still refresh the note and the progress ledger")
	}
}

func TestPhysicalSieve_WarnFlagControlsTheWarning(t *testing.T) {
	history := longHistory(20)
	for _, warn := range []bool{true, false} {
		s := &physicalSieve{logger: logging.NewNopLogger(), contextBudget: 10, warn: warn}
		got := s.Sieve(append([]proxy.Message(nil), history...))
		if has := countContent(got, prompts.ContextSieveWarning) == 1; has != warn {
			t.Errorf("warn=%v: warning present = %v", warn, has)
		}
		if countContent(got, prompts.SieveSystemNote) != 1 {
			t.Errorf("warn=%v: the note must always be added", warn)
		}
		if s.warned != warn {
			t.Errorf("warn=%v: sieve reported warned=%v", warn, s.warned)
		}
	}
}

// pairedHistory is a system prompt, a task and turns tool calls, each answered. A turn with batch > 1 calls
// that many tools in one assistant message, answered one by one.
func pairedHistory(turns, batch int) []proxy.Message {
	h := []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}, {Role: proxy.UserRole, Content: "task"}}
	for i := range turns {
		var calls []proxy.ToolCall
		for j := range batch {
			calls = append(calls, proxy.ToolCall{ID: fmt.Sprintf("call_%d_%d", i, j), Type: "function", Function: proxy.FunctionCall{Name: "probe_tool", Arguments: "{}"}})
		}
		h = append(h, proxy.Message{Role: proxy.AssistantRole, ToolCalls: calls})
		for _, c := range calls {
			h = append(h, proxy.Message{Role: proxy.ToolRole, ToolCallID: c.ID, Content: "result of " + c.ID})
		}
	}
	return h
}

// pairingProblems lists every tool result without an earlier call and every call without a result.
func pairingProblems(msgs []proxy.Message) []string {
	called, answered := map[string]bool{}, map[string]bool{}
	var problems []string
	for _, m := range msgs {
		for _, c := range m.ToolCalls {
			called[c.ID] = true
		}
		if m.Role == proxy.ToolRole {
			answered[m.ToolCallID] = true
			if !called[m.ToolCallID] {
				problems = append(problems, "result without a call: "+m.ToolCallID)
			}
		}
	}
	for id := range called {
		if !answered[id] {
			problems = append(problems, "call without a result: "+id)
		}
	}
	return problems
}

// A sieve drops the middle of the history; whatever it keeps must still pair every call with its result. A
// kept call whose result was dropped (a 2026-10-18 run's locked head) leaves the model an orphaned call and a
// strict chat template an error.
func TestSieves_NeverSplitACallFromItsResult(t *testing.T) {
	logger := logging.NewNopLogger()
	sieves := map[string]func() HistorySieve{
		"physical":   func() HistorySieve { return &physicalSieve{logger: logger, contextBudget: 10} },
		"reactive":   func() HistorySieve { return &reactiveSieve{logger: logger} },
		"aggressive": func() HistorySieve { return &aggressiveSieve{logger: logger} },
	}
	for name, newSieve := range sieves {
		for _, batch := range []int{1, 2, 3} {
			for turns := 1; turns <= 14; turns++ {
				got := newSieve().Sieve(pairedHistory(turns, batch))
				if problems := pairingProblems(got); len(problems) > 0 {
					t.Errorf("%s sieve, %d turns of %d calls: %v", name, turns, batch, problems)
				}
				if got[0].Role != proxy.SystemRole || got[1].Content != "task" {
					t.Errorf("%s sieve, %d turns: the system prompt and the task must stay first", name, turns)
				}
			}
		}
	}
}

// A conversation the sieve already damaged (saved before the pairing fix) must not resend its orphaned call: the
// run starts from a repaired copy, so what it measures and saves is clean too.
func TestNewRunSession_RepairsASavedOrphan(t *testing.T) {
	history := pairedHistory(3, 1)
	damaged := append([]proxy.Message{history[0], history[1], history[2]}, history[4:]...) // call_0_0's result gone
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5, ContextBudget: 21848, UseNativeTools: new(true)})
	s := newRunSession(agent, context.Background(), damaged)
	if problems := pairingProblems(s.history); len(problems) > 0 {
		t.Errorf("the run's history still carries a broken pair: %v", problems)
	}
	if len(damaged) != len(history)-1 {
		t.Error("the caller's history must not be modified")
	}
}

// laterTurnHistory is a conversation whose earlier question was answered after a tool call, then a new question
// (question) and turns tool calls working on it, each answered. The sieve's locked head holds the first turns of a
// conversation, so the new question lies in the middle that a long tool loop makes it drop.
func laterTurnHistory(question string, turns int) []proxy.Message {
	h := []proxy.Message{
		{Role: proxy.SystemRole, Content: "sys"},
		{Role: proxy.UserRole, Content: "first question"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: "old_call", Type: "function", Function: proxy.FunctionCall{Name: "probe_tool", Arguments: "{}"}}}},
		{Role: proxy.ToolRole, ToolCallID: "old_call", Content: "old result"},
		{Role: proxy.AssistantRole, Content: "first answer"},
		{Role: proxy.UserRole, Content: question},
	}
	return append(h, pairedHistory(turns, 1)[2:]...)
}

// The question being worked on survives every sieve, however long the tool loop is. A 2026-10-22 chat's second
// question ("research how to build an agent harness in Go") was dropped in the middle after four steps, and the model
// went back to answering the first question, the only one it could still see.
func TestSieves_KeepTheCurrentQuestion(t *testing.T) {
	const question = "second question: research how to build an agent harness in Go"
	logger := logging.NewNopLogger()
	sieves := map[string]func() HistorySieve{
		"physical":   func() HistorySieve { return &physicalSieve{logger: logger, contextBudget: 10} },
		"reactive":   func() HistorySieve { return &reactiveSieve{logger: logger} },
		"aggressive": func() HistorySieve { return &aggressiveSieve{logger: logger} },
	}
	for name, newSieve := range sieves {
		for turns := 1; turns <= 14; turns++ {
			got := newSieve().Sieve(laterTurnHistory(question, turns))
			if n := countContent(got, question); n != 1 {
				t.Errorf("%s sieve, %d turns: the current question appears %d times, want once", name, turns, n)
			}
			if problems := pairingProblems(got); len(problems) > 0 {
				t.Errorf("%s sieve, %d turns: %v", name, turns, problems)
			}
			if got[0].Role != proxy.SystemRole || got[1].Content != "first question" {
				t.Errorf("%s sieve, %d turns: the system prompt and the first question stay first", name, turns)
			}
			note := slices.IndexFunc(got, isSieveMessage)
			asked := slices.IndexFunc(got, func(m proxy.Message) bool { return m.Content == question })
			if note >= 0 && asked > note && turns > 4 {
				t.Errorf("%s sieve, %d turns: the question comes after the sieve note, the model reads it as a log line", name, turns)
			}
		}
	}
}

// A sieve changes nothing about a first question: it is already in the locked head, so it is not kept twice.
func TestSieves_FirstQuestionIsNotDuplicated(t *testing.T) {
	logger := logging.NewNopLogger()
	for turns := 1; turns <= 14; turns++ {
		got := (&physicalSieve{logger: logger, contextBudget: 10}).Sieve(pairedHistory(turns, 1))
		if n := countContent(got, "task"); n != 1 {
			t.Errorf("%d turns: the task appears %d times", turns, n)
		}
	}
}

// A long question is the task, not log: the physical sieve's compression of old messages leaves it whole.
func TestPhysicalSieve_DoesNotCompressTheCurrentQuestion(t *testing.T) {
	question := "summarise this document: " + strings.Repeat("a long pasted paragraph. ", 400)
	history := laterTurnHistory(question, 8)
	got := (&physicalSieve{logger: logging.NewNopLogger(), contextBudget: len(question) + 3000}).Sieve(history)
	if countContent(got, question) != 1 {
		t.Errorf("the long question was cut or lost (kept %d bytes of %d)", len(strings.Join(contents(got), "")), len(question))
	}
}

func contents(msgs []proxy.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Content
	}
	return out
}
