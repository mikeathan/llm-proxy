package api_test

import (
	"errors"
	"llm-proxy/internal/api"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AssistantMessageHandler Tests
func TestAssistantMessageHandler_InvalidContentType(t *testing.T) {
	handler := newTestAssistantHandler(
		&mocks.MockNodeHerder{},
		&mocks.MockLogger{},
	)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "text/plain")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rr.Code)
	}
}

func TestAssistantMessageHandler_InvalidJSON(t *testing.T) {
	logger := &mocks.MockLogger{}

	handler := newTestAssistantHandler(
		&mocks.MockNodeHerder{},
		logger,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/",
		strings.NewReader("{bad json"),
	)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	if len(logger.Errors()) == 0 {
		t.Fatalf("expected error to be logged")
	}
}

func TestAssistantMessageHandler_DeviceContextError(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(nil, errors.New("boom"))

	handler := newTestAssistantHandler(provider, logger)

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}

	if provider.CallCount() != 1 {
		t.Fatalf("expected provider to be called once")
	}

	if len(logger.Errors()) == 0 {
		t.Fatalf("expected error to be logged")
	}
}

func TestAssistantMessageHandler_ModelStarting(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(&mocks.TestDeviceContext{}, nil)

	clientProvider := &mocks.MockLLMClientProvider{
		GetClientErr: llm.ErrModelStarting,
	}

	service := &mocks.MockAssistantService{
		Herder:      provider,
		LoggerRef:   logger,
		Client:      clientProvider,
		RateLimiter: &mocks.MockRateLimiter{},
		Model:       "test-model",
	}

	handler := api.NewAssistantMessageHandler(service)

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestAssistantMessageHandler_SuccessReply(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(&mocks.TestDeviceContext{}, nil)

	mockClient := &mocks.MockLLMClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Content: "hi there"}},
			},
		},
	}

	clientProvider := &mocks.MockLLMClientProvider{Client: mockClient}

	service := &mocks.MockAssistantService{
		Herder:      provider,
		LoggerRef:   logger,
		Client:      clientProvider,
		RateLimiter: &mocks.MockRateLimiter{},
		Model:       "test-model",
	}

	handler := api.NewAssistantMessageHandler(service)

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), `"reply":"hi there"`) {
		t.Fatalf("unexpected response body: %s", rr.Body.String())
	}
}

func TestAssistantMessageHandler_ToolCallPassthrough(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(&mocks.TestDeviceContext{}, nil)

	mockClient := &mocks.MockLLMClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					ToolCalls: []proxy.ToolCall{{ID: "1"}},
				},
			},
		},
	}

	clientProvider := &mocks.MockLLMClientProvider{Client: mockClient}

	service := &mocks.MockAssistantService{
		Herder:      provider,
		LoggerRef:   logger,
		Client:      clientProvider,
		RateLimiter: &mocks.MockRateLimiter{},
		Model:       "test-model",
	}

	handler := api.NewAssistantMessageHandler(service)

	body := `{"message":"run tool"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), `"tool_calls"`) {
		t.Fatalf("expected tool call in response: %s", rr.Body.String())
	}
}

func TestAssistantMessageHandler_EmptyModelResponse(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(&mocks.TestDeviceContext{}, nil)

	mockClient := &mocks.MockLLMClient{
		Response: proxy.ChatResponse{},
	}

	clientProvider := &mocks.MockLLMClientProvider{Client: mockClient}

	service := &mocks.MockAssistantService{
		Herder:      provider,
		LoggerRef:   logger,
		Client:      clientProvider,
		RateLimiter: &mocks.MockRateLimiter{},
		Model:       "test-model",
	}

	handler := api.NewAssistantMessageHandler(service)

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}
}

func newTestAssistantHandler(
	deviceCtx *mocks.MockNodeHerder,
	logger *mocks.MockLogger,
) http.Handler {

	service := &mocks.MockAssistantService{
		Herder:      deviceCtx,
		LoggerRef:   logger,
		Client:      &mocks.MockLLMClientProvider{},
		RateLimiter: &mocks.MockRateLimiter{},
		Model:       "test-model",
	}

	return api.NewAssistantMessageHandler(service)
}
