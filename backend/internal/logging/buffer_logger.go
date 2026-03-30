package logging

import (
	"fmt"
	"sync"
	"time"
)

// BufferLogger keeps the last N bytes written to it so we can expose recent output
// without unbounded memory growth.
type BufferLogger struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func NewBufferLogger(limit int) *BufferLogger {
	return &BufferLogger{limit: limit}
}

func (b *BufferLogger) Write(p []byte) (int, error) {
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

func (b *BufferLogger) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *BufferLogger) Debug(msg string, args ...any) {
	b.log(LevelDebug, msg, args...)
}

func (b *BufferLogger) Info(msg string, args ...any) {
	b.log(LevelInfo, msg, args...)
}

func (b *BufferLogger) Warn(msg string, args ...any) {
	b.log(LevelWarn, msg, args...)
}

func (b *BufferLogger) Error(msg string, args ...any) {
	b.log(LevelError, msg, args...)
}

func (b *BufferLogger) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = nil
}

func (b *BufferLogger) log(level Level, msg string, args ...any) {
	ts := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s [%s] %s%s\n", ts, level, msg, formatArgs(args...))
	_, _ = b.Write([]byte(line))
}
