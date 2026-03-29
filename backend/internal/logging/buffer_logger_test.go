package logging

import (
	"strings"
	"testing"
)

func TestBufferLogger_WriteKeepsTail(t *testing.T) {
	buf := NewBufferLogger(5)
	_, _ = buf.Write([]byte("hello"))
	_, _ = buf.Write([]byte("world"))

	if got := buf.String(); got != "world" {
		t.Fatalf("expected tail to be \"world\", got %q", got)
	}
}

func TestBufferLogger_WriteLargeChunkKeepsTail(t *testing.T) {
	buf := NewBufferLogger(4)
	_, _ = buf.Write([]byte("abcdef"))

	if got := buf.String(); got != "cdef" {
		t.Fatalf("expected tail to be \"cdef\", got %q", got)
	}
}

func TestBufferLogger_InfoFormatsMessage(t *testing.T) {
	buf := NewBufferLogger(1024)
	buf.Info("hello", "one", 2)

	out := buf.String()
	if !strings.Contains(out, "[INFO] hello") {
		t.Fatalf("expected INFO line, got %q", out)
	}
	if !strings.Contains(out, " | one 2") {
		t.Fatalf("expected args in output, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("expected trailing newline, got %q", out)
	}
}
