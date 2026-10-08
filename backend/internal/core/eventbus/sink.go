package eventbus

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"llm-proxy/internal/core/assistant"
)

// Sink writes AgentEvents to a JSONL file as they fire during a run.
// Thread-safe. Writes are buffered and flushed on every write so the file is
// current; the file is fsynced periodically (syncInterval) and once
// more on Close, so a crash mid-run loses at most one sync interval of events.
type Sink struct {
	mu       sync.Mutex
	writer   *bufio.Writer
	file     *os.File
	encoder  *json.Encoder
	stop     chan struct{}
	stopOnce sync.Once

	// pending is the newest snapshot of the stream being written, held back
	// until the stream ends, another event arrives or the checkpoint is due.
	pending      *assistant.AgentEvent
	checkpointAt time.Time
	now          func() time.Time
}

var errSinkClosed = errors.New("events sink is closed")

// snapshotCheckpointInterval bounds how much of a long reasoning stream a
// crash can lose: the held snapshot is written once this long after the last
// write, so a four-minute stream leaves a couple of dozen lines, not thousands.
const snapshotCheckpointInterval = 10 * time.Second

// syncInterval is how often the events file is fsynced. High-frequency
// events (reasoning/tool_stream chunks) must not fsync per write — that blocks
// the agent loop on a disk syscall per chunk.
const syncInterval = time.Second

func NewSink(path string) (*Sink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create events file %s: %w", path, err)
	}
	w := bufio.NewWriterSize(f, 65536)
	s := &Sink{
		file:    f,
		writer:  w,
		encoder: json.NewEncoder(w),
		stop:    make(chan struct{}),
		now:     time.Now,
	}
	go s.syncLoop()
	return s, nil
}

// syncLoop periodically fsyncs the events file so buffered data survives a
// crash without blocking the hot write path on a per-event fsync.
func (s *Sink) syncLoop() {
	ticker := time.NewTicker(syncInterval)
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
func (s *Sink) sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
}

func (s *Sink) syncLocked() {
	// A held snapshot older than the checkpoint interval is written now, so a
	// stream that stalls (or a crash after it) still leaves its latest text.
	if s.pending != nil && s.writer != nil && s.now().Sub(s.checkpointAt) >= snapshotCheckpointInterval {
		_ = s.flushPendingLocked()
	}
	if s.writer != nil {
		_ = s.writer.Flush()
	}
	if s.file != nil {
		_ = s.file.Sync()
	}
}

// Write records ev. Reasoning and tool_stream events carry the full text so
// far, so only the newest snapshot of a stream is kept (see assistant.SupersedesSnapshot): the
// file stays the run's complete record without growing quadratically.
func (s *Sink) Write(ev assistant.AgentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return errSinkClosed
	}
	if s.pending != nil && assistant.SupersedesSnapshot(*s.pending, ev) {
		s.pending = &ev
		if s.now().Sub(s.checkpointAt) < snapshotCheckpointInterval {
			return nil
		}
		return s.flushPendingLocked()
	}
	if err := s.flushPendingLocked(); err != nil {
		return err
	}
	if ev.Type.IsSnapshot() {
		s.pending = &ev
		s.checkpointAt = s.now()
		return nil
	}
	return s.writeLocked(ev)
}

// flushPendingLocked writes the held snapshot, if any. The caller holds s.mu.
func (s *Sink) flushPendingLocked() error {
	if s.pending == nil {
		return nil
	}
	ev := *s.pending
	s.pending = nil
	s.checkpointAt = s.now()
	return s.writeLocked(ev)
}

func (s *Sink) writeLocked(ev assistant.AgentEvent) error {
	if err := s.encoder.Encode(ev); err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	return s.writer.Flush()
}

func (s *Sink) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		_ = s.flushPendingLocked()
		s.writer.Flush()
		s.writer = nil
	}
	if s.file != nil {
		_ = s.file.Sync()
		s.file.Close()
		s.file = nil
	}
}
