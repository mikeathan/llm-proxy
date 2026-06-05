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
	File   string // file path, empty = no file
	Level  Level
}

type FileLogger struct {
	mu    sync.Mutex
	level atomic.Value

	out    *log.Logger
	path   string
	ctx    []any
	closer io.Closer
}

var _ Logger = (*FileLogger)(nil)

func NewFileLogger(opts Options) (*FileLogger, error) {
	var writers []io.Writer
	logPath := ""
	var closer io.Closer

	if opts.Stdout {
		writers = append(writers, os.Stdout)
	}

	if opts.File != "" {
		path, err := resolveLogPath(opts.File)
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		writers = append(writers, f)
		logPath = path
		closer = f
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
		closer: closer,
	}
	fileLogger.level.Store(level)
	return fileLogger, nil
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
		closer: l.closer,
	}
}

func (l *FileLogger) LogPath() string {
	return l.path
}

func (l *FileLogger) Close() error {
	if l.closer != nil {
		return l.closer.Close()
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
	if f, ok := l.closer.(*os.File); ok && f != nil {
		_ = f.Sync()
	}
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
