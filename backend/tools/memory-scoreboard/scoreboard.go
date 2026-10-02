package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	// charsPerToken is the real tokenizer ratio the budget code uses
	// (SPEC-005 §II.3), so memory sizes here compare with the budgets.
	charsPerToken = 4

	maxLineBytes = 64 << 20 // recorded requests carry the whole prompt

	memoryOpen  = "<memory>"
	memoryClose = "</memory>"

	// Sieve markers. prompts.SieveSystemNote / ContextSieveWarning contain
	// them (asserted in scoreboard_test.go so they cannot drift).
	sieveNoteMarker    = "History distilled to save context"
	sieveWarningMarker = "HISTORY PRUNED"

	notAvailable = "n/a"

	fileEvents    = "events.jsonl"
	fileRecording = "recording.jsonl"
	fileMeta      = "run-meta.json"

	eventStepStart  = "step_start"
	eventToolCall   = "tool_call"
	lineTypeRequest = "request"
)

// Row is one run's memory scoreboard line.
type Row struct {
	Dir   string
	Model string

	// From recording.jsonl (HasRecording false => n/a).
	HasRecording    bool
	Requests        int
	MemoryRequests  int // requests whose prompt carried a <memory> block
	MemoryTokensAvg int // average memory tokens per request (all requests)
	SieveFirings    int // derived: history shrank, or a sieve note newly appeared

	// From events.jsonl.
	RepeatedCalls     int // tool calls whose (name, args) already ran earlier in the run
	RepeatsAfterSieve int // of those, the ones after the first sieve firing (approximate step mapping)
	HasTurn1          bool
	Turn1Latency      time.Duration // first step_start -> first tool_call
	ToolCalls         int

	// From run-meta.json (falls back to event counts).
	Steps    int
	Duration time.Duration
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (m message) text() string {
	if s, ok := m.Content.(string); ok {
		return s
	}
	return ""
}

type event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

type runMeta struct {
	Model      string `json:"model"`
	DurationMS int64  `json:"duration_ms"`
	LLMCalls   int    `json:"llm_calls"`
	ToolCalls  int    `json:"tool_calls"`
}

// Analyze reads one run directory. Missing files degrade the affected columns
// to n/a; a directory with none of the run files is an error.
func Analyze(dir string) (Row, error) {
	row := Row{Dir: dir}
	meta, metaOK, err := readMeta(dir)
	if err != nil {
		return row, err
	}
	events, eventsOK, err := readEvents(dir)
	if err != nil {
		return row, err
	}
	requests, recOK, err := readRequests(dir)
	if err != nil {
		return row, err
	}
	if !metaOK && !eventsOK && !recOK {
		return row, fmt.Errorf("%s: not a run directory (no %s, %s or %s)", dir, fileMeta, fileEvents, fileRecording)
	}

	row.Model = meta.Model
	row.HasRecording = recOK
	applyRequests(&row, requests)
	firstSieve := -1
	if sieveAt := sieveRequests(requests); len(sieveAt) > 0 {
		firstSieve = sieveAt[0]
	}
	applyEvents(&row, events, firstSieve)
	applyMeta(&row, meta, metaOK, events)
	return row, nil
}

func applyRequests(row *Row, requests [][]message) {
	row.Requests = len(requests)
	chars := 0
	for _, msgs := range requests {
		n := memoryChars(msgs)
		chars += n
		if n > 0 {
			row.MemoryRequests++
		}
	}
	if row.Requests > 0 {
		row.MemoryTokensAvg = chars / row.Requests / charsPerToken
	}
	row.SieveFirings = len(sieveRequests(requests))
}

// memoryChars sums the characters of every <memory>…</memory> block in a request.
func memoryChars(msgs []message) int {
	total := 0
	for _, m := range msgs {
		rest := m.text()
		for {
			start := strings.Index(rest, memoryOpen)
			if start < 0 {
				break
			}
			end := strings.Index(rest[start:], memoryClose)
			if end < 0 {
				break
			}
			total += end + len(memoryClose)
			rest = rest[start+end+len(memoryClose):]
		}
	}
	return total
}

func hasSieveMarker(msgs []message) bool {
	for _, m := range msgs {
		if s := m.text(); strings.Contains(s, sieveNoteMarker) || strings.Contains(s, sieveWarningMarker) {
			return true
		}
	}
	return false
}

func totalChars(msgs []message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.text())
	}
	return n
}

// sieveRequests returns the indexes of requests at which a sieve fired. The
// run files record no sieve event, so it is derived: the prompt shrank versus
// the previous request (fewer messages, or fewer characters from in-place
// compression — a healthy run only grows), or a sieve note appeared that the
// previous request lacked. A prune is not always visible as fewer characters: in
// a real run the sieve dropped the middle of the history in the same turn that
// nine large tool results were added, so the request still grew (9K -> 35K) while
// its message count fell (17 -> 16) and the note appeared. Each request counts at
// most once.
func sieveRequests(requests [][]message) []int {
	var at []int
	for i := 1; i < len(requests); i++ {
		shrank := len(requests[i]) < len(requests[i-1]) || totalChars(requests[i]) < totalChars(requests[i-1])
		noteNew := hasSieveMarker(requests[i]) && !hasSieveMarker(requests[i-1])
		if shrank || noteNew {
			at = append(at, i)
		}
	}
	return at
}

