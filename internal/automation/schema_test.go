package automation_test

import (
	"testing"

	"llm-proxy/internal/automation"
)

func TestMetricsToolSchema(t *testing.T) {
	schema := automation.MetricsToolSchema()

	if schema.Type != "function" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if schema.Function.Name != "query_metrics" {
		t.Fatalf("unexpected function name: %s", schema.Function.Name)
	}

	params, ok := schema.Function.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("expected required to be []string")
	}

	required := map[string]bool{}
	for _, v := range params {
		required[v] = true
	}

	for _, k := range []string{"device_id", "expose", "from", "to"} {
		if !required[k] {
			t.Fatalf("missing required field: %s", k)
		}
	}
}
