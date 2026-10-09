package assistant

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// streamingToolRun drives a streaming run of toolTurns tool calls followed by
// a final answer and returns every request the model saw. It uses a real
// StreamFunc: the default MockClient fails Stream and so exercises the
// non-streaming fallback, which would mask streaming-path defects.
func streamingToolRun(t *testing.T, store *memory.Store, opts AgentOptions, toolTurns int) [][]proxy.Message {
	t.Helper()
	requests, _ := streamingToolRunWith(t, store, opts, "how should I build?", toolTurns)
	return requests
}

// streamingToolRunWith is streamingToolRun for a chosen user message; it also returns the history Execute hands back.
func streamingToolRunWith(t *testing.T, store *memory.Store, opts AgentOptions, message string, toolTurns int) ([][]proxy.Message, []proxy.Message) {
	t.Helper()
	var requests [][]proxy.Message
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			requests = append(requests, append([]proxy.Message(nil), req.Messages...))
			ch := make(chan *proxy.ChatResponse, 1)
			if len(requests) <= toolTurns {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
					ID:       fmt.Sprintf("call_%d", len(requests)),
					Type:     "function",
					Function: proxy.FunctionCall{Name: "read_file", Arguments: fmt.Sprintf(`{"path":"f%d.txt"}`, len(requests))},
				}}}}}}
			} else {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "# Done\nTask finished successfully"}}}}
			}
			close(ch)
			return ch, nil
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}}}}
	opts.MemoryStore = store
	opts.EnableHotMemory = true
	if opts.MaxSteps == 0 {
		opts.MaxSteps = 6
	}
	agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, opts)
	_, history, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: message},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(requests) != toolTurns+1 {
		t.Fatalf("expected %d requests, got %d", toolTurns+1, len(requests))
	}
	return requests, history
}

// TestHotMemory_PresentEveryTurn is the M1 acceptance test
// (docs/PLANS/memory/small-context-memory.md, Phase 0/1.1): the model sees the
// hot memory on every request of a run, exactly once, inside the head system
// message so the prompt prefix stays byte-identical and the KV cache is reused.
func TestHotMemory_PresentEveryTurn(t *testing.T) {
	store := newTestMemoryStore(t)
	if _, err := store.Insert(context.Background(), "ws-1", memory.LongTerm, "build", "run go build ./... to verify", []string{"hot"}, "agent"); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	requests := streamingToolRun(t, store, AgentOptions{WorkspaceID: "ws-1"}, 2)

	head := requests[0][0].Content
	if requests[0][0].Role != proxy.SystemRole || !strings.Contains(head, "<memory>") || !strings.Contains(head, "go build") {
		t.Fatalf("request 1: hot memory must be inside the head system message, got %q", head)
	}
	for i, msgs := range requests {
		if msgs[0].Content != head {
			t.Errorf("request %d: head system message differs from request 1 (prefix not byte-stable)", i+1)
		}
		for j, m := range msgs[1:] {
			if strings.Contains(m.Content, "<memory>") {
				t.Errorf("request %d: stray <memory> in message %d (role %s)", i+1, j+1, m.Role)
			}
		}
	}
}

func TestHotMemory_SizedFromServingContext(t *testing.T) {
	cases := []struct {
		name     string
		budget   int
		workload models.WorkloadClass
		want     int
	}{
		{"local 8K window", 21848, models.WorkloadLocal, 1747},
		{"local 16K window", 57344, models.WorkloadLocal, 4587},
		{"local 32K window hits max", 122880, models.WorkloadLocal, hotMemoryMaxChars},
		{"local 4K window", 8192, models.WorkloadLocal, 655},
		{"tiny window clamps to min", 1000, models.WorkloadLocal, hotMemoryMinChars},
		{"cloud 128K hits max", 512000, models.WorkloadCloud, hotMemoryMaxChars},
		{"cloud mid window uses lower share", 40000, models.WorkloadCloud, 2000},
		{"unset budget falls back", 0, models.WorkloadLocal, hotMemoryFallbackChars},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hotMemoryCharBudget(tc.budget, tc.workload); got != tc.want {
				t.Errorf("hotMemoryCharBudget(%d, %s) = %d, want %d", tc.budget, tc.workload, got, tc.want)
			}
		})
	}
}

