package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/logger"
	"llm-proxy/internal/logging"
)

// mockLogger is a simple implementation of logging.Logger for testing
type mockLogger struct {
	logging.Logger
}

func (m *mockLogger) Info(msg string, keysAndValues ...interface{})  {}
func (m *mockLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (m *mockLogger) Error(msg string, keysAndValues ...interface{}) {}
func (m *mockLogger) Debug(msg string, keysAndValues ...interface{}) {}

func TestClient_Connect_HTTPClientSettings(t *testing.T) {
	// Set up a mock SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: connected\n\n"))
		
		// Wait a bit to keep the connection open for the client to parse it
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Convert http:// to http:// so we can use it, SSE client handles the http scheme fine
	sseURL := server.URL

	mLogger := &mockLogger{}
	pLogger := logger.NewPulseLogger(mLogger, "test-client")

	client := NewClient("test-client", sseURL, "127.0.0.1:0", mLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Intercept the connect method
	err := client.connect(ctx, pLogger)
	
	// We might get an error due to no initialize response from our mock server,
	// but the connection should have been established, and we really just want to
	// verify it runs without crashing, since we cannot easily inspect unexported
	// fields of net/http inside the mcp-go client.
	// But we can verify that our URL format and the connection attempt completes.
	
	// Expected error is about initialization failure because our mock doesn't send 
	// a valid JSON-RPC initialization response
	if err == nil {
		t.Fatalf("Expected an error from initialize, got nil")
	}

	if !strings.Contains(err.Error(), "failed to initialize") && !strings.Contains(err.Error(), "failed to start MCP client") {
		t.Errorf("Expected initialize or start error, got: %v", err)
	}
}
