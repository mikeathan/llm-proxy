package assistant

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

func TestAlternatingSpiral_Detected(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
		isAlternating, err := rd.checkAlternating()
		if isAlternating {
			detected = true
			if err == nil {
				t.Error("expected error message with detection")
			}
		}
	}
	if !detected {
		t.Fatal("expected alternating spiral detection with 2 tools over alternatingMinTurns*2 turns")
	}
}

func TestAlternatingSpiral_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	tools := []string{"read_file", "directory_list", "terminal_execute", "network_fetch", "memory_search",
		"write_file", "append_file", "search", "grep", "edit_block", "notify_user"}
	for i := 0; i < alternatingMinTurns; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: tools[i%len(tools)]}},
		})
		rd.checkAlternating()
	}

	if isAlternating, _ := rd.checkAlternating(); isAlternating {
		t.Error("should not detect alternating spiral with many unique tools")
	}
}

func TestAlternatingSpiral_WindowRollsBeyondThreshold(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
		isAlternating, _ := rd.checkAlternating()
		if isAlternating {
			detected = true
		}
	}
	if !detected {
		t.Fatal("expected alternating spiral detection with sustained oscillation")
	}

	if isAlternating, _ := rd.checkAlternating(); isAlternating {
		t.Error("after reset, check should be clean")
	}
}

func TestAlternatingSpiral_StreakResetsOnUniqueTurn(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	for i := 0; i < 10; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
	}

	for i := 0; i < 10; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "unique_tool_" + strings.Repeat("x", i)}},
		})
	}

	if isAlternating, _ := rd.checkAlternating(); isAlternating {
		t.Error("should not detect when unique ratio above threshold")
	}
}

// ---------------------------------------------------------------------------
// Sequence-repeat (n-gram cycle) tests
// ---------------------------------------------------------------------------

func TestSequenceRepeat_CycleDetected(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	// 5-tool cycle: A→B→C→D→E repeated 6 times over 30 calls.
	cycle := []string{"file_read", "grep", "directory_list", "memory_search", "edit_block"}
	for i := 0; i < nGramWindowSize; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: cycle[i%len(cycle)]}},
		})
	}

	isCycle, err := rd.checkSequenceRepeat()
	if !isCycle {
		t.Fatal("expected 5-tool cycle detection")
	}
	if err == nil {
		t.Error("expected error message with detection")
	}
}

func TestSequenceRepeat_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	// 30 calls, each different — no repeating n-gram.
	for i := 0; i < nGramWindowSize; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "tool_" + strings.Repeat("x", i)}},
		})
	}

	if isCycle, _ := rd.checkSequenceRepeat(); isCycle {
		t.Error("all-unique calls should not trigger sequence-repeat detection")
	}
}

func TestSequenceRepeat_ShortWindowIgnores(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	// Under 30 entries, returns early.
	for i := 0; i < nGramWindowSize-1; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "file_read"}},
		})
	}

	if isCycle, _ := rd.checkSequenceRepeat(); isCycle {
		t.Error("should not detect before window fills")
	}
}

func TestSeqKey_LengthDiscriminates(t *testing.T) {
	// Same a,b fields with different lengths must not collide in a map.
	m := make(map[seqKey]bool)

	k3 := seqKey{a: "A", b: "B", length: 3}
	m[k3] = true

	k5 := seqKey{a: "A", b: "B", c: "C", d: "D", e: "E", length: 5}
	if m[k5] {
		t.Error("seqKey with different length must not collide in map")
	}

	k3b := seqKey{a: "A", b: "B", length: 3}
	if !m[k3b] {
		t.Error("seqKey with same fields and length must match")
	}
}

