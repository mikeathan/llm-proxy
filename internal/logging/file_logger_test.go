package logging_test

import (
	"bytes"
	"llm-proxy/internal/logging"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestFileLogger_Stdout(t *testing.T) {
	out := captureStdout(t, func() {
		logger, err := logging.NewFileLogger(logging.Options{
			Stdout: true,
			Level:  logging.LevelInfo,
		})
		if err != nil {
			t.Fatal(err)
		}

		logger.Info("hello world", "foo", 123)
	})

	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected message in output, got: %s", out)
	}
	if !strings.Contains(out, "foo 123") {
		t.Fatalf("expected args in output, got: %s", out)
	}
}

func TestFileLogger_File(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger, err := logging.NewFileLogger(logging.Options{
		File:  logFile,
		Level: logging.LevelInfo,
	})
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("file test", "key", "value")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}

	out := string(data)
	if !strings.Contains(out, "file test") {
		t.Fatalf("expected message in file, got: %s", out)
	}
}

func TestFileLogger_LevelFiltering(t *testing.T) {
	out := captureStdout(t, func() {
		logger, err := logging.NewFileLogger(logging.Options{
			Stdout: true,
			Level:  logging.LevelWarn,
		})
		if err != nil {
			t.Fatal(err)
		}

		logger.Info("should NOT log")
		logger.Warn("should log")
	})

	if strings.Contains(out, "should NOT log") {
		t.Fatalf("info log should have been filtered")
	}
	if !strings.Contains(out, "should log") {
		t.Fatalf("warn log missing")
	}
}

func TestFileLogger_DefaultStdoutFallback(t *testing.T) {
	out := captureStdout(t, func() {
		logger, err := logging.NewFileLogger(logging.Options{})
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("fallback")
	})

	if !strings.Contains(out, "fallback") {
		t.Fatalf("expected fallback to stdout")
	}
}

func TestFileLogger_Concurrent(t *testing.T) {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelDebug,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger.Debug("msg", i)
		}(i)
	}

	wg.Wait()
}
