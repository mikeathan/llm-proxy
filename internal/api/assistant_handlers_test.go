package api_test

import (
	"errors"
	"llm-proxy/internal/api"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestAssistantMessageHandler_RateLimited(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(nil)
	provider.SetDeviceContextResult(&mocks.TestDeviceContext{})

	limiter := &denyLimiter{}

	service := &mocks.MockAssistantService{
		Herder:      provider,
		LoggerRef:   logger,
		Client:      &mocks.MockLLMClientProvider{},
		RateLimiter: limiter,
		Model:       "test-model",
	}

	handler := api.NewAssistantMessageHandler(service)

	body := `{"conversation_id":"conv-123","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	if limiter.Calls != 1 {
		t.Fatalf("expected limiter to be called once, got %d", limiter.Calls)
	}
}

func TestAssistantMessageHandler_DeviceContextError(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(errors.New("boom"))

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
	provider := mocks.NewMockNodeHerder(nil)
	provider.SetDeviceContextResult(&mocks.TestDeviceContext{})
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
	provider := mocks.NewMockNodeHerder(nil)
	provider.SetDeviceContextResult(&mocks.TestDeviceContext{})

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
	provider := mocks.NewMockNodeHerder(nil)
	provider.SetDeviceContextResult(&mocks.TestDeviceContext{})
	provider.SetMetricsResult(&nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		From:   1,
		To:     10,
		Values: []nodeherder.MetricsQueryDeviceResponse{
			{DeviceId: "dev1", Value: 22.5, Timestamp: 5},
		},
	})

	mockClient := &mocks.MockLLMClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					ToolCalls: []proxy.ToolCall{
						{
							ID:   "1",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      "query_metrics",
								Arguments: `{"device_id":"dev1","expose":"temperature","from":1,"to":10,"aggregation":"avg"}`,
							},
						},
					},
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

	if !strings.Contains(rr.Body.String(), `"deviceId":"dev1"`) {
		t.Fatalf("expected metrics response: %s", rr.Body.String())
	}
}

func TestAssistantMessageHandler_EmptyModelResponse(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(nil)
	provider.SetDeviceContextResult(&mocks.TestDeviceContext{})

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

func TestAssistantMessageHandler_HandleToolCall_QueryMetrics(t *testing.T) {
	logger := &mocks.MockLogger{}
	provider := mocks.NewMockNodeHerder(nil)

	provider.SetMetricsResult(&nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		From:   1,
		To:     2,
	})

	mockClient := &mocks.MockLLMClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					ToolCalls: []proxy.ToolCall{
						{
							Function: proxy.FunctionCall{
								Name:      "query_metrics",
								Arguments: `{"device_id":"dev1","expose":"temperature","from":1,"to":2}`,
							},
						},
					},
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

	if !strings.Contains(rr.Body.String(), `"Expose":"temperature"`) {
		t.Fatalf("unexpected response: %s", rr.Body.String())
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

type denyLimiter struct {
	Calls int
}

func (d *denyLimiter) Allow(key string, interval time.Duration) bool {
	d.Calls++
	return false
}

func (d *denyLimiter) Clear() {}
