package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

func rangeTestFS(t *testing.T, content string) (*FileSystemTools, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{dir}}
	})
	return fs, path
}

func intp(n int) *int { return &n }

var (
	rangeHeaderRe = regexp.MustCompile(`^\[bytes (\d+)-(\d+) of (\d+)\]\n`)
	nextOffsetRe  = regexp.MustCompile(`offset=(\d+)\]$`)
)

// chunkBody strips the header and the trailing note from a ranged read.
func chunkBody(t *testing.T, got string) (body string, next int, more bool) {
	t.Helper()
	m := rangeHeaderRe.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("no range header in %q", got)
	}
	body = got[len(m[0]):]
	if n := nextOffsetRe.FindStringSubmatch(body); n != nil {
		next, _ = strconv.Atoi(n[1])
		body = body[:strings.LastIndex(body, "\n[more:")]
		return body, next, true
	}
	return strings.TrimSuffix(body, "\n[end of file]"), 0, false
}

func TestReadFileRange_ReadsALargeFileEndToEndInChunks(t *testing.T) {
	var b strings.Builder
	for b.Len() < 200_000 {
		b.WriteString("line \"quoted\" <tag> & café 日本語\n")
	}
	original := b.String()
	fs, path := rangeTestFS(t, original)

	var rebuilt strings.Builder
	offset, chunks := 0, 0
	for {
		got, err := fs.ReadFileRange(context.Background(), path, intp(offset), nil)
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		if n := encodedLen(got); n > proxy.MaxReturnChars {
			t.Fatalf("chunk at offset %d encodes to %d, over the %d cap (it would be cut again)", offset, n, proxy.MaxReturnChars)
		}
		body, next, more := chunkBody(t, got)
		if !utf8.ValidString(body) {
			t.Fatalf("chunk at offset %d is not valid UTF-8", offset)
		}
		rebuilt.WriteString(body)
		chunks++
		if !more {
			break
		}
		if next <= offset {
			t.Fatalf("no progress: offset %d next %d", offset, next)
		}
		offset = next
		if chunks > 2000 {
			t.Fatal("too many chunks")
		}
	}
	if rebuilt.String() != original {
		t.Errorf("reassembled file differs (%d vs %d bytes) after %d chunks", rebuilt.Len(), len(original), chunks)
	}
}

func TestReadFileRange_ExactBytesAndLimit(t *testing.T) {
	fs, path := rangeTestFS(t, "0123456789abcdefghij")
	got, err := fs.ReadFileRange(context.Background(), path, intp(5), intp(10))
	if err != nil {
		t.Fatal(err)
	}
	body, next, more := chunkBody(t, got)
	if body != "56789abcde" || !more || next != 15 {
		t.Errorf("body %q next %d more %v", body, next, more)
	}
	if !strings.HasPrefix(got, "[bytes 5-15 of 20]\n") {
		t.Errorf("header: %q", got)
	}
}

func TestReadFileRange_LastChunkSaysEndOfFile(t *testing.T) {
	fs, path := rangeTestFS(t, "0123456789")
	got, _ := fs.ReadFileRange(context.Background(), path, intp(4), intp(100))
	if body, _, more := chunkBody(t, got); body != "456789" || more || !strings.HasSuffix(got, "[end of file]") {
		t.Errorf("got %q", got)
	}
}

func TestReadFileRange_OffsetInsideACharacterMovesToItsStart(t *testing.T) {
	fs, path := rangeTestFS(t, "aé日本")
	got, err := fs.ReadFileRange(context.Background(), path, intp(2), intp(100)) // byte 2 is inside "é"
	if err != nil {
		t.Fatal(err)
	}
	body, _, _ := chunkBody(t, got)
	if !utf8.ValidString(body) || body != "日本" {
		t.Errorf("got %q", body)
	}
	if !strings.HasPrefix(got, "[bytes 3-") {
		t.Errorf("header should report the real start: %q", got)
	}
}

func TestReadFileRange_BadArguments(t *testing.T) {
	fs, path := rangeTestFS(t, "0123456789")
	ctx := context.Background()
	if _, err := fs.ReadFileRange(ctx, path, intp(-1), nil); err == nil {
		t.Error("negative offset must fail")
	}
	if _, err := fs.ReadFileRange(ctx, path, nil, intp(0)); err == nil {
		t.Error("zero limit must fail")
	}
	got, err := fs.ReadFileRange(ctx, path, intp(10), nil)
	if err != nil || !strings.Contains(got, "past the end") || !strings.Contains(got, "10 bytes") {
		t.Errorf("offset at the end: %q, %v", got, err)
	}
	got, err = fs.ReadFileRange(ctx, path, intp(500), nil)
	if err != nil || !strings.Contains(got, "past the end") {
		t.Errorf("offset past the end: %q, %v", got, err)
	}
}

