package api_test

import (
	"errors"
	"llm-proxy/internal/api"
	"llm-proxy/internal/mocks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AssistantMessageHandler Tests
func TestAssistantMessageHandler_InvalidContentType(t *testing.T) {
	handler := api.NewAssistantMessageHandler(
		&mocks.MockDeviceContextProvider{},
		nil,
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

	handler := api.NewAssistantMessageHandler(
		&mocks.MockDeviceContextProvider{},
		nil,
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
	provider := mocks.NewMockDeviceContextProvider(nil, errors.New("boom"))

	handler := api.NewAssistantMessageHandler(provider, nil, logger)

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
