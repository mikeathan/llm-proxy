package app

import (
	"context"
	"testing"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

func newSystemTestServer(t *testing.T) *AppContext {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LLM_PROXY_HOME", dir)
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	dataMgr, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := dataMgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	srv := NewServer(mocks.NewMockManager(), dataMgr)
	t.Cleanup(srv.metrics.Stop)
	return srv
}

// The two global hot-memory defaults are saved independently of each other and of the rest of the memory settings.
func TestApplySystemUpdate_MemoryHotDefaults(t *testing.T) {
	srv := newSystemTestServer(t)
	if got := srv.GetSettings().Memory.HotDefaults(); got != (models.MemoryHotDefaults{AssistantHot: true, AutomationHot: false}) {
		t.Fatalf("shipped defaults = %+v, want assistant on, automations off", got)
	}
	retention := srv.GetSettings().Memory.RetentionDays

	off := models.MemoryHotDefaults{AssistantHot: false, AutomationHot: true}
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Memory: &off}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	set := srv.GetSettings()
	if got := set.Memory.HotDefaults(); got != off {
		t.Errorf("saved defaults = %+v, want %+v", got, off)
	}
	if set.Memory.RetentionDays != retention {
		t.Errorf("retention changed from %d to %d: only the hot defaults may be written", retention, set.Memory.RetentionDays)
	}

	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{}); err != nil {
		t.Fatalf("ApplySystemUpdate without memory: %v", err)
	}
	if got := srv.GetSettings().Memory.HotDefaults(); got != off {
		t.Errorf("an update that omits memory changed the defaults to %+v", got)
	}
}
