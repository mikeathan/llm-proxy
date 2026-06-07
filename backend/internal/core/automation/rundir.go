package automation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunDir owns a single automation run's output directory.
// Created before agent execution, produces:
//   {root}/events.jsonl      — live AgentEvent stream
//   {root}/recording.jsonl   — LLM request/response (when RecordingClient is configured)
//   {root}/final-report.md   — submit_final_answer output
//   {root}/run-meta.json     — summary (duration, model, recordings path, error)
type RunDir struct {
	Root  string // Absolute path to the run folder
	model string
	task  string
}

// RunMeta is written to run-meta.json at the end of the run.
type RunMeta struct {
	Model          string `json:"model"`
	Task           string `json:"task"`
	DurationMs     int64  `json:"duration_ms"`
	StepCount      int    `json:"step_count,omitempty"`
	LLMCalls       int    `json:"llm_calls,omitempty"`
	ToolCalls      int    `json:"tool_calls,omitempty"`
	Error          string `json:"error,omitempty"`
	Result         string `json:"result,omitempty"`
	RecordingPath  string `json:"recording_path,omitempty"`
}

// NewRunDir creates the run directory under parent/{workspaceID}/{task}/{model}/{timestamp}_{sessionID}.
// parent is the base directory (record-dir or data/runs/) — must already exist.
func NewRunDir(parent, workspaceID, task, model string) (*RunDir, error) {
	sessionID := generateSessionID()
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	dirName := fmt.Sprintf("%s_%s", timestamp, sessionID)
	root := filepath.Join(parent, workspaceID, task, model, dirName)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create run dir %s: %w", root, err)
	}
	return &RunDir{Root: root, model: model, task: task}, nil
}

func (r *RunDir) RecordingPath() string   { return filepath.Join(r.Root, "recording.jsonl") }
func (r *RunDir) EventsPath() string      { return filepath.Join(r.Root, "events.jsonl") }
func (r *RunDir) MetaPath() string        { return filepath.Join(r.Root, "run-meta.json") }
func (r *RunDir) ReportPath() string      { return filepath.Join(r.Root, "final-report.md") }
func (r *RunDir) LogPath() string         { return filepath.Join(r.Root, "run.log") }

func (r *RunDir) RecordingRelPath(parent string) string {
	if parent == "" {
		return ""
	}
	rel, _ := filepath.Rel(parent, r.RecordingPath())
	return rel
}

func (r *RunDir) WriteFinalReport(content string) error {
	return os.WriteFile(r.ReportPath(), []byte(content), 0644)
}

func (r *RunDir) WriteMeta(meta RunMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run-meta: %w", err)
	}
	return os.WriteFile(r.MetaPath(), data, 0644)
}

func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
