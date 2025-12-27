package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
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
	level Level
	out   *log.Logger
}

var _ Logger = (*FileLogger)(nil)

func NewFileLogger(opts Options) (*FileLogger, error) {
	var writers []io.Writer

	if opts.Stdout {
		writers = append(writers, os.Stdout)
	}

	if opts.File != "" {
		f, err := os.OpenFile(opts.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		writers = append(writers, f)
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

	return &FileLogger{
		level: level,
		out:   log.New(mw, "", 0),
	}, nil
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

func (l *FileLogger) log(level Level, msg string, args ...any) {
	if !l.enabled(level) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)

	line := fmt.Sprintf(
		"%s [%s] %s%s",
		ts,
		level,
		msg,
		formatArgs(args...),
	)

	l.out.Println(line)
}

func (l *FileLogger) enabled(level Level) bool {
	order := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return order[level] >= order[l.level]
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
