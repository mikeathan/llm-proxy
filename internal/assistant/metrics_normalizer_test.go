package assistant_test

import (
	"testing"
	"time"

	"llm-proxy/internal/assistant"
	"llm-proxy/internal/nodeherder"
)

func TestNormalizeMetrics_TimestampHandling(t *testing.T) {
	resp := &nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		From:   1735689600000,
		To:     1735776000000,
		Values: []nodeherder.MetricsQueryDeviceResponse{
			{
				DeviceId:  "dev1",
				Value:     21.5,
				Timestamp: 1735693200000,
			},
		},
	}

	tests := []struct {
		name       string
		agg        nodeherder.AggregationType
		wantOp     string
		wantTSUnix int64
	}{
		{name: "last", agg: nodeherder.AggLast, wantOp: "last", wantTSUnix: 1735693200000},
		{name: "min", agg: nodeherder.AggMin, wantOp: "min", wantTSUnix: 1735693200000},
		{name: "max", agg: nodeherder.AggMax, wantOp: "max", wantTSUnix: 1735693200000},
		{name: "avg", agg: nodeherder.AggAvg, wantOp: "avg", wantTSUnix: 1735693200000},
		{name: "count", agg: nodeherder.AggCount, wantOp: "count", wantTSUnix: 1735693200000},
		{name: "none", agg: nodeherder.AggNone, wantOp: "", wantTSUnix: 1735693200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assistant.NormalizeMetrics(resp, tt.agg, false)
			if result.Metric != "temperature" {
				t.Fatalf("expected metric temperature, got %s", result.Metric)
			}
			if result.DeviceID != "dev1" {
				t.Fatalf("expected device id dev1, got %s", result.DeviceID)
			}
			if result.Operation != tt.wantOp {
				t.Fatalf("expected operation %q, got %q", tt.wantOp, result.Operation)
			}
			if result.Value != 21.5 {
				t.Fatalf("expected value 21.5, got %v", result.Value)
			}
			if !result.From.Equal(time.UnixMilli(1735689600000)) {
				t.Fatalf("unexpected from time: %v", result.From)
			}
			if !result.To.Equal(time.UnixMilli(1735776000000)) {
				t.Fatalf("unexpected to time: %v", result.To)
			}

			// Verify conditional timestamp fields
			isLast := tt.agg == nodeherder.AggLast || tt.agg == nodeherder.LastEvent
			if isLast {
				if result.LastChanged == nil {
					t.Fatal("expected LastChanged to be set for last aggregation")
				}
				if !result.LastChanged.Equal(time.UnixMilli(tt.wantTSUnix)) {
					t.Fatalf("unexpected LastChanged: %v", result.LastChanged)
				}
				if result.Timestamp != nil {
					t.Fatalf("expected Timestamp to be nil for last aggregation, got %v", result.Timestamp)
				}
			} else {
				// Special case: AggNone usually carries no op string, but might still have timestamp if value present
				// In our table, wantTSUnix is set for all.
				if result.Timestamp == nil {
					t.Fatal("expected Timestamp to be set for non-last aggregation")
				}
				if !result.Timestamp.Equal(time.UnixMilli(tt.wantTSUnix)) {
					t.Fatalf("unexpected Timestamp: %v", result.Timestamp)
				}
				if result.LastChanged != nil {
					t.Fatalf("expected LastChanged to be nil for non-last aggregation, got %v", result.LastChanged)
				}
			}
		})
	}
}

func TestNormalizeMetrics_NilValue(t *testing.T) {
	resp := &nodeherder.MetricsQueryResponse{
		Expose: "humidity",
		From:   1,
		To:     2,
		Values: []nodeherder.MetricsQueryDeviceResponse{
			{
				DeviceId:  "dev2",
				Value:     nil,
				Timestamp: 0,
			},
		},
	}

	result := assistant.NormalizeMetrics(resp, nodeherder.AggAvg, false)
	if result.Value != nil {
		t.Fatalf("expected value to be nil, got %v", result.Value)
	}
	if result.Timestamp != nil {
		t.Fatalf("expected timestamp to be nil, got %v", result.Timestamp)
	}
	if result.LastChanged != nil {
		t.Fatalf("expected LastChanged to be nil, got %v", result.LastChanged)
	}
	if result.Operation != "avg" {
		t.Fatalf("expected operation %q, got %q", "avg", result.Operation)
	}
}
