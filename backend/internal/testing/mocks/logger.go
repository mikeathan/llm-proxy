package mocks

import "llm-proxy/internal/platform/logging"

type MockLogger struct {
	errors []string
}

func (l *MockLogger) Debug(msg string, args ...any) {}
func (l *MockLogger) Info(msg string, args ...any)  {}
func (l *MockLogger) Warn(msg string, args ...any)  {}
func (l *MockLogger) Error(msg string, args ...any) {
	l.errors = append(l.errors, msg)
}

func (l *MockLogger) With(args ...any) logging.Logger {
	return l
}
func (l *MockLogger) SetLevel(level logging.Level) {}

func (l *MockLogger) Level() logging.Level {
	return logging.LevelDebug
}
func (l *MockLogger) Errors() []string {
	return l.errors
}