func TestSequenceRepeat_ErrorsIncludeArrowFormat(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	cycle := []string{"file_read", "grep", "directory_list"}
	for i := 0; i < nGramWindowSize; i++ {
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: cycle[i%len(cycle)]}},
		})
	}

	_, err := rd.checkSequenceRepeat()
	if err == nil {
		t.Fatal("expected detection")
	}
	if !strings.Contains(err.Error(), " → ") {
		t.Error("error must contain arrow-separated tool names, got:", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Same-target oscillation tests
// ---------------------------------------------------------------------------

func TestSameTarget_OscillationDetected(t *testing.T) {
	rd := &repetitionDetector{}

	// 6+ different tools hit the same path in one turn.
	calls := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/src/main.go"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"/src/main.go"}`}},
		{Function: proxy.FunctionCall{Name: "edit_block", Arguments: `{"path":"/src/main.go"}`}},
		{Function: proxy.FunctionCall{Name: "directory_list", Arguments: `{"path":"/src"}`}},
		{Function: proxy.FunctionCall{Name: "memory_search", Arguments: `{"file_path":"/src/main.go"}`}},
		{Function: proxy.FunctionCall{Name: "write_file", Arguments: `{"path":"/src/main.go"}`}},
		{Function: proxy.FunctionCall{Name: "append_file", Arguments: `{"path":"/src/main.go"}`}},
	}

	isSameTarget, err := rd.checkSameTarget(calls)
	if !isSameTarget {
		t.Fatal("expected same-target oscillation detection")
	}
	if err == nil {
		t.Error("expected error message with detection")
	}
}

func TestSameTarget_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}

	calls := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.go"}`}},
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/b.go"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"/c.go"}`}},
	}

	if isSameTarget, _ := rd.checkSameTarget(calls); isSameTarget {
		t.Error("different paths should not trigger same-target detection")
	}
}

func TestSameTarget_EmptyCalls(t *testing.T) {
	rd := &repetitionDetector{}

	if isSameTarget, _ := rd.checkSameTarget(nil); isSameTarget {
		t.Error("nil calls should not trigger detection")
	}
	if isSameTarget, _ := rd.checkSameTarget([]proxy.ToolCall{}); isSameTarget {
		t.Error("empty calls should not trigger detection")
	}
}

func TestAlternatingSpiral_DoesNotConflict(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	spiralDetected := false
	for i := 0; i < SpiralStreakThreshold+1 && !spiralDetected; i++ {
		isSpiral, _, err := rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/x.ts"}`}},
		})
		if isSpiral && err != nil {
			spiralDetected = true
		}
	}
	if !spiralDetected {
		t.Fatal("legacy single-tool spiral must still fire when alternating check is also active")
	}

	isAlt, _ := rd.checkAlternating()
	if isAlt {
		t.Error("alternating check must not fire after single-tool spiral resets detectors")
	}
}

func TestAllowedTools_ReadOnlyAutomation(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "directory_list"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "terminal_execute"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "grep"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "write_file"}},
		},
	}
	fp := NewAllowedToolsProvider(inner, []string{"read_file", "directory_list"})
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("read-only automation should expose only 2 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Function.Name] = true
	}
	if names["terminal_execute"] || names["grep"] || names["write_file"] {
		t.Error("terminal_execute, grep, write_file should be blocked for read-only automation")
	}
	if !names["read_file"] || !names["directory_list"] {
		t.Error("read_file and directory_list should be available")
	}

	fpAll := NewAllowedToolsProvider(inner, nil)
	toolsAll, err := fpAll.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools (all): %v", err)
	}
	if len(toolsAll) != 5 {
		t.Errorf("nil AllowedTools should pass all 5 through, got %d", len(toolsAll))
	}
}

func TestDuplicateDetector_Aborts(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	toolCall := proxy.ToolCall{
		Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.ts"}`},
	}
	var duplicateErr error
	for i := 0; i < DuplicateStreakThreshold+3 && duplicateErr == nil; i++ {
		isDuplicate, _, err := rd.check(log, []proxy.ToolCall{toolCall})
		if isDuplicate && err != nil {
			duplicateErr = err
		}
	}
	if duplicateErr == nil {
		t.Fatal("expected duplicate detection to abort after threshold")
	}
}
