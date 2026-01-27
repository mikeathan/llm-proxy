package nodeherder_test

import (
	"strings"
	"testing"

	"llm-proxy/internal/nodeherder"
)

func TestSummaryWithLimit_Truncates(t *testing.T) {
	ctx := &nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{ID: "dev1", Name: "One", Exposes: []nodeherder.LLMExpose{{Name: "temperature"}, {Name: "humidity"}}},
			{ID: "dev2", Name: "Two", Exposes: []nodeherder.LLMExpose{{Name: "power"}, {Name: "energy"}}},
			{ID: "dev3", Name: "Three", Exposes: []nodeherder.LLMExpose{{Name: "pressure"}, {Name: "battery"}}},
		},
	}

	out := ctx.SummaryWithLimit(60)
	if len(out) > 60 {
		t.Fatalf("expected summary to be truncated, got len %d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Fatalf("expected truncation notice, got %q", out)
	}
}
