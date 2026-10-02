package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
)

type fixtureMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type fixtureTool struct {
	Name string
	Args string
}

// writeRun builds a run directory: one recording request per element of
// requests, one tool_call event per element of calls (a step_start precedes
// each request), timestamps 1s apart starting at base.
func writeRun(t *testing.T, requests [][]fixtureMsg, calls []fixtureTool) string {
	t.Helper()
	dir := t.TempDir()
	base := time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)

	var rec strings.Builder
	for _, msgs := range requests {
		line, _ := json.Marshal(map[string]any{"type": "request", "model": "m", "messages": msgs})
		rec.Write(line)
		rec.WriteByte('\n')
		rec.WriteString(`{"type":"chunk","choices":[]}` + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "recording.jsonl"), []byte(rec.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	var ev strings.Builder
	emit := func(sec int, typ string, payload any) {
		p, _ := json.Marshal(payload)
		fmt.Fprintf(&ev, `{"id":"%d","type":"%s","payload":%s,"timestamp":"%s"}`+"\n", sec, typ, p, base.Add(time.Duration(sec)*time.Second).Format(time.RFC3339Nano))
	}
	sec := 0
	for i := range requests {
		emit(sec, "step_start", map[string]int{"step": i + 1})
		sec += 5 // turn-1 latency is 5s when the first call follows step 1
		if i < len(calls) {
			emit(sec, "tool_call", map[string]any{"id": fmt.Sprint("c", i), "function": map[string]string{"name": calls[i].Name, "arguments": calls[i].Args}})
		}
		sec++
	}
	for i := len(requests); i < len(calls); i++ {
		emit(sec, "tool_call", map[string]any{"id": fmt.Sprint("c", i), "function": map[string]string{"name": calls[i].Name, "arguments": calls[i].Args}})
		sec++
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(ev.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"model": "m", "task": "t", "duration_ms": 9000, "llm_calls": len(requests), "tool_calls": len(calls)})
	if err := os.WriteFile(filepath.Join(dir, "run-meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const memBlock = "<memory>\n- build: run go build ./...\n</memory>"

func headWithMemory() fixtureMsg {
	return fixtureMsg{"system", "prompt\n\n" + memBlock}
}

func grow(n int) []fixtureMsg {
	msgs := []fixtureMsg{headWithMemory(), {"user", "task"}}
	for i := 0; i < n; i++ {
		msgs = append(msgs, fixtureMsg{"assistant", "a"}, fixtureMsg{"tool", "r"})
	}
	return msgs
}

func TestAnalyze_MemoryPresentInEveryRequest(t *testing.T) {
	dir := writeRun(t, [][]fixtureMsg{grow(0), grow(1), grow(2)}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.Requests != 3 || row.MemoryRequests != 3 {
		t.Errorf("requests=%d memoryRequests=%d, want 3/3", row.Requests, row.MemoryRequests)
	}
	if want := len(memBlock) / charsPerToken; row.MemoryTokensAvg != want {
		t.Errorf("MemoryTokensAvg=%d, want %d", row.MemoryTokensAvg, want)
	}
}

// The pre-fix defect signature: memory only on request 1.
func TestAnalyze_MemoryOnlyOnFirstRequestIsVisible(t *testing.T) {
	noMem := func(n int) []fixtureMsg { return grow(n)[1:] }
	dir := writeRun(t, [][]fixtureMsg{grow(0), noMem(1), noMem(2)}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.MemoryRequests != 1 || row.Requests != 3 {
		t.Errorf("memoryRequests=%d of %d, want 1 of 3", row.MemoryRequests, row.Requests)
	}
}

func TestAnalyze_CountsSieveFiringWhenHistoryShrinks(t *testing.T) {
	dir := writeRun(t, [][]fixtureMsg{grow(0), grow(3), grow(1), grow(2)}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.SieveFirings != 1 {
		t.Errorf("SieveFirings=%d, want 1", row.SieveFirings)
	}
}

// The sieve compresses long messages in place before it drops any, so the
// message count can stay flat while the prompt shrinks.
func TestAnalyze_CountsSieveFiringWhenPromptShrinksInPlace(t *testing.T) {
	long := grow(2)
	long[2].Content = strings.Repeat("x", 4000)
	compressed := grow(2)
	compressed[2].Content = "x...[Truncated]...x"
	dir := writeRun(t, [][]fixtureMsg{grow(0), long, compressed}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.SieveFirings != 1 {
		t.Errorf("SieveFirings=%d, want 1 (same message count, fewer chars)", row.SieveFirings)
	}
}

// Seen in a real run (2026-10-01, remote model, 50K-char budget): the history grew
// past the budget in one turn because many large tool results arrived together, so
// the sieve dropped the middle AND the request still ended up larger (9K -> 35K
// chars). Fewer messages plus a new sieve note is a prune even though characters grew.
func TestAnalyze_PruneThatIsOutweighedByNewResultsIsStillCounted(t *testing.T) {
	before := grow(4)
	noted := append([]fixtureMsg(nil), before[:3]...)
	noted = append(noted, fixtureMsg{"user", "[System Note: History distilled to save context. DO NOT repeat]"})
	noted = append(noted, fixtureMsg{"tool", strings.Repeat("big tool result ", 800)})
	dir := writeRun(t, [][]fixtureMsg{grow(0), before, noted}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.SieveFirings != 1 {
		t.Errorf("SieveFirings=%d, want 1 (fewer messages and a new sieve note)", row.SieveFirings)
	}
}

func TestAnalyze_CountsSieveFiringFromNote(t *testing.T) {
	noted := append(grow(1), fixtureMsg{"system", "[System Note: History distilled to save context. DO NOT repeat]"})
	dir := writeRun(t, [][]fixtureMsg{grow(0), grow(1), noted, append(noted, fixtureMsg{"user", "x"})}, nil)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.SieveFirings != 1 {
		t.Errorf("SieveFirings=%d, want 1 (note appears once, not per request)", row.SieveFirings)
	}
}

func TestAnalyze_RepeatedCallsAndRepeatsAfterSieve(t *testing.T) {
	calls := []fixtureTool{
		{"terminal", `{"command":"uname -a"}`},
		{"terminal", `{"command":"uname -a"}`}, // repeat before sieve
		{"read_file", `{"path":"a"}`},
		{"terminal", `{"command":"uname -a"}`}, // repeat after sieve (request 3 shrank)
	}
	dir := writeRun(t, [][]fixtureMsg{grow(0), grow(2), grow(1), grow(2)}, calls)
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.RepeatedCalls != 2 {
		t.Errorf("RepeatedCalls=%d, want 2", row.RepeatedCalls)
	}
	if row.RepeatsAfterSieve != 1 {
		t.Errorf("RepeatsAfterSieve=%d, want 1", row.RepeatsAfterSieve)
	}
}

func TestAnalyze_Turn1LatencyAndMeta(t *testing.T) {
	dir := writeRun(t, [][]fixtureMsg{grow(0)}, []fixtureTool{{"read_file", `{}`}})
	row, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if row.Turn1Latency != 5*time.Second {
		t.Errorf("Turn1Latency=%v, want 5s", row.Turn1Latency)
	}
	if row.Steps != 1 || row.ToolCalls != 1 || row.Duration != 9*time.Second {
		t.Errorf("meta not read: %+v", row)
	}
}

func TestAnalyze_MissingRecordingReportsNotAvailable(t *testing.T) {
	dir := writeRun(t, [][]fixtureMsg{grow(0)}, nil)
	if err := os.Remove(filepath.Join(dir, "recording.jsonl")); err != nil {
		t.Fatal(err)
	}
	row, err := Analyze(dir)
	if err != nil {
		t.Fatalf("a missing recording must degrade, not fail: %v", err)
	}
	if row.HasRecording {
		t.Error("HasRecording must be false")
	}
	if out := FormatRows([]Row{row}); !strings.Contains(out, "n/a") {
		t.Errorf("columns derived from the recording must print n/a, got:\n%s", out)
	}
}

func TestAnalyze_NotARunDirectory(t *testing.T) {
	if _, err := Analyze(t.TempDir()); err == nil {
		t.Error("an empty directory is not a run and must error")
	}
}

func TestFormatRows_HasAllColumns(t *testing.T) {
	dir := writeRun(t, [][]fixtureMsg{grow(0)}, nil)
	row, _ := Analyze(dir)
	out := FormatRows([]Row{row})
	for _, col := range []string{"mem_tok", "mem_req", "sieve", "repeats", "after_sieve", "turn1_s", "steps"} {
		if !strings.Contains(out, col) {
			t.Errorf("missing column %q in:\n%s", col, out)
		}
	}
}

// The sieve markers are matched by substring; they must stay inside the real
// prompts so a reworded note cannot silently zero the sieve column.
func TestSieveMarkersMatchPrompts(t *testing.T) {
	if !strings.Contains(prompts.SieveSystemNote, sieveNoteMarker) {
		t.Errorf("sieveNoteMarker %q not in prompts.SieveSystemNote", sieveNoteMarker)
	}
	if !strings.Contains(prompts.ContextSieveWarning, sieveWarningMarker) {
		t.Errorf("sieveWarningMarker %q not in prompts.ContextSieveWarning", sieveWarningMarker)
	}
}
