package logging

type NopLogger struct{}

func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

func (l *NopLogger) Debug(msg string, args ...any) {}
func (l *NopLogger) Info(msg string, args ...any)  {}
func (l *NopLogger) Warn(msg string, args ...any)  {}
func (l *NopLogger) Error(msg string, args ...any) {}
func (l *NopLogger) With(args ...any) Logger       { return l }
func (l *NopLogger) SetLevel(level Level)          {}
func (l *NopLogger) Level() Level                 { return LevelInfo }
