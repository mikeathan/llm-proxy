package automation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"llm-proxy/internal/core/assistant"
)

// EventSink writes AgentEvents to a JSONL file as they fire during a run.
// Thread-safe. Writes are buffered and flushed on every write so the file is
// current; the file is fsynced periodically (eventSinkSyncInterval) and once
// more on Close, so a crash mid-run loses at most one sync interval of events.
type EventSink struct {
	mu      sync.Mutex
	writer  *bufio.Writer
	file    *os.File
	encoder *json.Encoder
	stop    chan struct{}
	stopOnce sync.Once
}

// eventSinkSyncInterval is how often the events file is fsynced. High-frequency
// events (reasoning/tool_stream chunks) must not fsync per write — that blocks
// the agent loop on a disk syscall per chunk.
const eventSinkSyncInterval = time.Second

func NewEventSink(path string) (*EventSink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create events file %s: %w", path, err)
	}
	w := bufio.NewWriterSize(f, 65536)
	s := &EventSink{
		file:    f,
		writer:  w,
		encoder: json.NewEncoder(w),
		stop:    make(chan struct{}),
	}
	go s.syncLoop()
	return s, nil
}

// syncLoop periodically fsyncs the events file so buffered data survives a
// crash without blocking the hot write path on a per-event fsync.
func (s *EventSink) syncLoop() {
	ticker := time.NewTicker(eventSinkSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sync()
		case <-s.stop:
			return
		}
	}
}

// sync flushes the buffered writer and fsyncs the file. Caller need not hold
// the mutex — sync acquires it.
func (s *EventSink) sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
}

func (s *EventSink) syncLocked() {
	if s.writer != nil {
		_ = s.writer.Flush()
	}
	if s.file != nil {
		_ = s.file.Sync()
	}
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
	return nil
}

func (s *EventSink) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		s.writer.Flush()
		s.writer = nil
	}
	if s.file != nil {
		_ = s.file.Sync()
		s.file.Close()
		s.file = nil
	}
}
