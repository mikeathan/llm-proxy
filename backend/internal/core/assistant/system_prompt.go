package assistant

import (
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

const (
	agentsCacheKey            = "agents:"
	agentsFileCacheMaxEntries = 100              // per-workspace cache bound (PL-6)
	agentsFileCacheTTL        = 30 * time.Minute // stale entries evicted lazily on Get
)

// agentsFileCache memoizes AGENTS.md reads per workspace, bounded at
// agentsFileCacheMaxEntries to prevent unbounded growth for long-lived servers.
// Entries expire after agentsFileCacheTTL (lazy eviction on Get) so edits to
// AGENTS.md are picked up without a restart.
var agentsFileCache = core.NewTTLCache[string, string](agentsFileCacheMaxEntries, agentsFileCacheTTL, nil)

// LoadAgentsFile returns the workspace's own notes from AGENTS.md, as the model should see them: comments removed.
// It is empty when the file is missing, holds only comments (the seeded stub) or is one of the old defaults this app
// used to seed — a persona that now lives in the profiles, which a workspace must not keep layering on top of them.
func LoadAgentsFile(pm *persistence.WorkspaceManager, workspaceID string) string {
	if workspaceID == "" || pm == nil {
		return ""
	}
	notes, err := agentsFileCache.Get(agentsCacheKey+workspaceID, func() (string, error) {
		content, err := pm.ReadTaskFile(workspaceID, models.RulesFilename)
		if err != nil || prompts.IsLegacyDefaultNotes(content) {
			return "", nil
		}
		return models.WithoutComments(content), nil
	})
	if err != nil {
		return ""
	}
	return notes
}

// BuildSystemPrompt layers a run's system prompt — the profile's behaviour, the core rules, the workspace's notes —
// for the tool-call mode in use. Chat and automation both build it here.
func BuildSystemPrompt(pm *persistence.WorkspaceManager, workspaceID string, p profile.Profile, useNativeTools bool) string {
	return prompts.AssembleSystemPrompt(prompts.PromptParts{Native: useNativeTools, Profile: p.Instructions, Notes: LoadAgentsFile(pm, workspaceID)})
}
