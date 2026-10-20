package recordings

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// ReadLines returns the lines of a recording file whose "type" is lineType ("" for every line): all of them, or only the
// last `last` when last > 0. A recording line can be megabytes (a request carries the whole conversation), so lines are
// read without a token limit, and the result is bounded by maxBytes: when the matching lines do not fit, the oldest are
// dropped, and a single line over the cap is cut. truncated reports that anything was dropped or cut.
func ReadLines(path, lineType string, last, maxBytes int) (data []byte, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	collect := newLineCollector(lineType, last)
	reader := bufio.NewReaderSize(f, 1<<20)
	for {
		line, readErr := reader.ReadBytes('\n')
		collect.offer(line)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, false, readErr
		}
		if collect.pastCap(maxBytes) {
			break // a whole-file read stops once it is past the cap: the rest could not be returned
		}
	}
	return fit(collect.lines, maxBytes)
}

// lineCollector keeps the lines of the wanted type: all of them, or only the newest `last`.
type lineCollector struct {
	prefix []byte // "" type matches every line
	any    bool
	last   int
	lines  [][]byte
	size   int // total bytes of lines, kept as lines come and go so the cap check is O(1)
}

func newLineCollector(lineType string, last int) *lineCollector {
	return &lineCollector{prefix: []byte(`{"type":"` + lineType + `"`), any: lineType == "", last: last}
}

// offer keeps line if it has the wanted type; a last line with no newline gets one.
func (c *lineCollector) offer(line []byte) {
	if len(line) == 0 || !(c.any || bytes.HasPrefix(line, c.prefix)) {
		return
	}
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	c.lines = append(c.lines, line)
	c.size += len(line)
	if c.last > 0 && len(c.lines) > c.last {
		c.size -= len(c.lines[0])
		c.lines = c.lines[1:]
	}
}

// pastCap is true once a whole-file read holds more than maxBytes (with `last` set the newest lines are kept instead).
func (c *lineCollector) pastCap(maxBytes int) bool {
	return c.last == 0 && c.size > maxBytes
}

func totalLen(lines [][]byte) int {
	n := 0
	for _, l := range lines {
		n += len(l)
	}
	return n
}

// fit joins the lines within maxBytes, dropping the oldest that do not fit and cutting a single line that is over alone.
func fit(lines [][]byte, maxBytes int) ([]byte, bool, error) {
	truncated := false
	for len(lines) > 1 && totalLen(lines) > maxBytes {
		lines = lines[1:]
		truncated = true
	}
	var out bytes.Buffer
	for _, l := range lines {
		out.Write(l)
	}
	if out.Len() > maxBytes {
		return out.Bytes()[:maxBytes], true, nil
	}
	return out.Bytes(), truncated, nil
}

// RedactJSONLines returns JSON-lines data with every string value masked by redact. Recordings written before masking
// existed hold raw secrets, so what is read back is masked as well as what is written. Each line is decoded, its string
// values masked and the line written back (keys come out sorted, numbers keep their text, nothing is HTML-escaped); a
// line that is not one whole JSON value (cut short by the size cap, or not JSON) is masked as plain text instead.
func RedactJSONLines(data []byte, redact func(string) string) []byte {
	if redact == nil || len(data) == 0 {
		return data
	}
	var out bytes.Buffer
	for len(data) > 0 {
		line, rest, hadNewline := data, []byte(nil), false
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, rest, hadNewline = data[:i], data[i+1:], true
		}
		out.Write(redactJSONLine(line, redact))
		if hadNewline {
			out.WriteByte('\n')
		}
		data = rest
	}
	return out.Bytes()
}

func redactJSONLine(line []byte, redact func(string) string) []byte {
	if len(bytes.TrimSpace(line)) == 0 {
		return line
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil || dec.More() {
		return []byte(redact(string(line)))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactStrings(value, redact)); err != nil {
		return []byte(redact(string(line)))
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// redactStrings masks every string value inside a decoded JSON value.
func redactStrings(value any, redact func(string) string) any {
	switch v := value.(type) {
	case string:
		return redact(v)
	case []any:
		for i := range v {
			v[i] = redactStrings(v[i], redact)
		}
	case map[string]any:
		for k := range v {
			v[k] = redactStrings(v[k], redact)
		}
	}
	return value
}