type toolCall struct {
	key       string
	stepsSeen int // step_start events seen before this call (1-based step number)
	ts        time.Time
}

func applyEvents(row *Row, events []event, firstSieveRequest int) {
	var firstStep time.Time
	calls := collectCalls(events, &firstStep)
	seen := map[string]bool{}
	for _, c := range calls {
		if seen[c.key] {
			row.RepeatedCalls++
			if firstSieveRequest >= 0 && c.stepsSeen >= firstSieveRequest+1 {
				row.RepeatsAfterSieve++
			}
		}
		seen[c.key] = true
	}
	row.ToolCalls = len(calls)
	if len(calls) > 0 && !firstStep.IsZero() {
		row.HasTurn1 = true
		row.Turn1Latency = calls[0].ts.Sub(firstStep)
	}
}

func collectCalls(events []event, firstStep *time.Time) []toolCall {
	var calls []toolCall
	steps := 0
	for _, e := range events {
		switch e.Type {
		case eventStepStart:
			steps++
			if steps == 1 {
				*firstStep = e.Timestamp
			}
		case eventToolCall:
			calls = append(calls, toolCall{key: callKey(e.Payload), stepsSeen: steps, ts: e.Timestamp})
		}
	}
	return calls
}

// callKey identifies a call by tool name and raw arguments.
func callKey(payload json.RawMessage) string {
	var p struct {
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return string(payload)
	}
	return p.Function.Name + "\x00" + p.Function.Arguments
}

func applyMeta(row *Row, meta runMeta, metaOK bool, events []event) {
	if metaOK {
		row.Steps = meta.LLMCalls
		row.Duration = time.Duration(meta.DurationMS) * time.Millisecond
		if meta.ToolCalls > 0 {
			row.ToolCalls = meta.ToolCalls
		}
		return
	}
	for _, e := range events {
		if e.Type == eventStepStart {
			row.Steps++
		}
	}
}

func readMeta(dir string) (runMeta, bool, error) {
	var meta runMeta
	data, err := os.ReadFile(filepath.Join(dir, fileMeta))
	if errors.Is(err, fs.ErrNotExist) {
		return meta, false, nil
	}
	if err != nil {
		return meta, false, fmt.Errorf("read %s: %w", fileMeta, err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, false, fmt.Errorf("parse %s: %w", fileMeta, err)
	}
	return meta, true, nil
}

// scanLines calls fn for every line of a JSONL file; ok is false when the file does not exist.
func scanLines(path string, fn func(line []byte)) (ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), maxLineBytes)
	for sc.Scan() {
		fn(sc.Bytes())
	}
	if err := sc.Err(); err != nil {
		return true, fmt.Errorf("scan %s: %w", path, err)
	}
	return true, nil
}

func readEvents(dir string) ([]event, bool, error) {
	var events []event
	ok, err := scanLines(filepath.Join(dir, fileEvents), func(line []byte) {
		var e event
		if json.Unmarshal(line, &e) == nil {
			events = append(events, e)
		}
	})
	return events, ok, err
}

func readRequests(dir string) ([][]message, bool, error) {
	var requests [][]message
	ok, err := scanLines(filepath.Join(dir, fileRecording), func(line []byte) {
		var r struct {
			Type     string    `json:"type"`
			Messages []message `json:"messages"`
		}
		if json.Unmarshal(line, &r) == nil && r.Type == lineTypeRequest {
			requests = append(requests, r.Messages)
		}
	})
	return requests, ok, err
}

// FormatRows renders rows as an aligned table. Columns that cannot be derived
// for a run print n/a rather than a guess.
func FormatRows(rows []Row) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "run\tmodel\tmem_tok\tmem_req\tsieve\trepeats\tafter_sieve\tturn1_s\tsteps")
	for _, r := range rows {
		memTok, memReq, sieve, after := notAvailable, notAvailable, notAvailable, notAvailable
		if r.HasRecording {
			memTok = fmt.Sprint(r.MemoryTokensAvg)
			memReq = fmt.Sprintf("%d/%d", r.MemoryRequests, r.Requests)
			sieve = fmt.Sprint(r.SieveFirings)
			after = fmt.Sprint(r.RepeatsAfterSieve)
		}
		turn1 := notAvailable
		if r.HasTurn1 {
			turn1 = fmt.Sprintf("%.1f", r.Turn1Latency.Seconds())
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%d\n",
			shortDir(r.Dir), r.Model, memTok, memReq, sieve, r.RepeatedCalls, after, turn1, r.Steps)
	}
	w.Flush()
	return b.String()
}

// shortDir keeps the last two path elements (task/run id) so rows stay readable.
func shortDir(dir string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	return strings.Join(parts[max(0, len(parts)-2):], "/")
}