func TestReadFileRange_QuoteHeavyTextStillFitsTheCapWhenEncoded(t *testing.T) {
	fs, path := rangeTestFS(t, strings.Repeat(`"<&>"`, 4000))
	got, err := fs.ReadFileRange(context.Background(), path, intp(0), intp(100_000))
	if err != nil {
		t.Fatal(err)
	}
	if n := encodedLen(got); n > proxy.MaxReturnChars {
		t.Errorf("encodes to %d, over the cap", n)
	}
	if _, next, more := chunkBody(t, got); !more || next == 0 {
		t.Error("a capped chunk must say where to continue")
	}
}

func TestReadFile_LargeFileDefaultKeepsHeadAndTailAndNamesRealTools(t *testing.T) {
	fs, path := rangeTestFS(t, strings.Repeat("abcdefghij", 400))
	got, err := fs.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[NOT SHOWN: bytes ") || !strings.Contains(got, "of the file.") || len(got) > 3200 {
		t.Errorf("default behaviour changed: %.120q (%d)", got, len(got))
	}
	if strings.Contains(got, "read_range") || strings.Contains(got, "grep") {
		t.Errorf("message names a tool that does not exist: %q", got)
	}
	if !strings.Contains(got, "read_file with offset=") {
		t.Errorf("message should say how to read the rest: %q", got)
	}
}

func TestFittingPrefix(t *testing.T) {
	cases := []struct {
		name, text string
		budget     int
	}{
		{"plain", strings.Repeat("a", 500), 100},
		{"escapes", strings.Repeat(`"`, 500), 100},
		{"html", strings.Repeat(`<&>`, 200), 100},
		{"multibyte", strings.Repeat("日", 200), 100},
		{"fits", "short", 100},
	}
	for _, c := range cases {
		n := fittingPrefix(c.text, c.budget)
		if !utf8.ValidString(c.text[:n]) {
			t.Errorf("%s: cut inside a character", c.name)
		}
		if encodedLen(c.text[:n]) > c.budget {
			t.Errorf("%s: prefix of %d bytes encodes to %d, over %d", c.name, n, encodedLen(c.text[:n]), c.budget)
		}
		if n < len(c.text) {
			if next := nextRuneEnd(c.text, n); encodedLen(c.text[:next]) <= c.budget {
				t.Errorf("%s: not the longest prefix (%d would also fit)", c.name, next)
			}
		}
	}
	if fittingPrefix("abc", 1) < 0 {
		t.Error("never negative")
	}
}

func nextRuneEnd(s string, n int) int {
	_, size := utf8.DecodeRuneInString(s[n:])
	return n + size
}

func TestReadFileRange_AHugeFileIsReadByWindowAndAnEmptyFileIsFine(t *testing.T) {
	dir := t.TempDir()
	huge := filepath.Join(dir, "huge.bin")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	const size = 64 << 20
	if err := f.Truncate(size); err != nil { // sparse: costs no disk
		t.Fatal(err)
	}
	f.Close()
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fs := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{dir}}
	})

	got, err := fs.ReadFileRange(context.Background(), huge, intp(size-100), intp(50))
	if err != nil || !strings.HasPrefix(got, "[bytes 67108764-67108814 of 67108864]\n") {
		t.Errorf("huge file: %.80q, %v", got, err)
	}
	got, err = fs.ReadFileRange(context.Background(), empty, intp(0), nil)
	if err != nil || got != "[bytes 0-0 of 0]\n\n[end of file]" {
		t.Errorf("empty file: %q, %v", got, err)
	}
}

func TestValidateFileSystemPath_ExtensionPolicy(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{dir}, AllowedExtensions: []string{".md", ".TXT"}}

	cases := []struct {
		name    string
		path    string
		write   bool
		wantErr string
	}{
		{"allowed extension", "notes.md", false, ""},
		{"extension case is ignored", "notes.txt", true, ""},
		{"disallowed extension", "run.sh", false, "not allowed"},
		{"a directory is not judged by extension", "sub", true, ""},
		{"reading a file with no extension is allowed", "Makefile", false, ""},
		{"writing a file with no extension is not", "Makefile", true, "not allowed"},
	}
	for _, c := range cases {
		_, err := ValidateFileSystemPath(c.path, c.write, cfg)
		if c.wantErr == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: got %v, want an error containing %q", c.name, err, c.wantErr)
		}
	}

	open := models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{dir}}
	if _, err := ValidateFileSystemPath("anything.exe", true, open); err != nil {
		t.Errorf("with no extension list every extension is allowed: %v", err)
	}
	if _, err := ValidateFileSystemPath("../escape.md", false, cfg); err == nil {
		t.Error("path rules still apply before the extension check")
	}
}

