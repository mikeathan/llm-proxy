package logging

import (
	"sync"
	"time"
)

// PulseLogger wraps Logger to provide "Quiet-Pulse" log suppression.
// It tracks failure counts and automatically suppresses logs when the failure threshold is reached.
type PulseLogger struct {
	logger       Logger
	serverName   string
	mu           sync.Mutex
	failureCount int
	threshold    int
	lastSuccess  time.Time
}

// NewPulseLogger ceates a new PulseLogger with a default threshold of 3.
func NewPulseLogger(logger Logger, serverName string) *PulseLogger {
	return &PulseLogger{
		logger:     logger,
		serverName: serverName,
		threshold:  3,
	}
}

func (l *PulseLogger) Info(msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.failureCount <= l.threshold {
		l.logger.Info(msg, args...)
	}
}

func (l *PulseLogger) Warn(msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.incrementFailure()

	if l.failureCount <= l.threshold {
		l.logger.Warn(msg, args...)
	}
}

func (l *PulseLogger) Error(msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.incrementFailure()

	if l.failureCount <= l.threshold {
		l.logger.Error(msg, args...)
	}
}

func (l *PulseLogger) Debug(msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.failureCount <= l.threshold {
		l.logger.Debug(msg, args...)
	}
}

// Success logs key-value pairs at the INFO level and resets the failure count.
// This should be called when an operation succeeds with a message (e.g. initialization).
func (l *PulseLogger) Success(msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.failureCount > l.threshold {
		l.logger.Info("PulseLogger: Recovered. Resuming normal logging.")
	}
	l.failureCount = 0
	l.lastSuccess = time.Now()

	l.logger.Info(msg, args...)
}

// Alive resets the failure count without logging (unless recovering from quiet mode).
// Useful for silent keep-alives like successful pings.
func (l *PulseLogger) Alive() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.failureCount > l.threshold {
		l.logger.Info("PulseLogger: Recovered (silent check). Resuming normal logging.")
	}
	l.failureCount = 0
	l.lastSuccess = time.Now()
}

func (l *PulseLogger) incrementFailure() {
	l.failureCount++

	if l.failureCount == l.threshold+1 {
		l.logger.Info("PulseLogger: Failure threshold reached. Muting logs until recovery.", "server", l.serverName, "threshold", l.threshold)
	}
}
