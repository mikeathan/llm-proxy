package device_context_test

import (
	"errors"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/mocks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// DeviceContextProvider Tests
func TestDeviceContextProvider_ReturnsFromCache(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)

	expected := &device_context.LLMDeviceContext{Version: "1"}
	cache.Set(expected)

	mockClient := mocks.NewMockHttpDeviceContextFetcher(nil, nil)

	provider := device_context.NewDeviceContextProvider(mockClient, cache)

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

func TestDeviceContextProvider_FetchesAndCaches(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)

	raw := &device_context.DeviceContextResponse{Version: "1"}
	mockClient := mocks.NewMockHttpDeviceContextFetcher(raw, nil)

	provider := device_context.NewDeviceContextProvider(mockClient, cache)

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

func TestDeviceContextProvider_PropagatesFetchError(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)

	mockClient := mocks.NewMockHttpDeviceContextFetcher(nil, errors.New("backend down"))
	provider := device_context.NewDeviceContextProvider(mockClient, cache)

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

func TestDeviceContextProvider_RefreshesAfterTTL(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := device_context.NewDeviceContextCache(1*time.Second, clock)

	first := &device_context.DeviceContextResponse{Version: "1"}
	second := &device_context.DeviceContextResponse{Version: "2"}

	mockClient := mocks.NewMockHttpDeviceContextFetcher(first, nil)
	provider := device_context.NewDeviceContextProvider(mockClient, cache)

	ctx1, _ := provider.GetDeviceContext()
	if ctx1.Version != "1" {
		t.Fatalf("unexpected first version")
	}

	clock.Advance(2 * time.Second)
	mockClient.SetResult(second)

	ctx2, _ := provider.GetDeviceContext()
	if ctx2.Version != "2" {
		t.Fatalf("expected refreshed context")
	}

	if mockClient.CallCount() != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", mockClient.CallCount())
	}
}

func TestDeviceContextProvider_TransformsResponse(t *testing.T) {
	clock := mocks.NewFakeClock(time.Now().UTC())
	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)

	raw := &device_context.DeviceContextResponse{
		Version: "1",
		Devices: []device_context.DeviceContext{
			{
				ID:   "dev1",
				Name: "Kitchen Sensor",
				Exposes: []device_context.ExposeInfo{
					{
						Name:         "temperature",
						Type:         "numeric",
						Unit:         "°C",
						Aggregations: []device_context.AggregationType{"last", "avg"},
					},
					{
						Name:    "state",
						Type:    "binary",
						Values:  []string{"OFF", "ON"},
						ValueOn: "ON",
						ValueOff: "OFF",
						Aggregations: []device_context.AggregationType{"last"},
					},
				},
			},
		},
	}

	mockClient := mocks.NewMockHttpDeviceContextFetcher(raw, nil)
	provider := device_context.NewDeviceContextProvider(mockClient, cache)

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

	temp := dev.Exposes["temperature"]
	if temp.Type != "numeric" || temp.Unit != "°C" {
		t.Fatalf("numeric expose not transformed correctly")
	}

	state := dev.Exposes["state"]
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

// HttpDeviceContextFetcher Tests
func TestDeviceContextFetcher_Success(t *testing.T) {
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

	client := device_context.NewHttpDeviceContextFetcher(server.URL, &http.Client{})

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

func TestDeviceContextFetcher_Non200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := device_context.NewHttpDeviceContextFetcher(server.URL, &http.Client{})

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
}

func TestDeviceContextFetcher_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid-json`))
	}))
	defer server.Close()

	client := device_context.NewHttpDeviceContextFetcher(server.URL, &http.Client{})

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected JSON error, got nil")
	}
}

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

func TestDeviceContextFetcher_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := device_context.NewHttpDeviceContextFetcher(server.URL, &http.Client{Timeout: 5 * time.Second})

	_, err := client.FetchDeviceContext()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func sanitiseUrl(url string) string {
	if after, ok := strings.CutPrefix(url, "/"); ok {
		return after
	}
	return url
}
