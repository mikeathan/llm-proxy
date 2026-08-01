package assistant

import (
	"testing"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/models"
)

func TestConfigCache_EvictsOnMaxSize(t *testing.T) {
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Hour, nil)

	const n = 101
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = "ws-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26))
	}
	for _, id := range ids {
		cache.Put(id, &workspaceConfigEntry{config: &models.WorkspaceConfig{}})
	}

	if got := cache.Len(); got != 100 {
		t.Errorf("expected cache bounded at 100 entries, got %d", got)
	}
}

func TestConfigCache_EvictsOnTTL(t *testing.T) {
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Millisecond, nil)
	wsID := "ws-ttl"
	first := &workspaceConfigEntry{config: &models.WorkspaceConfig{}}
	cache.Put(wsID, first)

	time.Sleep(5 * time.Millisecond)

	reloaded, err := cache.Get(wsID, func() (*workspaceConfigEntry, error) {
		return &workspaceConfigEntry{config: &models.WorkspaceConfig{}}, nil
	})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !cache.Contains(wsID) {
		t.Error("expected entry present after reload")
	}
	if reloaded == first {
		t.Error("expected a fresh entry after TTL expiry, got the same pointer (stale entry retained)")
	}
}

func TestGuardrailValidation_UsesCachedConfig(t *testing.T) {
	calls := 0
	cache := core.NewTTLCache[string, *workspaceConfigEntry](100, time.Hour, nil)
	for i := 0; i < 3; i++ {
		cache.Get("ws-cached", func() (*workspaceConfigEntry, error) {
			calls++
			return &workspaceConfigEntry{config: &models.WorkspaceConfig{}}, nil
		})
	}
	if calls != 1 {
		t.Errorf("cache should serve from memory on subsequent hits: got %d loads", calls)
	}
}
