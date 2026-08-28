package assistant

import (
	"context"
	"fmt"
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

// TestAlternatingSpiral_SingleToolDominantVariedArgs_NoFalsePositive
// reproduces the workspace-health-test incident: a run dominated by one tool
// (execute_terminal_command) with a distinct command each call, plus occasional
// list_directory interleaves. The tool-NAME unique ratio is tiny (~10%), but
// the calls are not an oscillation — each (tool, args) pair is different.
func TestAlternatingSpiral_SingleToolDominantVariedArgs_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	for i := 0; i < alternatingMinTurns*3; i++ {
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: args}},
		})
		if i%4 == 3 {
			rd.check(log, []proxy.ToolCall{
				{Function: proxy.FunctionCall{Name: "list_directory", Arguments: fmt.Sprintf(`{"path":"dir-%d"}`, i)}},
			})
		}
		if isAlternating, err := rd.checkAlternating(); isAlternating {
			t.Fatalf("varied-args single-tool-dominant run must not be flagged as alternating: %v", err)
		}
	}
}

// TestAlternatingSpiral_RecycledTwoToolOscillation_Detected verifies the
// alternating detector still catches a genuine 2-tool oscillation: the same
// (tool, args) pair recurs turn after turn, so the distinct-call ratio stays
// tiny even though the model is not exploring.
func TestAlternatingSpiral_RecycledTwoToolOscillation_Detected(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	calls := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.ts"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"foo"}`}},
	}
	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		rd.check(log, []proxy.ToolCall{calls[i%len(calls)]})
		if isAlternating, err := rd.checkAlternating(); isAlternating {
			detected = true
			if err == nil {
				t.Error("expected error message with detection")
			}
		}
	}
	if !detected {
		t.Fatal("expected alternating spiral detection with recycled 2-tool oscillation")
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

	k3 := seqKey{a: toolKey{name: "A"}, b: toolKey{name: "B"}, length: 3}
	m[k3] = true

	k5 := seqKey{a: toolKey{name: "A"}, b: toolKey{name: "B"}, c: toolKey{name: "C"}, d: toolKey{name: "D"}, e: toolKey{name: "E"}, length: 5}
	if m[k5] {
		t.Error("seqKey with different length must not collide in map")
	}

	k3b := seqKey{a: toolKey{name: "A"}, b: toolKey{name: "B"}, length: 3}
	if !m[k3b] {
		t.Error("seqKey with same fields and length must match")
	}
}

// TestSequenceRepeat_VariedArgsSameTool_NoFalsePositive reproduces the
// workspace-health-test incident: a run legitimately dominated by one tool
// (execute_terminal_command) whose arguments vary on every call. Name-only
// n-grams flagged it as a repeating cycle; keys must include arguments so
// genuinely different calls never form a cycle.
func TestSequenceRepeat_VariedArgsSameTool_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	for i := 0; i < nGramWindowSize+5; i++ {
		// Every terminal call is distinct (different command text).
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: args}},
		})
		// Occasional unrelated tool interleaves, as in the real run.
		if i%5 == 3 {
			rd.check(log, []proxy.ToolCall{
				{Function: proxy.FunctionCall{Name: "list_directory", Arguments: fmt.Sprintf(`{"path":"dir-%d"}`, i)}},
			})
		}
	}

	if isCycle, err := rd.checkSequenceRepeat(); isCycle {
		t.Fatalf("varied-args single-tool run must not be flagged as a cycle: %v", err)
	}
}

// TestSequenceRepeat_IdenticalToolArgsCycle_Detected verifies the n-gram
// detector still catches a true repeating cycle: the same (tool, args)
// sequence recurring with identical arguments.
func TestSequenceRepeat_IdenticalToolArgsCycle_Detected(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	cycle := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.go"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"foo"}`}},
		{Function: proxy.FunctionCall{Name: "edit_block", Arguments: `{"path":"/a.go"}`}},
	}
	for i := 0; i < nGramWindowSize; i++ {
		rd.check(log, []proxy.ToolCall{cycle[i%len(cycle)]})
	}

	isCycle, err := rd.checkSequenceRepeat()
	if !isCycle {
		t.Fatal("expected identical (tool,args) cycle detection")
	}
	if err == nil {
		t.Error("expected error message with detection")
	}
	if !strings.Contains(err.Error(), " → ") {
		t.Error("error must contain arrow-separated tool names, got:", err.Error())
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
	fp := resolveToolProvider(inner, nil, "", []string{"read_file", "directory_list"}, nil)
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

	fpAll := resolveToolProvider(inner, nil, "", nil, nil)
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

// TestSpiral_VariedArgsBurst_NoFalsePositive reproduces the workspace-health-test
// incident: a run that calls execute_terminal_command 12+ times consecutively
// with a distinct command each time (a storage-audit exploration burst). That is
// legitimate batching (Constitution II.1), not a spiral — identical-argument
// repeats are already caught by the duplicate detector and the args-aware
// n-gram detector.
func TestSpiral_VariedArgsBurst_NoFalsePositive(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	for i := 0; i < SpiralStreakThreshold+5; i++ {
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		_, _, err := rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: args}},
		})
		if err != nil {
			t.Fatalf("varied-args same-tool burst must not be flagged as a spiral: %v", err)
		}
	}
}

// TestSpiral_RecyclingArgs_Detected verifies the spiral detector still aborts
// when 12+ consecutive calls to the same tool recycle a small set of argument
// values (no exploration): the model is stuck re-running the same few commands.
func TestSpiral_RecyclingArgs_Detected(t *testing.T) {
	rd := &repetitionDetector{}
	log := logging.NewNopLogger()

	// Three commands cycling: no consecutive duplicates (the duplicate detector
	// stays quiet), but the streak recycles a tiny arg set.
	cmds := []string{
		`{"command":"df -h"}`,
		`{"command":"du -sh *"}`,
		`{"command":"find . -maxdepth 2 -type f -size +10M"}`,
	}
	var spiralErr error
	for i := 0; i < SpiralStreakThreshold+3 && spiralErr == nil; i++ {
		_, _, err := rd.check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: cmds[i%len(cmds)]}},
		})
		if err != nil {
			spiralErr = err
		}
	}
	if spiralErr == nil {
		t.Fatal("expected recycling spiral detection after threshold")
	}
}
