package logging_test

import (
	"bytes"
	"llm-proxy/internal/platform/logging"
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

func TestFileLogger_SetLevel_RuntimeChange(t *testing.T) {
	out := captureStdout(t, func() {
		logger, err := logging.NewFileLogger(logging.Options{
			Stdout: true,
			Level:  logging.LevelInfo,
		})
		if err != nil {
			t.Fatal(err)
		}

		logger.Debug("debug before") // should NOT log

		logger.SetLevel(logging.LevelDebug)

		logger.Debug("debug after") // should log
	})

	if strings.Contains(out, "debug before") {
		t.Fatalf("debug log should not appear before level change")
	}
	if !strings.Contains(out, "debug after") {
		t.Fatalf("debug log should appear after level change")
	}
}

func TestFileLogger_LevelGetter(t *testing.T) {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelWarn,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := logger.Level(); got != logging.LevelWarn {
		t.Fatalf("expected level WARN, got %s", got)
	}

	logger.SetLevel(logging.LevelDebug)

	if got := logger.Level(); got != logging.LevelDebug {
		t.Fatalf("expected level DEBUG after SetLevel, got %s", got)
	}
}

func TestFileLogger_ReopenAfterLogDirRemoved(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "llm-proxy.log")

	logger, err := logging.NewFileLogger(logging.Options{
		File:  logFile,
		Level: logging.LevelInfo,
	})
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("before clear")

	// Simulate clear-runtime-data removing the logs/ directory wholesale.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := logger.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	logger.Info("after clear")
	_ = logger.Close()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("recreated log file unreadable: %v", err)
	}
	if !strings.Contains(string(data), "after clear") {
		t.Fatalf("expected post-reopen line to land in the recreated file, got: %q", string(data))
	}
}

func TestFileLogger_ReopenDerivedLoggerShared(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "llm-proxy.log")

	base, err := logging.NewFileLogger(logging.Options{
		File:  logFile,
		Level: logging.LevelInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	derived := base.With("component", "test")

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := base.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	derived.Info("from derived logger")
	_ = base.Close()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("recreated log file unreadable: %v", err)
	}
	if !strings.Contains(string(data), "from derived logger") {
		t.Fatalf("expected derived-logger write to reach the reopened file, got: %q", string(data))
	}
}

func TestFileLogger_ConcurrentSetLevelAndLogging(t *testing.T) {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	// concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Debug("debug", id, j)
				logger.Info("info", id, j)
			}
		}(i)
	}

	// concurrent level flipper
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			logger.SetLevel(logging.LevelDebug)
			logger.SetLevel(logging.LevelInfo)
			logger.SetLevel(logging.LevelWarn)
		}
	}()

	wg.Wait()
}

func TestFileLogger_RotatesAtMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	logger, err := logging.NewFileLogger(logging.Options{
		File:         path,
		Level:        logging.LevelInfo,
		MaxSizeBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	// A padded line so each write advances the byte counter.
	line := strings.Repeat("x", 300) + "\n"

	// Write enough lines to exceed the 1024-byte cap several times over.
	// log.Logger writes each record straight to the target (no user-space
	// buffering), so the file is readable without an explicit flush.
	for i := 0; i < 12; i++ {
		logger.Info(strings.TrimSuffix(line, "\n"))
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated backup: %v", err)
	}
	if len(current) > 1024 {
		t.Fatalf("current log exceeded max size: %d bytes", len(current))
	}
	if len(backup) > 1024 {
		t.Fatalf("rotated backup exceeded max size: %d bytes", len(backup))
	}
	if len(current) == 0 {
		t.Fatal("current log should contain post-rotation lines")
	}
	if len(backup) == 0 {
		t.Fatal("rotated backup should contain pre-rotation lines")
	}
	// Disk is bounded at ~2×maxSize per path: the .1 backup is replaced on
	// every rotation, so only the latest two chunks survive (lines are ~340
	// bytes with the timestamp prefix, so 12 writes = ~4 rotations).
	if len(current)+len(backup) > 2*1024+len(line) {
		t.Fatalf("expected disk bounded near 2×max size, got %d bytes", len(current)+len(backup))
	}
}

func TestFileLogger_NoRotationWhenUnderMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	logger, err := logging.NewFileLogger(logging.Options{
		File:         path,
		Level:        logging.LevelInfo,
		MaxSizeBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		logger.Info("short line")
	}

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup file when under max size, stat err=%v", err)
	}
}