// injectionText is buildHotInjection without the kept count, for tests that
// only look at the rendered text.
func injectionText(entries []memory.MemoryEntry, maxChars int) string {
	text, _ := buildHotInjection(entries, maxChars)
	return text
}

func manyHotEntries(n int) []memory.MemoryEntry {
	entries := make([]memory.MemoryEntry, n)
	for i := range entries {
		entries[i] = memory.MemoryEntry{Title: fmt.Sprintf("fact-%02d", i), Content: strings.Repeat("x", 150)}
	}
	return entries
}

func TestBuildHotInjection_RespectsBudgetAndSaysWhatWasDropped(t *testing.T) {
	const budget = 1000
	got := injectionText(manyHotEntries(30), budget)

	if len(got) > budget {
		t.Errorf("block is %d chars, budget %d", len(got), budget)
	}
	if !strings.Contains(got, "fact-00") {
		t.Error("newest entry must survive")
	}
	if strings.Contains(got, "fact-29") {
		t.Error("oldest entry should have been cut")
	}
	if !strings.Contains(got, "more saved facts") || !strings.Contains(got, models.ToolMemorySearch) {
		t.Errorf("expected overflow hint naming %s, got tail %q", models.ToolMemorySearch, got[max(0, len(got)-120):])
	}
}

func TestBuildHotInjection_NoHintWhenEverythingFits(t *testing.T) {
	got := injectionText(manyHotEntries(2), 4000)
	if strings.Contains(got, "more saved facts") {
		t.Errorf("unexpected overflow hint: %q", got)
	}
}

func TestBuildHotInjection_AlwaysKeepsAtLeastOneFact(t *testing.T) {
	big := []memory.MemoryEntry{{Title: "huge", Content: strings.Repeat("y", 5000)}, {Title: "other", Content: "z"}}
	got := injectionText(big, hotMemoryMinChars)
	if got == "" {
		t.Fatal("a budget smaller than the newest fact must not drop memory silently")
	}
	if !strings.Contains(got, "more saved facts") {
		t.Errorf("expected overflow hint, got %q", got)
	}
}

// memoryBlockOf extracts the <memory>…</memory> block from a request head.
func memoryBlockOf(t *testing.T, head string) string {
	t.Helper()
	start := strings.Index(head, "<memory>")
	end := strings.Index(head, "</memory>")
	if start < 0 || end < start {
		t.Fatalf("no <memory> block in head: %q", head)
	}
	return head[start : end+len("</memory>")]
}

// The window-derived budget is the whole point on small models: with the same
// 60 hot facts, an 8K window gets a small block, a 16K window gets a larger
// one, and neither exceeds the share it is allowed.
func TestHotMemory_BlockScalesWithContextWindow(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	for _, e := range manyHotEntries(60) {
		if _, err := store.Insert(ctx, "ws-1", memory.LongTerm, e.Title, e.Content, []string{"hot"}, "agent"); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	blockLen := func(contextBudget int) int {
		reqs := streamingToolRun(t, store, AgentOptions{WorkspaceID: "ws-1", ContextBudget: contextBudget, WorkloadClass: models.WorkloadLocal}, 0)
		block := memoryBlockOf(t, reqs[0][0].Content)
		if limit := hotMemoryCharBudget(contextBudget, models.WorkloadLocal); len(block) > limit {
			t.Errorf("budget %d: block is %d chars, limit %d", contextBudget, len(block), limit)
		}
		return len(block)
	}
	small, large := blockLen(21848), blockLen(57344)
	if large <= small {
		t.Errorf("a 16K window must admit more memory than an 8K one: 8K block=%d, 16K block=%d", small, large)
	}
}

// preparedOverContextBudget must count the memory block: it is now in every
// request, so excluding it would make the sieve fire late and push 8K models
// into the reactive overflow path.
func TestPreparedOverContextBudget_CountsHotMemory(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     newTestMemoryStore(t),
		EnableHotMemory: true,
	})
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "sys"},
		{Role: proxy.UserRole, Content: "hello"},
	}
	agent.runS = newRunSession(agent, context.Background(), history)

	prepared, _ := agent.prepareMessagesForTurn(history, nil, nil)
	base := 0
	for _, m := range prepared {
		base += messageChars(m)
	}
	agent.config.ContextBudget = base + 100

	if agent.preparedOverContextBudget(history, nil) {
		t.Fatal("without a memory block the request fits")
	}
	agent.runS.prompt.memoryBlock = "<memory>\n" + strings.Repeat("m", 500) + "\n</memory>"
	if !agent.preparedOverContextBudget(history, nil) {
		t.Error("the 500-char memory block must be counted against the budget")
	}
}

