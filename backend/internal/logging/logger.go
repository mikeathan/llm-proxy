package logging

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
