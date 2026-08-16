package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Options struct {
	Stdout bool
	Stderr bool // write to os.Stderr (used for the pre-resolution fallback logger)
	File   string // file path, empty = no file
	Level  Level
}

type FileLogger struct {
	mu    sync.Mutex
	level atomic.Value

	out    *log.Logger
	path   string
	ctx    []any
	target *fileTarget // shared mutable file target; nil for stdout/stderr-only loggers

	stop     chan struct{}
	stopOnce sync.Once
}

// fileTarget owns the log file descriptor so Reopen can swap it under a lock.
// With-derived loggers share the same target, so a reopen is observed by every
// logger writing to the same file (e.g. after clear-runtime-data recreates
// logs/ and the old fd points at a deleted inode).
type fileTarget struct {
	mu   sync.Mutex
	file *os.File
}

var _ io.Writer = (*fileTarget)(nil)

func (t *fileTarget) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return 0, io.ErrClosedPipe
	}
	return t.file.Write(p)
}

func (t *fileTarget) Sync() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return nil
	}
	return t.file.Sync()
}

func (t *fileTarget) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return nil
	}
	err := t.file.Close()
	t.file = nil
	return err
}

// Reopen closes the current descriptor and reopens path, so a deleted log file
// (e.g. after clear-runtime-data removes and recreates logs/) is replaced with a
// live fd.
func (t *fileTarget) Reopen(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file != nil {
		_ = t.file.Close()
		t.file = nil
	}
	if path == "" {
		return nil
	}
	if err := ensureLogDir(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	t.file = f
	return nil
}

var _ Logger = (*FileLogger)(nil)

// fileLoggerSyncInterval is how often a FileLogger with a file target fsyncs.
// Per-line fsync would block callers (including the agent loop) on a disk
// syscall for every log line; periodic sync bounds crash loss to one interval.
const fileLoggerSyncInterval = time.Second

func NewFileLogger(opts Options) (*FileLogger, error) {
	var writers []io.Writer
	logPath := ""
	var target *fileTarget

	if opts.Stdout {
		writers = append(writers, os.Stdout)
	}

	if opts.Stderr {
		writers = append(writers, os.Stderr)
	}

	if opts.File != "" {
		path, err := resolveLogPath(opts.File)
		if err != nil {
			return nil, err
		}
		t := &fileTarget{}
		if err := t.Reopen(path); err != nil {
			return nil, err
		}
		writers = append(writers, t)
		logPath = path
		target = t
	}

	if len(writers) == 0 {
		// default safety: stdout
		writers = append(writers, os.Stdout)
	}

	mw := io.MultiWriter(writers...)

	level := opts.Level
	if level == "" {
		level = LevelInfo
	}

	fileLogger := &FileLogger{
		out:    log.New(mw, "", 0),
		path:   logPath,
		ctx:    []any{},
		target: target,
		stop:   make(chan struct{}),
	}
	fileLogger.level.Store(level)
	// Start a periodic fsync goroutine only when writing to a file. Stdout-only
	// loggers (and NopLogger) don't need one. The goroutine exits on Close.
	if target != nil {
		go fileLogger.syncLoop()
	}
	return fileLogger, nil
}

// NewStderrLogger returns a logger that writes only to os.Stderr. It is used as
// the fallback logger for the window before paths are resolved and the file
// logger is created at Paths.LogsDir() (Phase 0/7 boot ordering). It performs no
// fsync and owns no file descriptor to close.
func NewStderrLogger(level Level) *FileLogger {
	if level == "" {
		level = LevelInfo
	}
	l := &FileLogger{
		out:  log.New(os.Stderr, "", 0),
		ctx:  []any{},
		stop: make(chan struct{}),
	}
	l.level.Store(level)
	return l
}

// syncLoop periodically fsyncs the log file so buffered lines survive a crash.
func (l *FileLogger) syncLoop() {
	ticker := time.NewTicker(fileLoggerSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.syncFile()
		case <-l.stop:
			return
		}
	}
}

func (l *FileLogger) syncFile() {
	if l.target != nil {
		_ = l.target.Sync()
	}
}

// Reopen closes and reopens the file target (if any), so writes resume on a
// live descriptor after the log directory was removed and recreated. No-op for
// loggers without a file target.
func (l *FileLogger) Reopen() error {
	if l.target == nil {
		return nil
	}
	return l.target.Reopen(l.path)
}

func (l *FileLogger) Debug(msg string, args ...any) {
	l.log(LevelDebug, msg, args...)
}

func (l *FileLogger) Info(msg string, args ...any) {
	l.log(LevelInfo, msg, args...)
}

func (l *FileLogger) Warn(msg string, args ...any) {
	l.log(LevelWarn, msg, args...)
}

func (l *FileLogger) Error(msg string, args ...any) {
	l.log(LevelError, msg, args...)
}

// With adds contextual key-value pairs to the logging.
func (l *FileLogger) With(args ...any) Logger {
	if len(args) == 0 {
		return l
	}

	// copy existing context to avoid mutation
	ctx := make([]any, 0, len(l.ctx)+len(args))
	ctx = append(ctx, l.ctx...)
	ctx = append(ctx, args...)

	return &FileLogger{
		level:  l.level,
		out:    l.out,
		path:   l.path,
		ctx:    ctx,
		target: l.target,
	}
}

func (l *FileLogger) LogPath() string {
	return l.path
}

func (l *FileLogger) Close() error {
	l.stopOnce.Do(func() {
		close(l.stop)
	})
	l.syncFile()
	if l.target != nil {
		return l.target.Close()
	}
	return nil
}

func (l *FileLogger) log(level Level, msg string, args ...any) {
	if !l.enabled(level) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)

	allArgs := append(l.ctx, args...)
	line := fmt.Sprintf(
		"%s [%s] %s%s",
		ts,
		level,
		msg,
		formatArgs(allArgs...),
	)

	l.out.Println(line)
}

func (l *FileLogger) enabled(level Level) bool {
	current := l.level.Load().(Level)
	order := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return order[level] >= order[current]
}

func (l *FileLogger) SetLevel(level Level) {
	l.level.Store(level)
}

func (l *FileLogger) Level() Level {
	return l.level.Load().(Level)
}

func formatArgs(args ...any) string {
	if len(args) == 0 {
		return ""
	}

	out := " |"
	for i := range args {
		out += fmt.Sprintf(" %v", args[i])
	}
	return out
}

func resolveLogPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return path, ensureLogDir(path)
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(filepath.Dir(exePath), path)
	return absPath, ensureLogDir(absPath)
}

func ensureLogDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}
