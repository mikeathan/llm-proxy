package proxy

import "sync"

// logBuffer keeps the last N bytes written to it so we can expose recent
// llama-server output without unbounded memory growth.
type logBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func newLogBuffer(limit int) *logBuffer {
	return &logBuffer{limit: limit}
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(p) >= b.limit {
		// Keep only the tail of the incoming chunk if it alone exceeds the limit.
		b.buf = append([]byte(nil), p[len(p)-b.limit:]...)
		return len(p), nil
	}

	if len(b.buf)+len(p) > b.limit {
		trim := len(b.buf) + len(p) - b.limit
		b.buf = append(b.buf[trim:], p...)
	} else {
		b.buf = append(b.buf, p...)
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
