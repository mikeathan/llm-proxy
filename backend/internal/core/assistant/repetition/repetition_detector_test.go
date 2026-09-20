package repetition

import (
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func TestAlternatingSpiral_Detected(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
		isAlternating, err := rd.CheckAlternating()
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
	rd := &Detector{}
	log := logging.NewNopLogger()

	tools := []string{"read_file", "directory_list", "terminal_execute", "network_fetch", "memory_search",
		"write_file", "append_file", "search", "grep", "edit_block", "notify_user"}
	for i := 0; i < alternatingMinTurns; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: tools[i%len(tools)]}},
		})
		rd.CheckAlternating()
	}

	if isAlternating, _ := rd.CheckAlternating(); isAlternating {
		t.Error("should not detect alternating spiral with many unique tools")
	}
}

func TestAlternatingSpiral_WindowRollsBeyondThreshold(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
		isAlternating, _ := rd.CheckAlternating()
		if isAlternating {
			detected = true
		}
	}
	if !detected {
		t.Fatal("expected alternating spiral detection with sustained oscillation")
	}

	if isAlternating, _ := rd.CheckAlternating(); isAlternating {
		t.Error("after reset, check should be clean")
	}
}

func TestAlternatingSpiral_StreakResetsOnUniqueTurn(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	for i := 0; i < 10; i++ {
		name := "file_read"
		if i%2 == 1 {
			name = "grep"
		}
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: name}},
		})
	}

	for i := 0; i < 10; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "unique_tool_" + strings.Repeat("x", i)}},
		})
	}

	if isAlternating, _ := rd.CheckAlternating(); isAlternating {
		t.Error("should not detect when unique ratio above threshold")
	}
}

// TestAlternatingSpiral_SingleToolDominantVariedArgs_NoFalsePositive
// reproduces the workspace-health-test incident: a run dominated by one tool
// (execute_terminal_command) with a distinct command each call, plus occasional
// list_directory interleaves. The tool-NAME unique ratio is tiny (~10%), but
// the calls are not an oscillation — each (tool, args) pair is different.
func TestAlternatingSpiral_SingleToolDominantVariedArgs_NoFalsePositive(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	for i := 0; i < alternatingMinTurns*3; i++ {
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: args}},
		})
		if i%4 == 3 {
			rd.Check(log, []proxy.ToolCall{
				{Function: proxy.FunctionCall{Name: "list_directory", Arguments: fmt.Sprintf(`{"path":"dir-%d"}`, i)}},
			})
		}
		if isAlternating, err := rd.CheckAlternating(); isAlternating {
			t.Fatalf("varied-args single-tool-dominant run must not be flagged as alternating: %v", err)
		}
	}
}

// TestAlternatingSpiral_RecycledTwoToolOscillation_Detected verifies the
// alternating detector still catches a genuine 2-tool oscillation: the same
// (tool, args) pair recurs turn after turn, so the distinct-call ratio stays
// tiny even though the model is not exploring.
func TestAlternatingSpiral_RecycledTwoToolOscillation_Detected(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	calls := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.ts"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"foo"}`}},
	}
	detected := false
	for i := 0; i < alternatingMinTurns*2 && !detected; i++ {
		rd.Check(log, []proxy.ToolCall{calls[i%len(calls)]})
		if isAlternating, err := rd.CheckAlternating(); isAlternating {
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
	rd := &Detector{}
	log := logging.NewNopLogger()

	// 5-tool cycle: A→B→C→D→E repeated 6 times over 30 calls.
	cycle := []string{"file_read", "grep", "directory_list", "memory_search", "edit_block"}
	for i := 0; i < nGramWindowSize; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: cycle[i%len(cycle)]}},
		})
	}

	isCycle, err := rd.CheckSequenceRepeat()
	if !isCycle {
		t.Fatal("expected 5-tool cycle detection")
	}
	if err == nil {
		t.Error("expected error message with detection")
	}
}

func TestSequenceRepeat_NoFalsePositive(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	// 30 calls, each different — no repeating n-gram.
	for i := 0; i < nGramWindowSize; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "tool_" + strings.Repeat("x", i)}},
		})
	}

	if isCycle, _ := rd.CheckSequenceRepeat(); isCycle {
		t.Error("all-unique calls should not trigger sequence-repeat detection")
	}
}