func TestHotMemoryOverflowHint_IsCentralised(t *testing.T) {
	if !strings.Contains(prompts.HotMemoryOverflowHint, "%d") {
		t.Error("hint must carry the dropped count")
	}
}

// The operator preview is only worth having if it cannot lie: for the same
// facts and the same model settings it must be byte-identical to the block the
// run actually puts in the head system message.
func TestPreviewHotMemory_MatchesWhatTheRunInjects(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	for _, e := range manyHotEntries(60) {
		if _, err := store.Insert(ctx, "ws-1", memory.LongTerm, e.Title, e.Content, []string{"hot"}, "agent"); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	opts := AgentOptions{WorkspaceID: "ws-1", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}

	reqs := streamingToolRun(t, store, opts, 0)
	injected := memoryBlockOf(t, reqs[0][0].Content)

	entries, err := store.SearchHot(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	preview := PreviewHotMemory(entries, memory.OperatorNotes{}, opts)
	if preview.Block != injected {
		t.Errorf("preview differs from the injected block:\npreview  %q\ninjected %q", preview.Block, injected)
	}
}

func TestPreviewHotMemory_ReportsSizeIncludedAndCut(t *testing.T) {
	entries := manyHotEntries(30)
	for i := range entries {
		entries[i].ID = int64(i + 1)
	}
	opts := AgentOptions{ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}
	p := PreviewHotMemory(entries, memory.OperatorNotes{}, opts)

	if len(p.Included)+len(p.Cut) != len(entries) || len(p.Included) == 0 || len(p.Cut) == 0 {
		t.Fatalf("included %d + cut %d must cover all %d entries and both be non-empty", len(p.Included), len(p.Cut), len(entries))
	}
	if p.Included[0].ID != 1 || p.Cut[0].ID != int64(len(p.Included)+1) {
		t.Errorf("order must be preserved newest-first: included starts %d, cut starts %d", p.Included[0].ID, p.Cut[0].ID)
	}
	if p.Chars != len(p.Block) || p.TokensEstimate != p.Chars/4 {
		t.Errorf("chars=%d tokens=%d for a %d-char block", p.Chars, p.TokensEstimate, len(p.Block))
	}
	if want := hotMemoryCharBudget(21848, models.WorkloadLocal); p.BudgetChars != want || p.Chars > want {
		t.Errorf("budget=%d chars=%d, want budget %d and chars within it", p.BudgetChars, p.Chars, want)
	}
	if p.ContextBudgetChars != 21848 {
		t.Errorf("ContextBudgetChars=%d, want 21848", p.ContextBudgetChars)
	}
}

func TestPreviewHotMemory_UnresolvedBudgetAndNoFacts(t *testing.T) {
	p := PreviewHotMemory(nil, memory.OperatorNotes{}, AgentOptions{})
	if p.Block != "" || p.Included == nil || p.Cut == nil {
		t.Errorf("no facts: empty block and non-nil lists (JSON arrays), got %+v", p)
	}
	if p.BudgetChars != hotMemoryFallbackChars || p.ContextBudgetChars != 0 {
		t.Errorf("an unresolved model budget must say so: budget=%d context=%d", p.BudgetChars, p.ContextBudgetChars)
	}
}

// ── Operator notes (MEMORY.md) ──────────────────────────────────────────────

var localOpts = AgentOptions{ContextBudget: 21848, WorkloadClass: models.WorkloadLocal} // ~1,747-char memory budget

func TestPreviewHotMemory_OperatorNotesGoFirstThenSavedFacts(t *testing.T) {
	notes := memory.OperatorNotes{Global: "Reply in British English.", Workspace: "Always use tabs."}
	p := PreviewHotMemory(manyHotEntries(2), notes, localOpts)

	opHdr := strings.Index(p.Block, prompts.HotMemoryOperatorHeader)
	global := strings.Index(p.Block, "Reply in British English.")
	ws := strings.Index(p.Block, "Always use tabs.")
	saved := strings.Index(p.Block, prompts.HotMemorySavedHeader)
	fact := strings.Index(p.Block, "- fact-00")
	if !(0 <= opHdr && opHdr < global && global < ws && ws < saved && saved < fact) {
		t.Errorf("want operator header < global < workspace < saved header < first fact, got %d %d %d %d %d:\n%s", opHdr, global, ws, saved, fact, p.Block)
	}
	if !strings.HasPrefix(p.Block, "<memory>\n") || !strings.HasSuffix(p.Block, "\n</memory>") {
		t.Errorf("block must stay wrapped in <memory> tags: %q", p.Block)
	}
	if p.OperatorChars != len(notes.Text()) {
		t.Errorf("OperatorChars=%d, want %d", p.OperatorChars, len(notes.Text()))
	}
}

func TestPreviewHotMemory_NotesOnlyHasNoSavedFactsHeader(t *testing.T) {
	p := PreviewHotMemory(nil, memory.OperatorNotes{Workspace: "Be brief."}, localOpts)
	if !strings.Contains(p.Block, "Be brief.") || strings.Contains(p.Block, prompts.HotMemorySavedHeader) {
		t.Errorf("notes-only block wrong: %q", p.Block)
	}
}

func TestPreviewHotMemory_BlankNotesAddNoSections(t *testing.T) {
	entries := manyHotEntries(2)
	withEmpty := PreviewHotMemory(entries, memory.OperatorNotes{Global: "  ", Workspace: ""}, localOpts)
	if strings.Contains(withEmpty.Block, prompts.HotMemoryOperatorHeader) || strings.Contains(withEmpty.Block, prompts.HotMemorySavedHeader) {
		t.Errorf("blank notes must add nothing: %q", withEmpty.Block)
	}
}

// The operator wrote the notes on purpose: they are never clipped, even when
// they alone exceed the model's memory budget. The agent's facts give way.
func TestPreviewHotMemory_NotesAreNeverCutAndFactsGiveWay(t *testing.T) {
	big := strings.Repeat("Rule. ", 700) // ~4,200 chars vs a ~1,747-char budget
	notes := memory.OperatorNotes{Workspace: big}
	p := PreviewHotMemory(manyHotEntries(5), notes, localOpts)

	if !strings.Contains(p.Block, strings.TrimSpace(big)) {
		t.Error("operator notes must appear whole")
	}
	if len(p.Included) != 0 || len(p.Cut) != 5 {
		t.Errorf("no room is left for facts: included %d, cut %d", len(p.Included), len(p.Cut))
	}
	if !strings.Contains(p.Block, "5 more saved facts") {
		t.Errorf("the model must still be told facts were left out: tail %q", p.Block[max(0, len(p.Block)-120):])
	}
	if !p.OverBudget {
		t.Error("the preview must flag that the notes alone exceed the budget")
	}
}

func TestPreviewHotMemory_FactsShareTheBudgetNotesLeaveBehind(t *testing.T) {
	notes := memory.OperatorNotes{Workspace: strings.Repeat("n", 500)}
	p := PreviewHotMemory(manyHotEntries(30), notes, localOpts)

	if p.OverBudget || p.Chars > p.BudgetChars {
		t.Errorf("small notes plus cut facts must fit the budget: %d chars of %d", p.Chars, p.BudgetChars)
	}
	if len(p.Included) == 0 || len(p.Cut) == 0 {
		t.Errorf("some facts fit and some do not: included %d, cut %d", len(p.Included), len(p.Cut))
	}
	without := PreviewHotMemory(manyHotEntries(30), memory.OperatorNotes{}, localOpts)
	if len(p.Included) >= len(without.Included) {
		t.Errorf("notes use budget, so fewer facts fit: with %d, without %d", len(p.Included), len(without.Included))
	}
}

// End to end: the run puts the notes in the head system message, identically on
// every turn, and the preview is byte-identical to it.
func TestHotMemory_OperatorNotesReachEveryTurnAndMatchThePreview(t *testing.T) {
	store := newTestMemoryStore(t)
	root := t.TempDir()
	store.SetNotesLocations(filepath.Join(root, "MEMORY.md"), func(ws string) string { return filepath.Join(root, ws, "MEMORY.md") })
	ctx := context.Background()
	if err := store.SetOperatorNotes(ctx, memory.NotesGlobal, "", "Reply in British English."); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOperatorNotes(ctx, memory.NotesWorkspace, "ws-1", "Always use tabs."); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build", "run go build", []string{memory.HotTag}, "agent"); err != nil {
		t.Fatal(err)
	}
	opts := AgentOptions{WorkspaceID: "ws-1", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}

	reqs := streamingToolRun(t, store, opts, 2)
	head := reqs[0][0].Content
	if !strings.Contains(head, "Reply in British English.") || !strings.Contains(head, "Always use tabs.") {
		t.Fatalf("operator notes missing from the head message: %q", head)
	}
	for i, msgs := range reqs {
		if msgs[0].Content != head {
			t.Errorf("request %d: head changed (prefix not byte-stable)", i+1)
		}
	}

	entries, _ := store.SearchHot(ctx, "ws-1")
	notes, _ := store.OperatorNotes(ctx, "ws-1")
	if preview := PreviewHotMemory(entries, notes, opts); preview.Block != memoryBlockOf(t, head) {
		t.Errorf("preview differs from the injected block:\npreview  %q\ninjected %q", preview.Block, memoryBlockOf(t, head))
	}
}

// Notes alone are enough: a workspace with no saved facts still gets them.
func TestHotMemory_NotesWithoutAnyHotFactsStillInject(t *testing.T) {
	store := newTestMemoryStore(t)
	root := t.TempDir()
	store.SetNotesLocations(filepath.Join(root, "MEMORY.md"), func(ws string) string { return filepath.Join(root, ws, "MEMORY.md") })
	if err := store.SetOperatorNotes(context.Background(), memory.NotesWorkspace, "ws-1", "Be brief."); err != nil {
		t.Fatal(err)
	}
	reqs := streamingToolRun(t, store, AgentOptions{WorkspaceID: "ws-1"}, 0)
	if !strings.Contains(reqs[0][0].Content, "Be brief.") {
		t.Errorf("notes-only workspace got no memory block: %q", reqs[0][0].Content)
	}
}

// The point of priority: when a small window cuts the tail, an old fact the
// operator marked high must still be there, and newer normal ones give way.
func TestHotMemory_HighPriorityFactSurvivesTheBudgetCut(t *testing.T) {
	store, rawDB := newTestMemoryStoreAndDB(t)
	ctx := context.Background()
	var oldestID int64
	for i, e := range manyHotEntries(60) { // inserted oldest-first, so fact-00 is the oldest
		id, err := store.Insert(ctx, "ws-1", memory.LongTerm, e.Title, e.Content, []string{memory.HotTag}, "agent")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldestID = id
		}
	}
	// Recency must be explicit: same-second inserts tie on updated_at.
	for i := range 60 {
		rawDB.ExecContext(ctx, "UPDATE memories SET updated_at = datetime('2026-01-01', ?) WHERE title = ?", fmt.Sprintf("+%d minutes", i), fmt.Sprintf("fact-%02d", i))
	}
	opts := AgentOptions{WorkspaceID: "ws-1", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}

	before := memoryBlockOf(t, streamingToolRun(t, store, opts, 0)[0][0].Content)
	if strings.Contains(before, "fact-00") {
		t.Fatal("test premise broken: the oldest fact should be cut at normal priority")
	}
	if err := store.SetPriority(ctx, "ws-1", oldestID, memory.PriorityHigh); err != nil {
		t.Fatal(err)
	}
	after := memoryBlockOf(t, streamingToolRun(t, store, opts, 0)[0][0].Content)
	if !strings.Contains(after, "fact-00") {
		t.Errorf("a high-priority fact must survive the cut:\n%s", after)
	}
}

