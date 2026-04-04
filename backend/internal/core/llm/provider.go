package llm

import (
	"context"
	"net/http"
)

type ProviderStatus string

const (
	ProviderStatusRunning ProviderStatus = "running"
	ProviderStatusReady   ProviderStatus = "ready"
	ProviderStatusError   ProviderStatus = "error"
)

type ChatRequest struct {
	Model    string          `json:"model"`
	Messages []ChatMessage   `json:"messages"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Content string `json:"content"`
}

type Provider interface {
	Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	GetStatus() ProviderStatus
	Shutdown() error

	// For administrative and proxy management
	EnsureReady(ctx context.Context) error
	GetEndpoint(ctx context.Context) (string, http.Header, error)
	ListModels(ctx context.Context) ([]string, error)
	TestConnection(ctx context.Context) error
}
