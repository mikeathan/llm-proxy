package automation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
)

// teeLogger writes to both a workspace process logger and a run-specific log file.
type teeLogger struct {
	logging.Logger
	fileLogger *logging.FileLogger
}

func newTeeLogger(wsLogger logging.Logger, runLogPath string) (*teeLogger, error) {
	fl, err := logging.NewFileLogger(logging.Options{
		Stdout: false,
		File:   runLogPath,
		Level:  logging.LevelDebug,
	})
	if err != nil {
		return nil, fmt.Errorf("create run log %s: %w", runLogPath, err)
	}
	return &teeLogger{Logger: wsLogger, fileLogger: fl}, nil
}

func (t *teeLogger) Close() error {
	if t.fileLogger != nil {
		return t.fileLogger.Close()
	}
	return nil
}

func (t *teeLogger) Debug(msg string, args ...any) {
	t.Logger.Debug(msg, args...)
	t.fileLogger.Debug(msg, args...)
}

func (t *teeLogger) Info(msg string, args ...any) {
	t.Logger.Info(msg, args...)
	t.fileLogger.Info(msg, args...)
}

func (t *teeLogger) Warn(msg string, args ...any) {
	t.Logger.Warn(msg, args...)
	t.fileLogger.Warn(msg, args...)
}

func (t *teeLogger) Error(msg string, args ...any) {
	t.Logger.Error(msg, args...)
	t.fileLogger.Error(msg, args...)
}

func (t *teeLogger) SetLevel(l logging.Level) {
	t.Logger.SetLevel(l)
	t.fileLogger.SetLevel(l)
}

func (t *teeLogger) Level() logging.Level {
	return t.Logger.Level()
}

// EventSink writes AgentEvents to a JSONL file as they fire during a run.
// Thread-safe, flushes on every write so the file is always current.
type EventSink struct {
	mu      sync.Mutex
	writer  *bufio.Writer
	file    *os.File
	encoder *json.Encoder
}

func NewEventSink(path string) (*EventSink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create events file %s: %w", path, err)
	}
	w := bufio.NewWriterSize(f, 65536)
	return &EventSink{
		file:    f,
		writer:  w,
		encoder: json.NewEncoder(w),
	}, nil
}

func (s *EventSink) Write(ev assistant.AgentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.encoder.Encode(ev); err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if err := s.writer.Flush(); err != nil {
		return err
	}
	if s.file != nil {
		_ = s.file.Sync()
	}
	return nil
}

func (s *EventSink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		s.writer.Flush()
		s.writer = nil
	}
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
}
