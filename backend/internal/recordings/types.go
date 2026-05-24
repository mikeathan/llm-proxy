package recordings

import (
	"encoding/json"
	"time"
)

type RecordingMeta struct {
	ID             string    `json:"id"`
	Model          string    `json:"model"`
	AutomationName string    `json:"automation_name"`
	Timestamp      time.Time `json:"timestamp"`
	FilePath       string    `json:"file_path"`
	FileSize       int64     `json:"file_size"`
	SessionID      string    `json:"session_id"`
}

type RecordedTurn struct {
	Request struct {
		Messages json.RawMessage
		Tools    json.RawMessage
	}
	Response struct {
		Choices json.RawMessage
	}
	Chunks []json.RawMessage
	Error  string
}

type rawLine struct {
	Type        string          `json:"type"`
	Model       string          `json:"model,omitempty"`
	Messages    json.RawMessage `json:"messages,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	Choices     json.RawMessage `json:"choices,omitempty"`
	Message     string          `json:"message,omitempty"`
	TotalChunks int             `json:"total_chunks,omitempty"`
}
