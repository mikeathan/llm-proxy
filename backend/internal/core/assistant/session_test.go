package assistant

import (
	"testing"

	"llm-proxy/internal/platform/logging"
)

type testLogger struct {
	T *testing.T
}

func (l *testLogger) Debug(msg string, args ...any) { l.T.Logf("DEBUG: "+msg, args...) }
func (l *testLogger) Info(msg string, args ...any)  { l.T.Logf("INFO: "+msg, args...) }
func (l *testLogger) Warn(msg string, args ...any)  { l.T.Logf("WARN: "+msg, args...) }
func (l *testLogger) Error(msg string, args ...any) { l.T.Logf("ERROR: "+msg, args...) }
func (l *testLogger) With(args ...any) logging.Logger { return l }
func (l *testLogger) SetLevel(logging.Level)        {}
func (l *testLogger) Level() logging.Level          { return logging.LevelDebug }

var _ logging.Logger = (*testLogger)(nil)