func TestPreviewHotMemory_CarriesPriorityPerEntry(t *testing.T) {
	entries := manyHotEntries(2)
	entries[0].Priority = memory.PriorityHigh
	entries[1].Priority = memory.PriorityLow
	p := PreviewHotMemory(entries, memory.OperatorNotes{}, localOpts)
	if len(p.Included) != 2 || p.Included[0].Priority != memory.PriorityHigh || p.Included[1].Priority != memory.PriorityLow {
		t.Errorf("preview entries must report priority: %+v", p.Included)
	}
}

// ── Usage counters ──────────────────────────────────────────────────────────

// "Sent" means in the block the model saw: a fact the budget cut was not used,
// and a run counts once however many turns it takes.
func TestHotMemory_UsageCountsOnlyFactsThatWereSentOncePerRun(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	for _, e := range manyHotEntries(60) {
		if _, err := store.Insert(ctx, "ws-1", memory.LongTerm, e.Title, e.Content, []string{memory.HotTag}, "agent"); err != nil {
			t.Fatal(err)
		}
	}
	opts := AgentOptions{WorkspaceID: "ws-1", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}

	reqs := streamingToolRun(t, store, opts, 2) // three requests in one run
	block := memoryBlockOf(t, reqs[0][0].Content)
	if err := store.FlushUsage(ctx); err != nil {
		t.Fatal(err)
	}

	all, _ := store.List(ctx, "ws-1", "", 100, 0)
	var sent, cut int
	for _, e := range all {
		inBlock := strings.Contains(block, "- "+e.Title+":")
		switch {
		case inBlock && e.InjectedCount == 1:
			sent++
		case !inBlock && e.InjectedCount == 0:
			cut++
		default:
			t.Errorf("fact %s: inBlock=%v injected=%d (sent facts must count exactly once per run, cut facts never)", e.Title, inBlock, e.InjectedCount)
		}
	}
	if sent == 0 || cut == 0 {
		t.Errorf("test premise: some facts sent (%d) and some cut (%d)", sent, cut)
	}
}

