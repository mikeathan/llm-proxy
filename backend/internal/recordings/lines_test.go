package recordings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecording(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadLines_FiltersByTypeAndKeepsTheLastN(t *testing.T) {
	path := writeRecording(t,
		`{"type":"request","n":1}`, `{"type":"chunk","n":2}`, `{"type":"chunk","n":3}`,
		`{"type":"request","n":4}`, `{"type":"chunk","n":5}`, `{"type":"done"}`)

	all, truncated, err := ReadLines(path, "request", 0, 1<<20)
	if err != nil || truncated {
		t.Fatalf("%v %v", err, truncated)
	}
	if got := string(all); got != `{"type":"request","n":1}`+"\n"+`{"type":"request","n":4}`+"\n" {
		t.Errorf("requests: %q", got)
	}
	last, _, _ := ReadLines(path, "request", 1, 1<<20)
	if string(last) != `{"type":"request","n":4}`+"\n" {
		t.Errorf("last request: %q", last)
	}
	everything, _, _ := ReadLines(path, "", 0, 1<<20)
	if strings.Count(string(everything), "\n") != 6 {
		t.Errorf("no filter returns every line: %q", everything)
	}
	if none, _, _ := ReadLines(path, "error", 0, 1<<20); len(none) != 0 {
		t.Errorf("a type that is not there returns nothing: %q", none)
	}
}

func TestReadLines_ABigLineIsReadWholeAndTheCapTruncatesTheOutput(t *testing.T) {
	big := `{"type":"request","text":"` + strings.Repeat("x", 3<<20) + `"}` // 3 MB: past a bufio.Scanner token
	path := writeRecording(t, `{"type":"request","n":0}`, big, `{"type":"done"}`)

	got, truncated, err := ReadLines(path, "request", 1, 8<<20)
	if err != nil || truncated || len(got) != len(big)+1 {
		t.Fatalf("a 3 MB line must come back whole: err=%v truncated=%v len=%d", err, truncated, len(got))
	}
	capped, truncated, _ := ReadLines(path, "request", 1, 1000)
	if !truncated || len(capped) != 1000 {
		t.Errorf("a line over the cap is cut and flagged: truncated=%v len=%d", truncated, len(capped))
	}
	several, truncated, _ := ReadLines(path, "", 0, 100)
	if !truncated || len(several) > 100 {
		t.Errorf("the cap bounds a whole-file read too: truncated=%v len=%d", truncated, len(several))
	}
}

func TestReadLines_LastNDropsTheOldestWhenTheyDoNotFit(t *testing.T) {
	path := writeRecording(t, `{"type":"request","n":"aaaaaaaaaa"}`, `{"type":"request","n":"bbbbbbbbbb"}`, `{"type":"request","n":"cccccccccc"}`)
	got, truncated, _ := ReadLines(path, "request", 3, 80)
	if strings.Contains(string(got), "aaaaaaaaaa") || !strings.Contains(string(got), "cccccccccc") || !truncated {
		t.Errorf("the newest lines that fit must be kept: %q truncated=%v", got, truncated)
	}
}

func TestReadLines_MissingFileAndNoTrailingNewline(t *testing.T) {
	if _, _, err := ReadLines(filepath.Join(t.TempDir(), "nope.jsonl"), "", 0, 100); err == nil {
		t.Error("a missing file must be an error")
	}
	path := filepath.Join(t.TempDir(), "r.jsonl")
	_ = os.WriteFile(path, []byte(`{"type":"request"}`), 0o600) // a last line with no newline
	got, _, err := ReadLines(path, "request", 0, 100)
	if err != nil || string(got) != `{"type":"request"}`+"\n" {
		t.Errorf("got %q, %v", got, err)
	}
}

func maskSecret(s string) string { return strings.ReplaceAll(s, "SECRET-123", "[REDACTED]") }

// Recordings written before masking existed hold raw secrets; what is read back is masked too.
func TestRedactJSONLines_MasksEveryStringAndKeepsTheStructure(t *testing.T) {
	in := `{"type":"request","model":"m","messages":[{"role":"user","content":"key SECRET-123"},{"role":"assistant","tool_calls":[{"function":{"arguments":"{\"k\":\"SECRET-123\"}"}}]}],"n":12.50,"ok":true,"none":null}` + "\n" +
		`{"type":"done","total_chunks":3}` + "\n"
	got := string(RedactJSONLines([]byte(in), maskSecret))

	if strings.Contains(got, "SECRET-123") {
		t.Fatalf("a secret survived: %s", got)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(got, "\n", 2)[0]), &first); err != nil {
		t.Fatalf("the masked line must still be JSON: %v\n%s", err, got)
	}
	if first["type"] != "request" || first["ok"] != true || first["none"] != nil {
		t.Errorf("unrelated fields changed: %v", first)
	}
	if !strings.Contains(got, `"n":12.50`) {
		t.Errorf("numbers must keep their text: %s", got)
	}
	if !strings.Contains(got, `{"type":"done","total_chunks":3}`) {
		t.Errorf("a line with nothing to mask comes back as it was, key order included: %s", got)
	}
	if strings.Count(got, "\n") != 2 {
		t.Errorf("every line keeps its newline: %q", got)
	}
}