func TestSequenceRepeat_ShortWindowIgnores(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	// Under 30 entries, returns early.
	for i := 0; i < nGramWindowSize-1; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "file_read"}},
		})
	}

	if isCycle, _ := rd.CheckSequenceRepeat(); isCycle {
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
	rd := &Detector{}
	log := logging.NewNopLogger()

	for i := 0; i < nGramWindowSize+5; i++ {
		// Every terminal call is distinct (different command text).
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: args}},
		})
		// Occasional unrelated tool interleaves, as in the real run.
		if i%5 == 3 {
			rd.Check(log, []proxy.ToolCall{
				{Function: proxy.FunctionCall{Name: "list_directory", Arguments: fmt.Sprintf(`{"path":"dir-%d"}`, i)}},
			})
		}
	}

	if isCycle, err := rd.CheckSequenceRepeat(); isCycle {
		t.Fatalf("varied-args single-tool run must not be flagged as a cycle: %v", err)
	}
}

// TestSequenceRepeat_IdenticalToolArgsCycle_Detected verifies the n-gram
// detector still catches a true repeating cycle: the same (tool, args)
// sequence recurring with identical arguments.
func TestSequenceRepeat_IdenticalToolArgsCycle_Detected(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	cycle := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.go"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"foo"}`}},
		{Function: proxy.FunctionCall{Name: "edit_block", Arguments: `{"path":"/a.go"}`}},
	}
	for i := 0; i < nGramWindowSize; i++ {
		rd.Check(log, []proxy.ToolCall{cycle[i%len(cycle)]})
	}

	isCycle, err := rd.CheckSequenceRepeat()
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
	rd := &Detector{}
	log := logging.NewNopLogger()

	cycle := []string{"file_read", "grep", "directory_list"}
	for i := 0; i < nGramWindowSize; i++ {
		rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: cycle[i%len(cycle)]}},
		})
	}

	_, err := rd.CheckSequenceRepeat()
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
	rd := &Detector{}

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

	isSameTarget, err := rd.CheckSameTarget(calls)
	if !isSameTarget {
		t.Fatal("expected same-target oscillation detection")
	}
	if err == nil {
		t.Error("expected error message with detection")
	}
}