func TestHotMemory_PreviewDoesNotCountAsUse(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	store.Insert(ctx, "ws-1", memory.LongTerm, "a", "x", []string{memory.HotTag}, "agent")
	entries, _ := store.SearchHot(ctx, "ws-1")

	PreviewHotMemory(entries, memory.OperatorNotes{}, localOpts)
	store.FlushUsage(ctx)
	if got, _ := store.List(ctx, "ws-1", "", 10, 0); got[0].InjectedCount != 0 {
		t.Errorf("a preview is not a run: injected=%d", got[0].InjectedCount)
	}
}

// A fact saved without an explicit title gets its first 60 characters as the
// title, so rendering "- Title: Content" printed the same words twice. On a small
// window that doubles the cost of every short fact for nothing.
func TestBuildHotInjection_DoesNotRepeatAnAutoDerivedTitle(t *testing.T) {
	long := "Installed: Node v26.0.0, npm 11.12.1. This machine runs Darwin 27.0.0 and more words follow"
	entries := []memory.MemoryEntry{
		{Title: "My birthday is January 1st.", Content: "My birthday is January 1st."},          // title == content
		{Title: long[:60], Content: long},                                                          // title is a prefix of content
		{Title: "build", Content: "run go build ./... to verify"},                                  // an explicit title is kept
		{Title: "", Content: "no title at all"},                                                    // blank title
	}
	got := injectionText(entries, 10_000)
	want := "- My birthday is January 1st.\n- " + long + "\n- build: run go build ./... to verify\n- no title at all\n"
	if got != want {
		t.Errorf("block =\n%q\nwant\n%q", got, want)
	}
}

