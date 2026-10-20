package recordings

import (
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
