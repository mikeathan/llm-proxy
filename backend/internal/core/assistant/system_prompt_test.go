package assistant

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestLoadAgentsFile(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	const wsID = "ws1"
	write := func(t *testing.T, content string) {
		t.Helper()
		agentsFileCache.Clear()
		if err := mgr.WriteTaskFile(wsID, models.RulesFilename, content); err != nil {
			t.Fatalf("write task file: %v", err)
		}
	}

	t.Run("no notes when no file exists", func(t *testing.T) {
		agentsFileCache.Clear()
		if got := LoadAgentsFile(mgr, "no-such-workspace"); got != "" {
			t.Errorf("got %q, want no notes", got)
		}
	})

	t.Run("the operator's notes, without the comments that explain the file", func(t *testing.T) {
		write(t, "<!-- how to use this file -->\n# Mine\nAlways respond in French.\n")
		if got := LoadAgentsFile(mgr, wsID); got != "# Mine\nAlways respond in French." {
			t.Errorf("got %q", got)
		}
	})

	t.Run("the seeded stub adds nothing", func(t *testing.T) {
		write(t, prompts.DefaultAgentsMD)
		if got := LoadAgentsFile(mgr, wsID); got != "" {
			t.Errorf("the comment-only stub must add nothing, got %q", got)
		}
	})

	t.Run("an old seeded default is no notes: the persona now lives in the profiles", func(t *testing.T) {
		legacy, err := os.ReadFile("prompts/legacy/agents-5b89301.md")
		if err != nil {
			t.Fatal(err)
		}
		write(t, string(legacy))
		if got := LoadAgentsFile(mgr, wsID); got != "" {
			t.Errorf("a workspace still holding the old default must get the profile's behaviour, got notes %q", got)
		}
		write(t, string(legacy)+"\n- Our host is vertex.")
		if got := LoadAgentsFile(mgr, wsID); !strings.Contains(got, "Our host is vertex.") {
			t.Errorf("a file the operator changed is kept as their notes, got %q", got)
		}
	})

	t.Run("no notes without a workspace or a manager", func(t *testing.T) {
		agentsFileCache.Clear()
		if got := LoadAgentsFile(mgr, ""); got != "" {
			t.Errorf("empty workspaceID: got %q", got)
		}
		if got := LoadAgentsFile(nil, wsID); got != "" {
			t.Errorf("nil manager: got %q", got)
		}
	})
}

func TestAgentsFileCache_BoundedEviction(t *testing.T) {
	agentsFileCache.Clear()

	for i := 0; i < agentsFileCacheMaxEntries+1; i++ {
		key := agentsCacheKey + fmt.Sprintf("ws-%d", i)
		agentsFileCache.Put(key, fmt.Sprintf("content-%d", i))
	}

	if n := agentsFileCache.Len(); n > agentsFileCacheMaxEntries {
		t.Fatalf("cache exceeded max entries: got %d, want ≤%d", n, agentsFileCacheMaxEntries)
	}

	if agentsFileCache.Contains(agentsCacheKey + "ws-0") {
		t.Error("oldest entry ws-0 should have been evicted when cache was at capacity")
	}
	if !agentsFileCache.Contains(agentsCacheKey + fmt.Sprintf("ws-%d", agentsFileCacheMaxEntries)) {
		t.Error("most recent entry should still be present after eviction")
	}
}