// firstRequestHead runs one turn and returns the head system message of the first model request.
func firstRequestHead(t *testing.T, store *memory.Store, opts AgentOptions) string {
	t.Helper()
	var head string
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			if head == "" && len(req.Messages) > 0 {
				head = req.Messages[0].Content
			}
			ch := make(chan *proxy.ChatResponse, 1)
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "# Done\nTask finished successfully"}}}}
			close(ch)
			return ch, nil
		},
	}
	opts.MemoryStore = store
	if opts.MaxSteps == 0 {
		opts.MaxSteps = 4
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{Result: "ok"}, opts)
	if _, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "how should I build?"},
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return head
}

// The save guidance steers a model that can follow it, so it reaches only the operator's own assistant chats with
// memory on: not automations (unattended), not connector chats (an outside sender must not be able to push the model
// into saving), not a run without memory.
func TestSaveGuidance_ReachesOnlyTheOperatorsOwnChatsWithMemoryOn(t *testing.T) {
	cases := []struct {
		name string
		opts AgentOptions
		want bool
	}{
		{"the operator's chat", AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: true, ConversationID: "conv_1"}, true},
		{"a chat with no conversation id yet", AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: true}, true},
		{"memory switched off", AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: false, ConversationID: "conv_1"}, false},
		{"an automation", AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: true, Channel: ChannelAutomation}, false},
		{"a connector chat", AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: true, ConversationID: "wb_telegram_chat42"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := firstRequestHead(t, newTestMemoryStore(t), tc.opts)
			if got := strings.Contains(head, prompts.MemorySaveGuidance); got != tc.want {
				t.Errorf("guidance present = %v, want %v: %q", got, tc.want, head)
			}
		})
	}
}

