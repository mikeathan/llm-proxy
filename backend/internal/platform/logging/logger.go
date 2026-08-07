package logging

import "sync"

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
	SetLevel(Level)
	Level() Level
}

type LogPathProvider interface {
	LogPath() string
}

// Reopenable is implemented by loggers that can replace their file descriptor
// after the log directory was removed and recreated (e.g. clear-runtime-data).
type Reopenable interface {
	Reopen() error
}

var (
	globalLogger Logger
	mu           sync.RWMutex
)

func SetGlobalLogger(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	globalLogger = l
}

func GetGlobalLogger() Logger {
	mu.RLock()
	defer mu.RUnlock()
	return globalLogger
}

// ReopenGlobalLogger reopens the global logger's file target (if any) after the
// log directory was removed and recreated. No-op for loggers without a file
// target.
func ReopenGlobalLogger() error {
	if l := GetGlobalLogger(); l != nil {
		if r, ok := l.(Reopenable); ok {
			return r.Reopen()
		}
	}
	return nil
}

func Debug(msg string, args ...any) {
	if l := GetGlobalLogger(); l != nil {
		l.Debug(msg, args...)
	}
}

func Info(msg string, args ...any) {
	if l := GetGlobalLogger(); l != nil {
		l.Info(msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if l := GetGlobalLogger(); l != nil {
		l.Warn(msg, args...)
	}
}

func Error(msg string, args ...any) {
	if l := GetGlobalLogger(); l != nil {
		l.Error(msg, args...)
	}
}