func TestSameTarget_NoFalsePositive(t *testing.T) {
	rd := &Detector{}

	calls := []proxy.ToolCall{
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.go"}`}},
		{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/b.go"}`}},
		{Function: proxy.FunctionCall{Name: "grep", Arguments: `{"pattern":"/c.go"}`}},
	}

	if isSameTarget, _ := rd.CheckSameTarget(calls); isSameTarget {
		t.Error("different paths should not trigger same-target detection")
	}
}

func TestSameTarget_EmptyCalls(t *testing.T) {
	rd := &Detector{}

	if isSameTarget, _ := rd.CheckSameTarget(nil); isSameTarget {
		t.Error("nil calls should not trigger detection")
	}
	if isSameTarget, _ := rd.CheckSameTarget([]proxy.ToolCall{}); isSameTarget {
		t.Error("empty calls should not trigger detection")
	}
}

func TestAlternatingSpiral_DoesNotConflict(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	spiralDetected := false
	for i := 0; i < SpiralStreakThreshold+1 && !spiralDetected; i++ {
		isSpiral, _, err := rd.Check(log, []proxy.ToolCall{
			{Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/x.ts"}`}},
		})
		if isSpiral && err != nil {
			spiralDetected = true
		}
	}
	if !spiralDetected {
		t.Fatal("legacy single-tool spiral must still fire when alternating check is also active")
	}

	isAlt, _ := rd.CheckAlternating()
	if isAlt {
		t.Error("alternating check must not fire after single-tool spiral resets detectors")
	}
}

func TestDuplicateDetector_Aborts(t *testing.T) {
	rd := &Detector{}
	log := logging.NewNopLogger()

	toolCall := proxy.ToolCall{
		Function: proxy.FunctionCall{Name: "file_read", Arguments: `{"path":"/a.ts"}`},
	}
	var duplicateErr error
	for i := 0; i < DuplicateStreakThreshold+3 && duplicateErr == nil; i++ {
		isDuplicate, _, err := rd.Check(log, []proxy.ToolCall{toolCall})
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
	rd := &Detector{}
	log := logging.NewNopLogger()

	for i := 0; i < SpiralStreakThreshold+5; i++ {
		args := fmt.Sprintf(`{"command":"find . -maxdepth %d -type f -size +10M 2>/dev/null"}`, i)
		_, _, err := rd.Check(log, []proxy.ToolCall{
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
	rd := &Detector{}
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
		_, _, err := rd.Check(log, []proxy.ToolCall{
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

func TestRepetitionDetector_StreakReset(t *testing.T) {
	logger := logging.NewNopLogger()
	rd := Detector{}

	// Call tool A
	toolCallsA := []proxy.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: proxy.FunctionCall{
				Name:      "toolA",
				Arguments: `{"arg": 1}`,
			},
		},
	}
	isDup, nag, err := rd.Check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected first call to tool A not to be duplicate")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to be 0, got %d", rd.duplicateStreak)
	}

	// Call tool A again -> duplicate
	isDup, nag, err = rd.Check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDup {
		t.Error("expected second consecutive call to tool A to be duplicate")
	}
	if nag != prompts.AutomationDuplicateNagPrompt {
		t.Errorf("expected nag prompt, got %q", nag)
	}
	if rd.duplicateStreak != 1 {
		t.Errorf("expected duplicateStreak to be 1, got %d", rd.duplicateStreak)
	}

	// Call tool B -> resets streak
	toolCallsB := []proxy.ToolCall{
		{
			ID:   "2",
			Type: "function",
			Function: proxy.FunctionCall{
				Name:      "toolB",
				Arguments: `{"arg": 2}`,
			},
		},
	}
	isDup, nag, err = rd.Check(logger, toolCallsB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected call to tool B not to be duplicate")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to reset to 0, got %d", rd.duplicateStreak)
	}

	// Call tool A again -> found but NOT consecutive (B is last), allowed to execute
	isDup, nag, err = rd.Check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected tool A call after tool B to be allowed (non-consecutive)")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to be 0, got %d", rd.duplicateStreak)
	}
	if nag != "" {
		t.Errorf("expected empty nag on non-consecutive duplicate, got %q", nag)
	}

	// Call tool A again -> consecutive duplicate (last key is A), streak = 1
	isDup, _, _ = rd.Check(logger, toolCallsA)
	if !isDup || rd.duplicateStreak != 1 {
		t.Errorf("expected consecutive duplicate, got isDup=%t streak=%d", isDup, rd.duplicateStreak)
	}

	// Call tool A again -> consecutive duplicate, streak = 2
	isDup, _, err = rd.Check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDup || rd.duplicateStreak != 2 {
		t.Errorf("expected duplicate on second consecutive, got isDup=%t streak=%d", isDup, rd.duplicateStreak)
	}

	// Call tool A again -> consecutive duplicate (streak = 3 -> fatal error)
	isDup, nag, err = rd.Check(logger, toolCallsA)
	if err == nil {
		t.Fatal("expected error on third consecutive duplicate, got nil")
	}
	if !strings.Contains(err.Error(), "infinite loop") {
		t.Errorf("expected infinite loop error, got: %v", err)
	}
	if !isDup {
		t.Error("expected third consecutive duplicate to be detected")
	}
	if nag != "" {
		t.Errorf("expected empty nag on fatal duplicate, got %q", nag)
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak reset to 0 after skip, got %d", rd.duplicateStreak)
	}
	if rd.recentCalls != nil {
		t.Errorf("expected recentCalls cleared after skip, got %v", rd.recentCalls)
	}
}

func TestRepetitionDetector_SlidingWindow(t *testing.T) {
	logger := logging.NewNopLogger()

	makeCall := func(name, args string) proxy.ToolCall {
		return proxy.ToolCall{
			ID: fmt.Sprintf("id-%s", name), Type: "function",
			Function: proxy.FunctionCall{Name: name, Arguments: args},
		}
	}

	// Scenario 1: Consecutive duplicate detection
	// The detector only checks if the current call matches the immediately
	// previous call (consecutive duplicate).  Non-consecutive duplicates
	// reset the streak.  A→A→A hits streak=2 on the 3rd call and
	// A→A→A→A hits streak=3 → fatal on the 4th call.
	t.Run("consecutive detection", func(t *testing.T) {
		rd := Detector{}

		// First A: not a duplicate
		isDup, _, err := rd.Check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isDup {
			t.Error("expected first A not duplicate")
		}

		// Second A: consecutive duplicate, streak=1
		isDup, _, err = rd.Check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isDup {
			t.Error("expected second A to be duplicate (consecutive)")
		}
		if rd.duplicateStreak != 1 {
			t.Errorf("expected streak=1 after second A, got %d", rd.duplicateStreak)
		}

		// Third A: consecutive duplicate, streak=2 (still nag, not fatal)
		isDup, _, err = rd.Check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isDup {
			t.Error("expected third A to be duplicate")
		}
		if rd.duplicateStreak != 2 {
			t.Errorf("expected streak=2 after third A, got %d", rd.duplicateStreak)
		}

		// Fourth A: consecutive duplicate, streak=3 → fatal error
		isDup, nag, err := rd.Check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err == nil {
			t.Fatal("expected error on 4th consecutive duplicate, got nil")
		}
		if !strings.Contains(err.Error(), "infinite loop") {
			t.Errorf("expected infinite loop error, got: %v", err)
		}
		if !isDup {
			t.Error("expected 4th consecutive duplicate to be detected")
		}
		if nag != "" {
			t.Errorf("expected empty nag on fatal duplicate, got %q", nag)
		}
		if rd.duplicateStreak != 0 {
			t.Errorf("expected duplicateStreak reset to 0 after skip, got %d", rd.duplicateStreak)
		}
	})

	// Scenario 2: Legitimate iteration — scanning different targets
	t.Run("legitimate iteration", func(t *testing.T) {
		rd := Detector{}
		targets := []string{
			`{"mode":"fast"}`,
			`{"mode":"deep","target":"192.168.50.10"}`,
			`{"mode":"deep","target":"192.168.50.1"}`,
			`{"mode":"deep","target":"192.168.50.60"}`,
			`{"mode":"deep","target":"192.168.50.63"}`,
			`{"mode":"deep","target":"192.168.50.125"}`,
			`{"mode":"deep","target":"192.168.50.241"}`,
		}
		for i, args := range targets {
			_, _, err := rd.Check(logger, []proxy.ToolCall{makeCall("scan_local_network", args)})
			if err != nil {
				t.Fatalf("unexpected error at scan %d (%s): %v", i, args, err)
			}
		}
	})

	// Scenario 3: Consecutive same call is detected as duplicate
	t.Run("consecutive same call", func(t *testing.T) {
		rd := Detector{}

		_, _, err := rd.Check(logger, []proxy.ToolCall{makeCall("execute_terminal_command",
			`{"command":"ts-node quick-check/test.ts","cwd":""}`)})
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}

		// Same command repeated consecutively — detected as duplicate
		isDup, nag, err := rd.Check(logger, []proxy.ToolCall{makeCall("execute_terminal_command",
			`{"command":"ts-node quick-check/test.ts","cwd":""}`)})
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}
		if !isDup {
			t.Error("expected duplicate on consecutive identical call")
		}
		if nag == "" {
			t.Error("expected non-empty nag prompt")
		}
	})

	// Scenario 4: Different targets are NOT duplicates
	t.Run("different args not duplicate", func(t *testing.T) {
		rd := Detector{}

		_, _, err := rd.Check(logger, []proxy.ToolCall{makeCall("scan_local_network",
			`{"mode":"fast"}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Different target — should NOT be duplicate
		isDup, _, err := rd.Check(logger, []proxy.ToolCall{makeCall("scan_local_network",
			`{"mode":"deep","target":"192.168.50.10"}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isDup {
			t.Error("expected different scan targets not to be duplicates")
		}
	})

	// Scenario 5: system_error excluded from tracking (no-op bookkeeping tool)
	t.Run("system_error excluded", func(t *testing.T) {
		rd := Detector{}
		for range 6 {
			_, _, err := rd.Check(logger, []proxy.ToolCall{{
				ID: "sys", Type: "function",
				Function: proxy.FunctionCall{Name: models.ToolSystemError, Arguments: `{}`},
			}})
			if err != nil {
				t.Fatalf("system_error should never trigger loop: %v", err)
			}
		}
	})
}
