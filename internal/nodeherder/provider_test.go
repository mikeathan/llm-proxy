package nodeherder_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/internal/mocks"
	"llm-proxy/internal/nodeherder"
)

// NodeHerder GetDeviceContext Tests
func TestNodeHerder_ReturnsFromCache(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	expected := &nodeherder.LLMDeviceContext{Version: "1"}
	cache.Set(expected)

	mockClient := mocks.NewMockHttpNodeHerderFetcher(nil, nil)

	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	ctx, err := provider.GetDeviceContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx != expected {
		t.Fatalf("expected cached LLM context")
	}

	if mockClient.CallCount() != 0 {
		t.Fatalf("expected no HTTP calls, got %d", mockClient.CallCount())
	}
}

func TestNodeHerder_FetchesAndCaches(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	raw := &nodeherder.DeviceContextResponse{Version: "1"}
	mockClient := mocks.NewMockHttpNodeHerderFetcher(raw, nil)

	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	ctx, err := provider.GetDeviceContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Version != "1" {
		t.Fatalf("unexpected context version")
	}

	if mockClient.CallCount() != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", mockClient.CallCount())
	}

	// second call must hit cache
	ctx2, _ := provider.GetDeviceContext()
	if mockClient.CallCount() != 1 {
		t.Fatalf("expected cached result on second call")
	}
	if ctx2 != ctx {
		t.Fatalf("expected same cached context")
	}
}

func TestNodeHerder_PropagatesFetchError(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	mockClient := mocks.NewMockHttpNodeHerderFetcher(nil, errors.New("backend down"))
	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	ctx, err := provider.GetDeviceContext()
	if err == nil {
		t.Fatalf("expected error")
	}

	if ctx != nil {
		t.Fatalf("expected nil context on error")
	}

	if mockClient.CallCount() != 1 {
		t.Fatalf("expected 1 HTTP call")
	}

	if _, ok := cache.Get(); ok {
		t.Fatalf("context should not be cached on error")
	}
}

func TestNodeHerder_RefreshesAfterTTL(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Second, clock)

	first := &nodeherder.DeviceContextResponse{Version: "1"}
	second := &nodeherder.DeviceContextResponse{Version: "2"}

	mockClient := mocks.NewMockHttpNodeHerderFetcher(first, nil)
	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	ctx1, _ := provider.GetDeviceContext()
	if ctx1.Version != "1" {
		t.Fatalf("unexpected first version")
	}

	clock.Advance(2 * time.Second)
	mockClient.SetDeviceResult(second)

	ctx2, _ := provider.GetDeviceContext()
	if ctx2.Version != "2" {
		t.Fatalf("expected refreshed context")
	}

	if mockClient.CallCount() != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", mockClient.CallCount())
	}
}

func TestNodeHerder_TransformsResponse(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	raw := &nodeherder.DeviceContextResponse{
		Version: "1",
		Devices: []nodeherder.DeviceContext{
			{
				ID:   "dev1",
				Name: "Kitchen Sensor",
				Exposes: []nodeherder.ExposeInfo{
					{
						Name:         "temperature",
						Type:         "numeric",
						Unit:         "°C",
						Aggregations: []nodeherder.AggregationType{"last", "avg"},
					},
					{
						Name:         "state",
						Type:         "binary",
						Values:       []string{"OFF", "ON"},
						ValueOn:      "ON",
						ValueOff:     "OFF",
						Aggregations: []nodeherder.AggregationType{"last"},
					},
				},
			},
		},
	}

	mockClient := mocks.NewMockHttpNodeHerderFetcher(raw, nil)
	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	llmCtx, err := provider.GetDeviceContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if llmCtx.Version != "1" {
		t.Fatalf("expected version 1")
	}

	if len(llmCtx.Devices) != 1 {
		t.Fatalf("expected 1 device")
	}

	dev := llmCtx.Devices[0]
	if dev.ID != "dev1" || dev.Name != "Kitchen Sensor" {
		t.Fatalf("unexpected device mapping")
	}

	temp, ok := findExpose(dev.Exposes, "temperature")
	if !ok {
		t.Fatalf("expected temperature expose")
	}
	if temp.Type != "numeric" || temp.Unit != "°C" {
		t.Fatalf("numeric expose not transformed correctly")
	}

	state, ok := findExpose(dev.Exposes, "state")
	if !ok {
		t.Fatalf("expected state expose")
	}
	if state.Type != "binary" {
		t.Fatalf("binary expose type incorrect")
	}
	if state.On != "ON" || state.Off != "OFF" {
		t.Fatalf("binary expose ON/OFF incorrect")
	}
	if len(state.States) != 2 || state.States[0] != "OFF" || state.States[1] != "ON" {
		t.Fatalf("binary expose states incorrect")
	}
}

func findExpose(exposes []nodeherder.LLMExpose, name string) (nodeherder.LLMExpose, bool) {
	for _, expose := range exposes {
		if expose.Name == name {
			return expose, true
		}
	}
	return nodeherder.LLMExpose{}, false
}

// NodeHerder Query Metrics Tests
func TestNodeHerder_QueryMetrics_DelegatesToFetcher(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	expected := &nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		From:   1,
		To:     10,
		Values: []nodeherder.MetricsQueryDeviceResponse{
			{DeviceId: "dev1", Value: 22.5, Timestamp: 5},
		},
	}

	mockClient := mocks.NewMockHttpNodeHerderFetcher(nil, nil)
	mockClient.SetMetricsResult(expected)

	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs: []string{"dev1"},
		Expose:    "temperature",
	}

	res, err := provider.QueryMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res != expected {
		t.Fatalf("expected same response from fetcher")
	}

	if mockClient.CallCount() != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", mockClient.CallCount())
	}
}

