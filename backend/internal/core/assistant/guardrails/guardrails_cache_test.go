package guardrails

import (
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestGuardrailCache_TTL(t *testing.T) {
	e := newGuardrailEngine(
		func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} },
		storage.NewPathResolver("", "", ""),
		nil,
		nil,
		5*time.Millisecond, // overrideTTL
		time.Millisecond,   // reaperInterval
	)
	defer e.Stop()

	e.MarkOverride("ws", "read_file")
	if !e.hasOverride("ws", "read_file") {
		t.Fatal("expected override present immediately after MarkOverride")
	}

	time.Sleep(20 * time.Millisecond) // exceed overrideTTL

	if e.hasOverride("ws", "read_file") {
		t.Error("expected override to expire after TTL (lazy check)")
	}
}

func TestGuardrailCache_ReaperRemovesStale(t *testing.T) {
	e := newGuardrailEngine(
		func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} },
		storage.NewPathResolver("", "", ""),
		nil,
		nil,
		5*time.Millisecond,
		time.Millisecond,
	)
	defer e.Stop()

	e.MarkOverride("ws2", "read_file")
	time.Sleep(20 * time.Millisecond) // let the reaper run and expire it

	if e.overrideCache.Len() != 0 {
		t.Errorf("reaper did not remove stale override entry, len=%d", e.overrideCache.Len())
	}
}