// It must not depend on memory already holding a hot fact (the first save would never be nudged), and it follows the
// memory block so the block stays the stable head of the memory text.
func TestSaveGuidance_FollowsTheMemoryBlockAndNeedsNoHotFact(t *testing.T) {
	opts := AgentOptions{WorkspaceID: "ws-1", EnableHotMemory: true, ConversationID: "conv_1"}
	if head := firstRequestHead(t, newTestMemoryStore(t), opts); !strings.Contains(head, prompts.MemorySaveGuidance) || strings.Contains(head, "<memory>") {
		t.Errorf("with no hot facts the head must carry the guidance and no empty block: %q", head)
	}

	store := newTestMemoryStore(t)
	if _, err := store.Insert(context.Background(), "ws-1", memory.LongTerm, "build", "run go build ./... to verify", []string{"hot"}, "agent"); err != nil {
		t.Fatal(err)
	}
	head := firstRequestHead(t, store, opts)
	block, guidance := strings.Index(head, "</memory>"), strings.Index(head, prompts.MemorySaveGuidance)
	if block < 0 || guidance < block {
		t.Errorf("the guidance must come after the memory block (block %d, guidance %d)", block, guidance)
	}
}

// Within a run the head must be byte-identical on every request, or the server's prompt cache is lost.
func TestSaveGuidance_IsByteStableAcrossARunsRequests(t *testing.T) {
	requests := streamingToolRun(t, newTestMemoryStore(t), AgentOptions{WorkspaceID: "ws-1", ConversationID: "conv_1"}, 2)
	for i := 1; i < len(requests); i++ {
		if requests[i][0].Content != requests[0][0].Content {
			t.Fatalf("request %d head differs from request 1", i+1)
		}
	}
	if !strings.Contains(requests[0][0].Content, prompts.MemorySaveGuidance) {
		t.Error("the guidance is missing from the head")
	}
}
