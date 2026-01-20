package devices

import (
	"llm-proxy/internal/nodeherder"
	"testing"
)

func TestResolveDevice_Fuzzy(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "1",
				Name: "Attic air sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "co2"},
					{Name: "temperature"},
				},
			},
			{
				ID:   "2",
				Name: "Living Room Light",
				Exposes: []nodeherder.LLMExpose{
					{Name: "state"},
				},
			},
		},
	}

	tests := []struct {
		target      string
		metric      string
		expectID    string
		expectError bool
		ambiguous   bool
	}{
		// Exact match
		{"Attic air sensor", "co2", "1", false, false},
		// Case insensitive
		{"attic air sensor", "CO2", "1", false, false},
		// Fuzzy match
		{"attic room", "co2", "1", false, false},
		{"attic", "co2", "1", false, false},
		// No match for metric
		{"attic air sensor", "humidity", "", true, false},
		// No match for device
		{"garage", "co2", "", true, false},
	}

	for _, tt := range tests {
		dev, err := ResolveDevice(ctx, tt.target, tt.metric)
		if tt.expectError {
			if err == nil {
				t.Errorf("ResolveDevice(%q, %q) expected error, got nil", tt.target, tt.metric)
			}
			continue
		}
		if err != nil {
			t.Errorf("ResolveDevice(%q, %q) unexpected error: %v", tt.target, tt.metric, err)
			continue
		}
		if dev.ID != tt.expectID {
			t.Errorf("ResolveDevice(%q, %q) = %s, want %s", tt.target, tt.metric, dev.ID, tt.expectID)
		}
	}
}

func TestResolveDevice_Ambiguous(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "1",
				Name: "Attic Switch",
				Exposes: []nodeherder.LLMExpose{
					{Name: "state"},
				},
			},
			{
				ID:   "2",
				Name: "Attic Light",
				Exposes: []nodeherder.LLMExpose{
					{Name: "state"},
				},
			},
		},
	}

	// "attic" matches both equally well (both have just "attic" in common with target)
	// Actually "attic" target against "Attic Switch" -> score 0.5 (attic matched, switch missed)
	// against "Attic Light" -> score 0.5
	// Should be ambiguous.

	_, err := ResolveDevice(ctx, "attic", "state")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}

	amb, ok := err.(*AmbiguousDeviceError)
	if !ok {
		t.Fatalf("expected AmbiguousDeviceError, got %T: %v", err, err)
	}

	if len(amb.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(amb.Candidates))
	}
}

func TestDetectMultipleDevices(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{ID: "1", Name: "Living Room Light"},
			{ID: "2", Name: "Attic room Light"},
			{ID: "3", Name: "Garden Sensor"},
		},
	}

	tests := []struct {
		message     string
		expectMulti bool
		desc        string
	}{
		{"is the living room light on", false, "single device - should not flag"},
		{"what's the attic temperature", false, "single device - should not flag"},
		{"living room and garden", true, "distinct locations - should flag"},
		{"attic and garden sensors", true, "distinct locations - should flag"},
		{"what's the temperature", false, "no specific device - should not flag"},
	}

	for _, tt := range tests {
		result := DetectMultipleDevices(tt.message, ctx)
		gotMulti := len(result) > 1
		if gotMulti != tt.expectMulti {
			t.Errorf("%s: DetectMultipleDevices(%q) = %v, want multi=%v", tt.desc, tt.message, result, tt.expectMulti)
		}
	}
}
