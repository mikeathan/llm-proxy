package metrics

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var tokensPerSecondRe = regexp.MustCompile(`(?i)(?:^|[\s(])([0-9]+(?:\.[0-9]+)?)\s*(?:tok(?:ens)?/s|t/s|tokens per second)`)

func NewTokenTracker() *TokenTracker {
	return &TokenTracker{}
}

func (t *TokenTracker) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.buf = append(t.buf, p...)
	for {
		idx := bytes.IndexByte(t.buf, '\n')
		if idx == -1 {
			break
		}
		line := string(bytes.TrimSpace(t.buf[:idx]))
		t.buf = t.buf[idx+1:]
		t.parseLine(line)
	}

	const maxBuf = 8192
	if len(t.buf) > maxBuf {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-maxBuf:]...)
	}

	return len(p), nil
}

// promptSpeedMarkers label lines that report prompt-processing speed (the
// per-second progress lines and the "prompt eval time" summary); that is not
// the generation rate the dashboard shows.
var promptSpeedMarkers = []string{"prompt processing", "prompt eval time"}

func (t *TokenTracker) parseLine(line string) {
	for _, marker := range promptSpeedMarkers {
		if strings.Contains(line, marker) {
			return
		}
	}
	m := tokensPerSecondRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil || val <= 0 {
		return
	}
	t.last = val
	t.lastAt = time.Now()
}

func (t *TokenTracker) LastTokensPerSecond() (float64, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last, t.lastAt
}
