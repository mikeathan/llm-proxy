package assistant

import (
	"testing"
	"time"
)

func TestBuildMetricsQueryRequest_SanitizesFromTo(t *testing.T) {
	argJSON := `{
		"device_id": "dev1",
		"expose": "temperature",
		"time": {
			"from": 0,
			"to": 1735776000000,
			"lookback": "24h"
		}
	}`

	req, err := buildMetricsQueryRequest(argJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Time == nil {
		t.Fatal("expected time query to be set")
	}

	if !req.Time.From.IsZero() {
		t.Fatalf("expected zero from time, got %v", req.Time.From)
	}

	wantTo := time.UnixMilli(1735776000000)
	if !req.Time.To.Equal(wantTo) {
		t.Fatalf("expected to=%v, got %v", wantTo, req.Time.To)
	}

	if req.Time.Lookback != "24h" {
		t.Fatalf("expected lookback 24h, got %q", req.Time.Lookback)
	}
}

func TestBuildMetricsQueryRequest_AllowsLookbackOnly(t *testing.T) {
	argJSON := `{
		"device_id": "dev1",
		"expose": "temperature",
		"time": {
			"lookback": "24h"
		}
	}`

	req, err := buildMetricsQueryRequest(argJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Time == nil {
		t.Fatal("expected time query to be set")
	}

	if req.Time.Lookback != "24h" {
		t.Fatalf("expected lookback 24h, got %q", req.Time.Lookback)
	}

	if !req.Time.From.IsZero() || !req.Time.To.IsZero() {
		t.Fatalf("expected zero from/to for lookback-only, got %v %v", req.Time.From, req.Time.To)
	}
}
