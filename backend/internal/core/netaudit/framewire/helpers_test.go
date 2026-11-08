package framewire

import (
	"encoding/json"
	"testing"

	"llm-proxy/models"
)

func models0() models.CaptureSummary { return models.CaptureSummary{} }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
