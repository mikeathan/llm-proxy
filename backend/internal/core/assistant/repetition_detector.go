package assistant

import (
	"fmt"
	"strings"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

const (
	DuplicateStreakThreshold = 3
	SpiralStreakThreshold    = 12

	// spiralRecycleLimit is the max distinct argument values a consecutive
	// same-tool streak may recycle before it counts as a spiral. A burst of
	// varied-argument calls to one tool (e.g. an audit automation running a
	// sequence of distinct shell commands) is legitimate batching (Constitution
	// II.1), not a spiral — identical-argument repeats are caught earlier by
	// the duplicate detector and the args-aware n-gram detector.
	spiralRecycleLimit = 4

	alternatingWindowSize      = 20
	alternatingMinTurns        = 15
	alternatingUniqueThreshold = 0.3

	nGramWindowSize      = 30
	nGramMinLength       = 3
	nGramMaxLength       = 5
	nGramRepeatThreshold = 3
	sameTargetWindowSize = 20
	sameTargetThreshold  = 6
)

// Error format strings.  Each takes fmt.Errorf arguments matching its name.
const (
	errFmtDuplicateLoop      = "infinite loop detected: %s called %d+ times with identical args"
	errFmtSpiral             = "spiral detected: %s called %d+ consecutive times"
	errFmtAlternatingSpiral  = "alternating tool spiral detected: %.0f%% unique tool calls in last %d calls (threshold %.0f%%)"
	errFmtSequenceRepeat     = "repeating %d-tool cycle detected: %s (%d repeats in last %d calls)"
	errFmtSameTargetOsc      = "same-target oscillation detected: path %q accessed %d times by different tools in one turn"
)

type toolKey struct {
	name string
	args string
}

type repetitionDetector struct {
	recentCalls           []toolKey
	duplicateStreak       int
	lastTool              string
	consecutiveToolStreak int
	streakArgs            map[string]struct{} // distinct args in the current consecutive streak

	alternatingWindow []toolKey // last N tool calls (name+args) for oscillation detection
	alternatingStreak int       // turns with uniqueRatio <= threshold

	seqWindow []toolKey // last N tool calls (name+args) for n-gram cycle detection
}

// ---------------------------------------------------------------------------
// check — duplicate + single-tool spiral detection (unchanged logic)
// ---------------------------------------------------------------------------

func (rd *repetitionDetector) check(logger logging.Logger, toolCalls []proxy.ToolCall) (bool, string, error) {
	for _, tc := range toolCalls {
		key := toolKey{tc.Function.Name, tc.Function.Arguments}
		if tc.Function.Name != models.ToolSystemError {
			if len(rd.recentCalls) > 0 && rd.recentCalls[len(rd.recentCalls)-1] == key {
				rd.duplicateStreak++
				logger.Warn("duplicate action detected", "tool", key.name, "args", key.args, "streak", rd.duplicateStreak)
				if rd.duplicateStreak >= DuplicateStreakThreshold {
					rd.duplicateStreak = 0
					rd.recentCalls = nil
					return true, "", fmt.Errorf(errFmtDuplicateLoop, key.name, DuplicateStreakThreshold)
				}
				return true, prompts.AutomationDuplicateNagPrompt, nil
			}
			rd.duplicateStreak = 0

			if tc.Function.Name == rd.lastTool {
				rd.consecutiveToolStreak++
				if rd.streakArgs == nil {
					rd.streakArgs = make(map[string]struct{})
				}
				rd.streakArgs[key.args] = struct{}{}
				// A spiral is 12+ consecutive calls to the same tool that
				// recycle a small set of argument values (no exploration).
				// Varied-argument bursts are legitimate exploration/batching.
				if rd.consecutiveToolStreak >= SpiralStreakThreshold && len(rd.streakArgs) <= spiralRecycleLimit {
					rd.consecutiveToolStreak = 0
					rd.lastTool = ""
					rd.recentCalls = nil
					rd.streakArgs = nil
					return true, "", fmt.Errorf(errFmtSpiral, key.name, SpiralStreakThreshold)
				}
			} else {
				rd.consecutiveToolStreak = 0
				rd.lastTool = tc.Function.Name
				rd.streakArgs = map[string]struct{}{key.args: {}}
			}

			if len(rd.recentCalls) >= DuplicateStreakThreshold {
				rd.recentCalls = rd.recentCalls[1:]
			}
			rd.recentCalls = append(rd.recentCalls, key)

			// Feed the sliding windows.
			if len(rd.alternatingWindow) >= alternatingWindowSize {
				rd.alternatingWindow = rd.alternatingWindow[1:]
			}
			rd.alternatingWindow = append(rd.alternatingWindow, key)

			if len(rd.seqWindow) >= nGramWindowSize {
				rd.seqWindow = rd.seqWindow[1:]
			}
			rd.seqWindow = append(rd.seqWindow, key)
		}
	}
	return false, "", nil
}

// ---------------------------------------------------------------------------
// checkAlternating — tool-call oscillation detection
// ---------------------------------------------------------------------------

// checkAlternating detects 2-tool oscillation: a window in which the same few
// (tool, args) calls recur turn after turn. Keys are full toolKeys, so a run
// that legitimately dominates on one tool with varying arguments (an audit
// automation issuing a series of distinct shell commands) never trips it —
// each call is a different key. Only genuine recycling (the same calls
// repeating) drives the unique ratio down.
func (rd *repetitionDetector) checkAlternating() (bool, error) {
	if len(rd.alternatingWindow) < alternatingMinTurns {
		return false, nil
	}
	seen := make(map[toolKey]struct{}, len(rd.alternatingWindow))
	for _, k := range rd.alternatingWindow {
		seen[k] = struct{}{}
	}
	uniqueRatio := float64(len(seen)) / float64(len(rd.alternatingWindow))
	if uniqueRatio <= alternatingUniqueThreshold {
		rd.alternatingStreak++
		if rd.alternatingStreak >= alternatingMinTurns {
			rd.alternatingStreak = 0
			rd.alternatingWindow = nil
			return true, fmt.Errorf(errFmtAlternatingSpiral,
				uniqueRatio*100, alternatingWindowSize, alternatingUniqueThreshold*100)
		}
	} else {
		rd.alternatingStreak = 0
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// checkSequenceRepeat — n-gram cycle detection
// ---------------------------------------------------------------------------

// seqKey is a comparable stack-allocated key for n-gram cycle detection.
// Unused fields are zero-valued; comparison only considers fields up to length.
// Keys carry the full toolKey (name + arguments): a "cycle" means the same
// tool sequence recurs with identical arguments. Runs that legitimately use
// one tool with varying arguments (e.g. a series of distinct shell commands)
// therefore never form a false cycle — single-tool stagnation is covered by
// the duplicate-args and 12-consecutive spiral detectors instead.
type seqKey struct {
	a, b, c, d, e toolKey
	length        uint8
}

// checkSequenceRepeat detects tool-call cycles of length 3–5 that repeat
// at least nGramRepeatThreshold times in the seqWindow.  Uses a single
// comparable struct key (no heap allocs) and a single loop over the window.
func (rd *repetitionDetector) checkSequenceRepeat() (bool, error) {
	if len(rd.seqWindow) < nGramWindowSize {
		return false, nil
	}

	counts := make(map[seqKey]int)
	// Outer loop: start index of each n-gram in the sliding window.
	for i := 0; i <= len(rd.seqWindow)-nGramMinLength; i++ {
		// Inner loop: try all n-gram lengths (3, 4, 5) from position i.
		for l := nGramMinLength; l <= nGramMaxLength; l++ {
			idx := i + l
			if idx > len(rd.seqWindow) {
				break // out of bounds for this length at this position
			}
			// Build struct key: first two fields always set; c,d for
			// l≥4; e for l=5.  length field keeps different-length
			// n-grams distinct in the map even when leading fields match.
			k := seqKey{
				a: rd.seqWindow[i], b: rd.seqWindow[i+1],
				length: uint8(l),
			}
			if l >= 4 {
				k.c = rd.seqWindow[i+2]
				k.d = rd.seqWindow[i+3]
			}
			if l >= 5 {
				k.e = rd.seqWindow[i+4]
			}
			counts[k]++
			if counts[k] >= nGramRepeatThreshold {
				// strings.Join only on detection — not in the hot loop.
				arrow := strings.Join(namesOf(rd.seqWindow[i:idx]), " → ")
				return true, fmt.Errorf(errFmtSequenceRepeat,
					l, arrow, counts[k], nGramWindowSize)
			}
		}
	}
	return false, nil
}

// namesOf extracts the tool names from a window slice for cycle-detection
// error messages (the arrows show names; arguments are omitted for brevity).
func namesOf(keys []toolKey) []string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.name
	}
	return names
}

// ---------------------------------------------------------------------------
// checkSameTarget — same-path oscillation detection
// ---------------------------------------------------------------------------

type pathTarget struct {
	tool string
	path string
}

func (rd *repetitionDetector) checkSameTarget(toolCalls []proxy.ToolCall) (bool, error) {
	if len(toolCalls) == 0 {
		return false, nil
	}

	// Collect distinct (tool, path) pairs so repeated calls by the same tool
	// against the same path don't inflate the per-path count.
	seen := make(map[pathTarget]struct{})
	for _, tc := range toolCalls {
		p := extractPathFromArgs(tc.Function.Arguments)
		if p == "" {
			continue
		}
		seen[pathTarget{tool: tc.Function.Name, path: p}] = struct{}{}
	}

	// Track distinct-tool hits per path; an oscillation is a path reached by
	// many different tools within a single turn.
	pathHits := make(map[string]int)
	for pt := range seen {
		pathHits[pt.path]++
		if pathHits[pt.path] >= sameTargetThreshold {
			return true, fmt.Errorf(errFmtSameTargetOsc,
				pt.path, pathHits[pt.path])
		}
	}

	return false, nil
}

// pathSeparators lists characters that imply a value is a filesystem path
// (regardless of its JSON field name).  extractPathFromArgs returns the
// first string-valued field containing any of these separators.
var pathSeparators = []byte{'/'} // add '\\' for Windows if needed

// extractPathFromArgs unmarshals tool-call arguments JSON and scans every
// string-valued field for a path-like value.  No hardcoded field names —
// any field whose value contains a path separator is treated as a path.
func extractPathFromArgs(args string) string {
	if args == "" {
		return ""
	}
	var m map[string]any
	if err := proxy.DecodeToolArgs(args, &m); err != nil {
		return ""
	}
	for _, v := range m {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		for _, sep := range pathSeparators {
			if strings.IndexByte(s, sep) >= 0 {
				return s
			}
		}
	}
	return ""
}