var notShownRe = regexp.MustCompile(`\n\[NOT SHOWN: bytes (\d+)-(\d+), (\d+) bytes, (\d+)% of the file\. Call read_file with offset=(\d+) and limit=(\d+) to read it\.\]\n`)

// The model only learns it can read the rest of a long file from the marker in a default read. A marker that sat in the
// middle of an over-cap result was removed by appendToolResult's second cut; one that opens the result and reads as a
// header was ignored by the model, which answered from the two ends. So the marker sits at the gap, says how much is
// missing and gives a real call, and the whole result fits the cap.
func TestReadFile_LongFileMarkerSitsAtTheGapAndSurvivesTheResultCap(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 9000; i++ {
		b.WriteString("row \"" + strconv.Itoa(i) + "\" <x> & text\n")
	}
	raw := b.String()
	fs, path := rangeTestFS(t, raw)

	got, err := fs.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if n := encodedLen(got); n > proxy.MaxReturnChars {
		t.Fatalf("encodes to %d, over the %d cap, so the marker would be cut off", n, proxy.MaxReturnChars)
	}
	encoded, _ := json.Marshal(got)
	if recut := proxy.TruncateResultDefault(string(encoded)); recut != string(encoded) || strings.Contains(recut, "SYSTEM TRUNCATED") {
		t.Error("appendToolResult would cut this result again")
	}
	loc := notShownRe.FindStringSubmatchIndex(got)
	if loc == nil {
		t.Fatalf("no marker at the gap: %.300q", got)
	}
	m := notShownRe.FindStringSubmatch(got)
	from, _ := strconv.Atoi(m[1])
	to, _ := strconv.Atoi(m[2])
	missing, _ := strconv.Atoi(m[3])
	offset, _ := strconv.Atoi(m[5])
	if got[:loc[0]] != raw[:from] || got[loc[1]:] != raw[to:] {
		t.Error("the text before the marker must be the start of the file and the text after it the end")
	}
	if missing != to-from || offset != from || from <= 0 || to >= len(raw) {
		t.Errorf("marker numbers do not describe the gap: %v (file %d bytes)", m[1:], len(raw))
	}
	if strings.ContainsAny(got[loc[0]:loc[1]], "<>&") {
		t.Error("the marker must not contain characters that are escaped when the result is encoded")
	}
	if !utf8.ValidString(got) {
		t.Error("not valid UTF-8")
	}
}

func TestReadFile_LongFileMarkerOffsetContinuesTheFile(t *testing.T) {
	raw := strings.Repeat("abcdefghij", 1000)
	fs, path := rangeTestFS(t, raw)
	got, _ := fs.ReadFile(context.Background(), path)
	m := notShownRe.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("no marker: %.200q", got)
	}
	offset, _ := strconv.Atoi(m[5])
	limit, _ := strconv.Atoi(m[6])
	piece, err := fs.ReadFileRange(context.Background(), path, intp(offset), intp(limit))
	if err != nil {
		t.Fatal(err)
	}
	body, _, _ := chunkBody(t, piece)
	if !strings.HasPrefix(raw[offset:], body) || body == "" {
		t.Errorf("the offered call (offset %d limit %d) does not continue the file", offset, limit)
	}
}

func TestFittingSuffix(t *testing.T) {
	for _, s := range []string{strings.Repeat("a", 500), strings.Repeat(`"`, 500), strings.Repeat("日", 200), "short"} {
		n := fittingSuffix(s, 100)
		if !utf8.ValidString(s[len(s)-n:]) || encodedLen(s[len(s)-n:]) > 100 {
			t.Errorf("suffix of %d bytes of %.10q is invalid or too long", n, s)
		}
		if n < len(s) {
			_, size := utf8.DecodeLastRuneInString(s[:len(s)-n])
			if encodedLen(s[len(s)-n-size:]) <= 100 {
				t.Errorf("%.10q: not the longest suffix", s)
			}
		}
	}
}
