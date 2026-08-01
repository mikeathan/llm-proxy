package recorder

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type runState struct {
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
}

type RecordingClient struct {
	underlying   proxy.Client
	recordDir    string
	modelName    string
	mu           sync.Mutex
	states       map[string]*runState
	dirs         map[string]string
	currentDir   string // fallback/legacy
	syncRunning  bool
	stop         chan struct{}
	stopOnce     sync.Once
}

// recordingSyncInterval is how often open recording files are fsynced. Chunks
// stream at high frequency; a per-chunk fsync blocks the stream forwarding path
// on a disk syscall each time. Periodic + on-close sync bounds crash loss to
// one interval without the hot-path cost.
const recordingSyncInterval = time.Second

func New(underlying proxy.Client, recordDir string, modelName string) *RecordingClient {
	return &RecordingClient{
		underlying: underlying,
		recordDir:  recordDir,
		modelName:  modelName,
		states:     make(map[string]*runState),
		dirs:       make(map[string]string),
		stop:       make(chan struct{}),
	}
}

// startSyncLoopIfNeeded lazily launches the periodic sync goroutine. It is
// called from ensureFile once a run file exists and self-terminates when no
// states remain, so rebuilding the client (model switch) never leaks a
// goroutine.
func (rc *RecordingClient) startSyncLoopIfNeeded() {
	rc.mu.Lock()
	if rc.syncRunning {
		rc.mu.Unlock()
		return
	}
	rc.syncRunning = true
	rc.mu.Unlock()
	go rc.syncLoop()
}

// syncLoop periodically fsyncs all open recording files and exits once no run
// states remain.
func (rc *RecordingClient) syncLoop() {
	ticker := time.NewTicker(recordingSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rc.mu.Lock()
			empty := len(rc.states) == 0
			if !empty {
				for _, state := range rc.states {
					if state.writer != nil {
						_ = state.writer.Flush()
					}
					if state.file != nil {
						_ = state.file.Sync()
					}
				}
			}
			rc.mu.Unlock()
			if empty {
				rc.mu.Lock()
				rc.syncRunning = false
				rc.mu.Unlock()
				return
			}
		case <-rc.stop:
			return
		}
	}
}

// Stop terminates the periodic sync goroutine. Safe to call multiple times.
func (rc *RecordingClient) Stop() {
	rc.stopOnce.Do(func() {
		close(rc.stop)
	})
}

func (rc *RecordingClient) SetDir(dir string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.currentDir = dir
}

func (rc *RecordingClient) SetDirForRun(runID string, dir string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.dirs[runID] = dir
}

func (rc *RecordingClient) CloseRun(runID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if state, ok := rc.states[runID]; ok {
		_ = state.writer.Flush()
		_ = state.file.Sync()
		_ = state.file.Close()
		delete(rc.states, runID)
	}
	delete(rc.dirs, runID)
}

func (rc *RecordingClient) ensureFile(ctx context.Context) error {
	rc.mu.Lock()
	err := rc.ensureFileLocked(ctx)
	rc.mu.Unlock()
	if err != nil {
		return err
	}
	rc.startSyncLoopIfNeeded()
	return nil
}

// ensureFileLocked creates the recording file for the run, if one does not yet
// exist. The caller must hold rc.mu.
func (rc *RecordingClient) ensureFileLocked(ctx context.Context) error {
	runID := models.GetRunID(ctx)

	state := rc.states[runID]
	if state != nil {
		return nil
	}

	if rc.modelName == "" {
		rc.modelName = "unknown"
	}

	targetDir := rc.dirs[runID]
	if targetDir == "" && runID == "" {
		targetDir = rc.currentDir
	}

	if targetDir == "" && rc.recordDir == "" {
		return nil
	}

	var filePath string
	if targetDir != "" {
		filePath = filepath.Join(targetDir, "recording.jsonl")
	} else if rc.recordDir != "" {
		sessionID := generateSessionID()
		timestamp := time.Now().UTC().Format("20060102T150405Z")
		taskName := models.GetTaskName(ctx)
		dir := filepath.Join(rc.recordDir, rc.modelName, taskName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("recorder: create dir %s: %w", dir, err)
		}
		filename := fmt.Sprintf("%s_%s.jsonl", timestamp, sessionID)
		filePath = filepath.Join(dir, filename)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("recorder: create file %s: %w", filePath, err)
	}

	w := bufio.NewWriterSize(f, 65536)
	rc.states[runID] = &runState{
		file:    f,
		writer:  w,
		encoder: json.NewEncoder(w),
	}
	return nil
}

