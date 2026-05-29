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

type RecordingClient struct {
	underlying   proxy.Client
	recordDir    string
	modelName    string
	mu           sync.Mutex
	file         *os.File
	writer       *bufio.Writer
	encoder      *json.Encoder
	currentRunID string
}

func New(underlying proxy.Client, recordDir string, modelName string) *RecordingClient {
	return &RecordingClient{
		underlying: underlying,
		recordDir:  recordDir,
		modelName:  modelName,
	}
}

func (rc *RecordingClient) ensureFile(ctx context.Context) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	runID := models.GetRunID(ctx)
	if runID != "" && runID != rc.currentRunID {
		rc.closeFile()
		rc.currentRunID = runID
	}

	if rc.file != nil {
		return nil
	}

	if rc.modelName == "" {
		rc.modelName = "unknown"
	}

	sessionID := generateSessionID()
	timestamp := time.Now().UTC().Format("20060102T150405Z")

	taskName := models.GetTaskName(ctx)
	dir := filepath.Join(rc.recordDir, rc.modelName, taskName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("recorder: create dir %s: %w", dir, err)
	}

	filename := fmt.Sprintf("%s_%s.jsonl", timestamp, sessionID)
	filePath := filepath.Join(dir, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("recorder: create file %s: %w", filePath, err)
	}

	rc.file = f
	rc.writer = bufio.NewWriterSize(f, 65536)
	rc.encoder = json.NewEncoder(rc.writer)
	return nil
}

func (rc *RecordingClient) closeFile() {
	if rc.file != nil {
		_ = rc.writer.Flush()
		_ = rc.file.Close()
		rc.file = nil
		rc.writer = nil
		rc.encoder = nil
	}
}

func (rc *RecordingClient) flush() {
	if rc.writer != nil {
		_ = rc.writer.Flush()
	}
	if rc.file != nil {
		_ = rc.file.Sync()
	}
}

func (rc *RecordingClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	if err := rc.ensureFile(ctx); err != nil {
		return nil, err
	}

	rc.writeLine(recordLine{
		Type:     "request",
		Model:    rc.modelName,
		Messages: req.Messages,
		Tools:    req.Tools,
	})

	resp, err := rc.underlying.Chat(ctx, req)
	if err != nil {
		rc.writeLine(recordLine{
			Type:    "error",
			Message: err.Error(),
		})
		return nil, err
	}

	rc.writeLine(recordLine{
		Type:    "response",
		Choices: resp.Choices,
	})
	rc.writeLine(recordLine{
		Type:        "done",
		TotalChunks: 1,
	})

	return resp, nil
}

func (rc *RecordingClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	if err := rc.ensureFile(ctx); err != nil {
		return nil, err
	}

	rc.writeLine(recordLine{
		Type:     "request",
		Model:    rc.modelName,
		Messages: req.Messages,
		Tools:    req.Tools,
	})

	underlyingCh, err := rc.underlying.Stream(ctx, req)
	if err != nil {
		rc.writeLine(recordLine{
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
					rc.writeLine(recordLine{
						Type:        "done",
						TotalChunks: chunks,
					})
					return
				}
				chunks++
				rc.writeLine(recordLine{
					Type:    "chunk",
					Choices: chunk.Choices,
				})
				select {
				case out <- chunk:
				case <-ctx.Done():
					if chunks > 0 {
						rc.writeLine(recordLine{
							Type:        "done",
							TotalChunks: chunks,
						})
					} else {
						rc.writeLine(recordLine{
							Type:    "error",
							Message: "context cancelled",
						})
					}
					return
				}
			case <-ctx.Done():
				if chunks > 0 {
					rc.writeLine(recordLine{
						Type:        "done",
						TotalChunks: chunks,
					})
				} else {
					rc.writeLine(recordLine{
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

func (rc *RecordingClient) writeLine(line recordLine) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.encoder != nil {
		_ = rc.encoder.Encode(line)
		if line.Type == "done" || line.Type == "error" {
			_ = rc.writer.Flush()
			_ = rc.file.Sync()
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