func TestRedactJSONLines_LinesThatAreNotWholeJSONAreMaskedAsText(t *testing.T) {
	cut := `{"type":"request","content":"key SECRET-123 and then it was cut off`
	notJSON := "plain SECRET-123 line"
	got := string(RedactJSONLines([]byte(cut+"\n"+notJSON), maskSecret))
	if strings.Contains(got, "SECRET-123") {
		t.Errorf("text fallback must mask: %q", got)
	}
	if !strings.HasSuffix(got, "[REDACTED] line") || strings.HasSuffix(got, "\n") {
		t.Errorf("a last line with no newline stays without one: %q", got)
	}
}

func TestRedactJSONLines_EdgeCases(t *testing.T) {
	if got := RedactJSONLines(nil, maskSecret); len(got) != 0 {
		t.Errorf("nil: %q", got)
	}
	same := []byte(`{"a":"SECRET-123"}` + "\n")
	if got := RedactJSONLines(same, nil); string(got) != string(same) {
		t.Errorf("no redactor returns the data as it is: %q", got)
	}
	once := RedactJSONLines([]byte(`{"a":"SECRET-123","b":["x","SECRET-123"]}`+"\n\n"), maskSecret)
	if string(RedactJSONLines(once, maskSecret)) != string(once) {
		t.Errorf("masking twice changes nothing: %q", once)
	}
	if !strings.HasSuffix(string(once), "\n\n") {
		t.Errorf("blank lines are kept: %q", once)
	}
	html := RedactJSONLines([]byte(`{"a":"x <tag> & y"}`+"\n"), maskSecret)
	if !strings.Contains(string(html), `<tag> &`) {
		t.Errorf("text is not HTML-escaped when written back: %q", html)
	}
}

// The point of the endpoint is to show what was sent, and a model writes a call in the order a schema lists its arguments
// (the order of tool properties decided whether read_file's offset was used), so masking must not reorder anything.
func TestRedactJSONLines_PreservesKeyOrderAndNumberText(t *testing.T) {
	line := `{"type":"request","model":"m","messages":[{"role":"user","content":"hi SECRET-123"}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"],"type":"object"}}}],"temperature":0.10,"n":1e3,"z":{"b":1,"a":2}}`
	got := string(RedactJSONLines([]byte(line+"\n"), maskSecret))
	want := strings.Replace(line, "SECRET-123", "[REDACTED]", 1) + "\n"
	if got != want {
		t.Errorf("only the secret may change:\n got  %s\n want %s", got, want)
	}
}

func TestRedactJSONLines_KeepsEscapesAndUnicodeReadable(t *testing.T) {
	line := `{"a":"line1\nline2 \"q\" \\ tab\t caf\u00e9 日本","b":"<x> & y"}`
	got := string(RedactJSONLines([]byte(line), maskSecret))
	var v map[string]string
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("still JSON: %v: %s", err, got)
	}
	if v["a"] != "line1\nline2 \"q\" \\ tab\t café 日本" || v["b"] != "<x> & y" {
		t.Errorf("values changed: %q", v)
	}
	if !strings.Contains(got, `<x> & y`) {
		t.Errorf("text is not HTML-escaped when written back: %s", got)
	}
}
