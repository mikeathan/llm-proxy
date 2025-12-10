package proxy

import (
	"bytes"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var tokensPerSecondRe = regexp.MustCompile(`(?i)(?:^|[\s(])([0-9]+(?:\.[0-9]+)?)\s*(?:tok(?:ens)?/s|t/s|tokens per second)`)

// tokenTracker watches llama-server output for tokens/sec lines.
// It keeps the most recent numeric throughput and when it was seen.
type tokenTracker struct {
	mu     sync.Mutex
	buf    []byte
	last   float64
	lastAt time.Time
}

func newTokenTracker() *tokenTracker {
	return &tokenTracker{}
}

func (t *tokenTracker) Write(p []byte) (int, error) {
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

func (t *tokenTracker) parseLine(line string) {
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

func (t *tokenTracker) LastTokensPerSecond() (float64, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last, t.lastAt
}