func (rc *RecordingClient) closeFile() {
	if state, ok := rc.states[""]; ok {
		_ = state.writer.Flush()
		_ = state.file.Sync()
		_ = state.file.Close()
		delete(rc.states, "")
	}
}

func (rc *RecordingClient) flush() {
	if state, ok := rc.states[""]; ok {
		_ = state.writer.Flush()
		_ = state.file.Sync()
	}
}

func (rc *RecordingClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	if err := rc.ensureFile(ctx); err != nil {
		return nil, err
	}

	rc.writeLine(ctx, recordLine{
		Type:     "request",
		Model:    rc.modelName,
		Messages: req.Messages,
		Tools:    req.Tools,
	})

	resp, err := rc.underlying.Chat(ctx, req)
	if err != nil {
		rc.writeLine(ctx, recordLine{
			Type:    "error",
			Message: err.Error(),
		})
		return nil, err
	}

	rc.writeLine(ctx, recordLine{
		Type:    "response",
		Choices: resp.Choices,
	})
	rc.writeLine(ctx, recordLine{
		Type:        "done",
		TotalChunks: 1,
	})

	return resp, nil
}

// ReasoningField delegates to the underlying client so the agent asks the real
// upstream (not the recorder wrapper) which reasoning wire field to use.
func (rc *RecordingClient) ReasoningField() string {
	return rc.underlying.ReasoningField()
}

func (rc *RecordingClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {	if err := rc.ensureFile(ctx); err != nil {
		return nil, err
	}

	rc.writeLine(ctx, recordLine{
		Type:     "request",
		Model:    rc.modelName,
		Messages: req.Messages,
		Tools:    req.Tools,
	})

	underlyingCh, err := rc.underlying.Stream(ctx, req)
	if err != nil {
		rc.writeLine(ctx, recordLine{
			Type:    "error",
			Message: err.Error(),
		})
		return nil, err
	}

	out := make(chan *proxy.ChatResponse, 100)
	go func() {
		defer close(out)
		var chunks int
		for {
			select {
			case chunk, ok := <-underlyingCh:
				if !ok {
					rc.writeLine(ctx, recordLine{
						Type:        "done",
						TotalChunks: chunks,
					})
					return
				}
				chunks++
				rc.writeLine(ctx, recordLine{
					Type:    "chunk",
					Choices: chunk.Choices,
				})
				select {
				case out <- chunk:
				case <-ctx.Done():
					if chunks > 0 {
						rc.writeLine(ctx, recordLine{
							Type:        "done",
							TotalChunks: chunks,
						})
					} else {
						rc.writeLine(ctx, recordLine{
							Type:    "error",
							Message: "context cancelled",
						})
					}
					return
				}
			case <-ctx.Done():
				if chunks > 0 {
					rc.writeLine(ctx, recordLine{
						Type:        "done",
						TotalChunks: chunks,
					})
				} else {
					rc.writeLine(ctx, recordLine{
						Type:    "error",
						Message: "context cancelled",
					})
				}
				return
			}
		}
	}()

	return out, nil
}

func (rc *RecordingClient) writeLine(ctx context.Context, line recordLine) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	runID := models.GetRunID(ctx)
	state := rc.states[runID]
	if state != nil && state.encoder != nil {
		_ = state.encoder.Encode(line)
		if state.writer != nil {
			_ = state.writer.Flush()
		}
	}
}

type recordLine struct {
	Type        string          `json:"type"`
	Model       string          `json:"model,omitempty"`
	Messages    []proxy.Message `json:"messages,omitempty"`
	Tools       []proxy.Tool    `json:"tools,omitempty"`
	Choices     []proxy.Choice  `json:"choices,omitempty"`
	StatusCode  int             `json:"status_code,omitempty"`
	Message     string          `json:"message,omitempty"`
	TotalChunks int             `json:"total_chunks,omitempty"`
}

func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