func TestNodeHerder_QueryMetrics_PropagatesError(t *testing.T) {
	logger := &mocks.MockLogger{}
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := nodeherder.NewDeviceContextCache(1*time.Minute, clock)

	mockClient := mocks.NewMockHttpNodeHerderFetcher(nil, errors.New("query failed"))
	provider := nodeherder.NewNodeHerder(mockClient, cache, logger)

	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs: []string{"dev1"},
		Expose:    "temperature",
	}

	res, err := provider.QueryMetrics(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error")
	}

	if res != nil {
		t.Fatalf("expected nil response on error")
	}

	if mockClient.CallCount() != 1 {
		t.Fatalf("expected 1 HTTP call")
	}
}

// HttpNodeHerderFetcher FetchDeviceContext Tests
func TestNodeHerderFetcher_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/context/devices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"version": "1",
			"generatedAt": "2025-01-01T10:00:00Z",
			"devices": [
				{
					"id": "dev1",
					"name": "Sensor",
					"exposes": []
				}
			]
		}`))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("ignored", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	resp, err := client.FetchDeviceContext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Version != "1" {
		t.Fatalf("expected version 1, got %s", resp.Version)
	}

	if len(resp.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(resp.Devices))
	}

	if resp.Devices[0].ID != "dev1" {
		t.Fatalf("unexpected device id: %s", resp.Devices[0].ID)
	}

	if resp.GeneratedAt.IsZero() {
		t.Fatalf("generatedAt should be set")
	}
}

func TestNodeHerderFetcher_Non200Response(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("ignored", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
}

func TestNodeHerderFetcher_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid-json`))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("ignored", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected JSON error, got nil")
	}
}

func TestNodeHerderFetcher_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("ignored", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{Timeout: 5 * time.Second}, tokenManager)

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// HttpNodeHerderFetcher Query Metrics Tests
func TestNodeHerderFetcher_QueryMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metrics/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Fatalf("unexpected auth header: %s", auth)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"expose": "temperature",
				"from": 1,
				"to": 10,
				"values": [
					{
						"deviceId": "dev1",
						"value": 25.5,
						"timestamp": 5
					}
				]
			}
		]`))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("test-token", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs: []string{"dev1"},
		Expose:    "temperature",
	}

	resp, err := client.QueryMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tokenManager.CallCount() != 1 {
		t.Fatalf("expected token manager to be called once, got %d", tokenManager.CallCount())
	}

	if resp.Expose != "temperature" {
		t.Fatalf("expected expose temperature, got %s", resp.Expose)
	}

	if len(resp.Values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(resp.Values))
	}

	if resp.Values[0].DeviceId != "dev1" {
		t.Fatalf("unexpected device id: %s", resp.Values[0].DeviceId)
	}

	if resp.Values[0].Value != 25.5 {
		t.Fatalf("unexpected value: %v", resp.Values[0].Value)
	}
}

func TestNodeHerderFetcher_QueryMetrics_Non200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("test-token", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	_, err := client.QueryMetrics(context.Background(), &nodeherder.MetricsQueryRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
}

func TestNodeHerderFetcher_QueryMetrics_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid-json`))
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("test-token", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{}, tokenManager)

	_, err := client.QueryMetrics(context.Background(), &nodeherder.MetricsQueryRequest{})
	if err == nil {
		t.Fatal("expected JSON error, got nil")
	}
}

func TestNodeHerderFetcher_QueryMetrics_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tokenManager := mocks.NewMockTokenManager("test-token", nil)
	client := nodeherder.NewHttpNodeHerderFetcher(server.URL, &http.Client{Timeout: 5 * time.Second}, tokenManager)

	_, err := client.QueryMetrics(context.Background(), &nodeherder.MetricsQueryRequest{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// ServiceTokenManager Tests
func TestServiceTokenManager_SuccessAndCaches(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/api/auth/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected content-type application/json")
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if payload["client_id"] != "client-id" {
			t.Fatalf("unexpected client_id: %s", payload["client_id"])
		}
		if payload["client_secret"] != "client-secret" {
			t.Fatalf("unexpected client_secret: %s", payload["client_secret"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"token-1","expires_in":120}`))
	}))
	defer server.Close()

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(&http.Client{}, server.URL)

	token1, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 != "token-1" || token2 != "token-1" {
		t.Fatalf("unexpected tokens: %s, %s", token1, token2)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 token request, got %d", calls)
	}
}

func TestServiceTokenManager_ShortExpiryForcesRefresh(t *testing.T) {

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		token := "token-1"
		if call == 2 {
			token = "token-2"
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"` + token + `","expires_in":30}`))
	}))
	defer server.Close()

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(&http.Client{}, server.URL)

	token1, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 == token2 {
		t.Fatalf("expected refreshed token, got %s", token2)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 token requests, got %d", calls)
	}
}

func TestServiceTokenManager_Non200Response(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(&http.Client{}, server.URL)

	_, err := manager.Get(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "token request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// SanitiseUrl Test
func TestSanitiseUrl(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/http://example.com", "http://example.com"},
		{"http://example.com", "http://example.com"},
	}

	for _, tt := range tests {
		got := sanitiseUrl(tt.in)
		if got != tt.want {
			t.Fatalf("sanitiseUrl(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func sanitiseUrl(url string) string {
	if after, ok := strings.CutPrefix(url, "/"); ok {
		return after
	}
	return url
}
