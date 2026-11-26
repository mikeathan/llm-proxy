package handlers

import (
	"context"
	"testing"

	"llm-proxy/models"
)

// A local model whose budget comes from a window smaller than 16384 tokens is flagged, so the Models page can say that
// its chats are offered the core tools only. The flag is the agent's own rule (toolpolicy.IsSmallWindow), never a second
// estimate.
func TestGetModelsView_FlagsASmallLocalWindow(t *testing.T) {
	meta := &models.ModelMetadata{ContextLength: 8192}
	list := []models.ModelConfig{
		{Name: "small", Provider: "local", WorkloadClass: models.WorkloadLocal, Path: "/models/small.gguf", Metadata: meta, ContextBudget: 21848},
		{Name: "big", Provider: "local", WorkloadClass: models.WorkloadLocal, Path: "/models/big.gguf", Metadata: meta, ContextBudget: 43692},
		{Name: "cloud", Provider: "openai", WorkloadClass: models.WorkloadCloud, ContextBudget: 20000},
		{Name: "unknown", Provider: "local", WorkloadClass: models.WorkloadLocal, Path: "/models/u.gguf", Metadata: meta},
	}
	want := map[string]bool{"small": true, "big": false, "cloud": false, "unknown": false}
	for _, v := range getModelsView(context.Background(), list, "", false, "127.0.0.1", nil) {
		if v.SmallWindow != want[v.Name] {
			t.Errorf("%s: small_window = %v, want %v", v.Name, v.SmallWindow, want[v.Name])
		}
	}
}
