package devices_test

import (
	"errors"
	"testing"

	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/nodeherder"
)

func TestResolveDevice_ExactMatch(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Living Room Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	}

	device, err := devices.ResolveDevice(ctx, "living room", "temperature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if device.ID != "dev1" {
		t.Fatalf("expected dev1, got %s", device.ID)
	}
}

func TestResolveDevice_Ambiguous(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Attic Air Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
			{
				ID:   "dev2",
				Name: "Attic Room Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	}

	_, err := devices.ResolveDevice(ctx, "attic", "temperature")
	var amb *devices.AmbiguousDeviceError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousDeviceError, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(amb.Candidates))
	}
}

func TestResolveDevice_PrefersHigherScore(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Living Room Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
			{
				ID:   "dev2",
				Name: "Living Room Temperature Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	}

	device, err := devices.ResolveDevice(ctx, "living room temperature", "temperature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if device.ID != "dev2" {
		t.Fatalf("expected dev2, got %s", device.ID)
	}
}

func TestResolveDevice_NoMatch(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Garden Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "humidity"},
				},
			},
		},
	}

	if _, err := devices.ResolveDevice(ctx, "kitchen", "temperature"); err == nil {
		t.Fatal("expected error")
	}
}
