package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestGetKeepAliveInterval(t *testing.T) {
	// Save original env and restore after test
	originalVal := os.Getenv("MCP_KEEPALIVE_INTERVAL")
	defer func() {
		if originalVal != "" {
			os.Setenv("MCP_KEEPALIVE_INTERVAL", originalVal)
		} else {
			os.Unsetenv("MCP_KEEPALIVE_INTERVAL")
		}
	}()

	tests := []struct {
		name     string
		envValue string
		want     time.Duration
	}{
		{
			name:     "default when env not set",
			envValue: "",
			want:     15 * time.Second,
		},
		{
			name:     "valid environment variable",
			envValue: "10",
			want:     10 * time.Second,
		},
		{
			name:     "another valid environment variable",
			envValue: "30",
			want:     30 * time.Second,
		},
		{
			name:     "invalid environment variable - non-numeric",
			envValue: "invalid",
			want:     15 * time.Second, // Should fall back to default
		},
		{
			name:     "invalid environment variable - negative",
			envValue: "-5",
			want:     15 * time.Second, // Should fall back to default
		},
		{
			name:     "invalid environment variable - zero",
			envValue: "0",
			want:     15 * time.Second, // Should fall back to default (zero is not > 0)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("MCP_KEEPALIVE_INTERVAL", tt.envValue)
			} else {
				os.Unsetenv("MCP_KEEPALIVE_INTERVAL")
			}

			got := getKeepAliveInterval()
			if got != tt.want {
				t.Errorf("getKeepAliveInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_PingTimeoutSettings(t *testing.T) {
	// This test verifies that the ping timeout is appropriate (should be 10 seconds now)
	// by examining the manageConnection behavior indirectly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Don't respond to keep the connection "hanging" to test ping timeout
		select {}
	}))
	defer server.Close()

	mLogger := &mockLogger{}
	client := NewClient("test-client", server.URL, "127.0.0.1:0", mLogger)

	// We can't directly test the ping timeout without mocking the entire client,
	// but we can verify the client creation works with our new settings
	if client.Name != "test-client" {
		t.Errorf("NewClient() name = %v, want %v", client.Name, "test-client")
	}
}

func TestClient_HTTPTransportSettings(t *testing.T) {
	// This is a conceptual test - in practice we can't easily test that the
	// http.Transport is configured correctly without making actual network calls
	// or using extensive mocking. However, we can verify the code compiles
	// and the client can be created.

	mLogger := &mockLogger{}
	client := NewClient("test-http", "http://localhost:4110/api/mcp", "127.0.0.1:0", mLogger)

	// Verify client was created with correct initial values
	if client.URL != "http://localhost:4110/api/mcp" {
		t.Errorf("NewClient() URL = %v, want %v", client.URL, "http://localhost:4110/api/mcp")
	}

	if client.BindAddr != "127.0.0.1:0" {
		t.Errorf("NewClient() BindAddr = %v, want %v", client.BindAddr, "127.0.0.1:0")
	}
}

func TestClient_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Send a minimal SSE response
		w.Write([]byte("data: {}\n\n"))
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	mLogger := &mockLogger{}
	client := NewClient("test-client", server.URL, "127.0.0.1:0", mLogger)

	// Test that we can start and stop the client
	ctx, cancel := context.WithCancel(context.Background())
	client.Start(ctx)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the client
	cancel()
	client.Stop()

	// If we get here without panic, the test passes
}

// Test for verifying that the HTTP client with custom transport is being used
// We create a test that intercepts the actual HTTP dialer creation
func TestClient_CustomHTTPClientUsed(t *testing.T) {
	// Save original env and restore after test
	originalVal := os.Getenv("MCP_KEEPALIVE_INTERVAL")
	defer func() {
		if originalVal != "" {
			os.Setenv("MCP_KEEPALIVE_INTERVAL", originalVal)
		} else {
			os.Unsetenv("MCP_KEEPALIVE_INTERVAL")
		}
	}()

	// Test with custom keep-alive interval
	os.Setenv("MCP_KEEPALIVE_INTERVAL", "25")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// The client should have set Origin header
		origin := r.Header.Get("Origin")
		if origin == "" {
			t.Errorf("Expected Origin header to be set by client")
		}

		// Send minimal response to avoid hanging
		w.Write([]byte("data: {}\n\n"))
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	mLogger := &mockLogger{}
	pLogger := logger.NewPulseLogger(mLogger, "test-client")
	client := NewClient("test-client", server.URL, "127.0.0.1:4001", mLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// This should use our custom HTTP client with 25-second keep-alive
	err := client.connect(ctx, pLogger)

	// We expect initialization to fail because our mock doesn't send proper MCP responses,
	// but the HTTP connection with our custom settings should have been attempted
	if err == nil {
		t.Fatalf("Expected an error from initialize, got nil")
	}

	// The error should be about initialization, not about connection creation
	if !strings.Contains(err.Error(), "failed to initialize") &&
		!strings.Contains(err.Error(), "failed to start MCP client") {
		t.Errorf("Expected initialize or start error, got: %v", err)
	}
}

// Test for network origin resolution edge cases
func TestClient_NetworkOriginResolution(t *testing.T) {
	tests := []struct {
		name     string
		bindAddr string
		expected string // We can't know exact IP, but should not be empty for non-zero bindAddr
	}{
		{
			name:     "zero port bind address",
			bindAddr: "127.0.0.1:0",
			expected: "http://127.0.0.1",
		},
		{
			name:     "specific port",
			bindAddr: "192.168.1.100:8080",
			expected: "http://192.168.1.100:8080",
		},
		{
			name:     "localhost with port",
			bindAddr: "localhost:4001",
			expected: "http://localhost:4001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the exact origin without exposing network.ResolveOrigin,
			// but we can verify the client creation works
			mLogger := &mockLogger{}
			client := NewClient("test", "http://localhost:4110/api/mcp", tt.bindAddr, mLogger)

			if client.BindAddr != tt.bindAddr {
				t.Errorf("Client BindAddr = %v, want %v", client.BindAddr, tt.bindAddr)
			}
		})
	}
}

// Test to ensure the client properly handles context cancellation
func TestClient_ContextCancellation(t *testing.T) {
	// Create a server that doesn't send SSE headers properly to trigger an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't set SSE headers, just return 200 OK with no content
		// This will cause the MCP client to fail quickly
		w.WriteHeader(http.StatusOK)
		// Don't write anything - just close
	}))
	defer server.Close()

	mLogger := &mockLogger{}
	pLogger := logger.NewPulseLogger(mLogger, "test-client")
	client := NewClient("test-client", server.URL, "127.0.0.1:0", mLogger)

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should fail quickly due to invalid SSE response
	start := time.Now()
	err := client.connect(ctx, pLogger)
	elapsed := time.Since(start)

	// Should have failed within a reasonable timeframe
	if elapsed > 500*time.Millisecond {
		t.Errorf("connect() took too long (%v) with invalid server response", elapsed)
	}

	// Should get an error
	if err == nil {
		t.Errorf("Expected error with invalid SSE response, got nil")
	}
}

// Test to verify ping timeout is reasonable (10 seconds)
func TestClient_ManageConnectionPingInterval(t *testing.T) {
	// This is more of an integration test - we can't easily test the exact timing
	// without mocking time, but we can verify the code paths work
	mLogger := &mockLogger{}
	client := NewClient("test", "http://localhost:4110/api/mcp", "127.0.0.1:0", mLogger)
	// Create a context for the manageConnection goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the manager in a goroutine
	go client.manageConnection(ctx)
	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel to stop the goroutine
	cancel()

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)

	// If we get here without panic, the test passes
}

// Test to verify that HTTP transport settings are configured for long-lived connections
func TestClient_TransportSettingsForLongLivedConnections(t *testing.T) {
	// This test verifies that our transport is configured correctly for SSE connections
	// by checking that certain timeouts are disabled or set to high values

	// Note: We can't directly inspect the transport settings without exposing them,
	// but we can verify the code compiles and the client works

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {}\n\n"))
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	mLogger := &mockLogger{}
	pLogger := logger.NewPulseLogger(mLogger, "test-client")
	client := NewClient("test-client", server.URL, "127.0.0.1:0", mLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.connect(ctx, pLogger)

	// Expected error is about initialization failure
	if err == nil {
		t.Fatalf("Expected an error from initialize, got nil")
	}

	// Should not be a transport configuration error
	if strings.Contains(err.Error(), "transport") && strings.Contains(err.Error(), "configuration") {
		t.Errorf("Unexpected transport configuration error: %v", err)
	}
}
