package guardrails

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

// "Allow and remember" changes the workspace's own guardrail layer. What it will
// change is planned in one place: the approval prompt shows the plan's lines
// (PreviewRemember) and the save applies the same plan (PersistOverride), so the
// prompt can never say one thing and the save do another.

// RememberPreview is what "Allow and remember" would do for a blocked step, in
// words. Lines is empty when there is nothing to remember, and Reason says why.
type RememberPreview struct {
	Lines  []string
	Reason string
}

type rememberPlan struct {
	preview RememberPreview
	apply   func(cfg *models.WorkspaceConfig)
}

const (
	reasonUnreadableCommand = "This command can't be read, so nothing can be remembered."
	reasonAlreadyAllowed    = "Every command in it is already allowed, so another rule blocked it. Nothing to remember."
	reasonNothingToRemember = "Nothing can be remembered for this kind of step."
)

func nothingToRemember(reason string) rememberPlan {
	return rememberPlan{preview: RememberPreview{Reason: reason}}
}

// PreviewRemember is what remembering this blocked step would change.
func (e *GuardrailEngine) PreviewRemember(workspaceID, category, args string) RememberPreview {
	return e.planRemember(workspaceID, category, args).preview
}

// PersistOverride saves a guardrail override to the workspace config so
// future tool calls matching this category and pattern are not blocked.
// Also marks the override in the in-memory cache so subsequent checks in the
// same agent loop iteration see it immediately (avoids a file I/O race).
func (e *GuardrailEngine) PersistOverride(workspaceID, category, toolName, args string) error {
	if e.persistence == nil || workspaceID == "" {
		return fmt.Errorf("persistence not available")
	}
	e.ensureReaper()

	cfg, err := e.readConfig(workspaceID)
	if err != nil {
		return fmt.Errorf("read workspace config: %w", err)
	}
	if cfg.Guardrails == nil {
		cfg.Guardrails = &models.AgentGuardrailsConfig{}
	}
	if plan := e.planRemember(workspaceID, category, args); plan.apply != nil {
		plan.apply(cfg)
	}
	e.MarkApproved(workspaceID, proxy.ToolCall{Function: proxy.FunctionCall{Name: toolName, Arguments: args}})
	return e.persistence.WriteConfig(workspaceID, cfg)
}

func (e *GuardrailEngine) planRemember(workspaceID, category, args string) rememberPlan {
	effective := e.mergedCfg(workspaceID)
	switch category {
	case models.CategoryTerminal:
		return planTerminal(args, effective)
	case models.CategoryFileSystem:
		return planFilesystem(args, effective)
	case models.CategoryNetwork:
		return planNetwork(args, effective, e.workspaceLayer(workspaceID))
	case models.CategorySearch:
		return planSearch(effective)
	case models.CategoryCommunication:
		return planCommunication(effective)
	}
	return nothingToRemember(reasonNothingToRemember)
}

// workspaceLayer is the workspace's own guardrail layer, nil when it has none.
func (e *GuardrailEngine) workspaceLayer(workspaceID string) *models.AgentGuardrailsConfig {
	if workspaceID == "" || e.readConfig == nil {
		return nil
	}
	cfg, err := e.readConfig(workspaceID)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.Guardrails
}

// planTerminal remembers the commands the shell would run that are not allowed
// yet — never shell syntax, never what the policy already allows.
func planTerminal(args string, effective models.AgentGuardrailsConfig) rememberPlan {
	var a struct {
		Command string `json:"command"`
	}
	if proxy.DecodeToolArgs(args, &a) != nil || strings.TrimSpace(a.Command) == "" {
		return nothingToRemember(reasonUnreadableCommand)
	}
	names, err := tools.ShellCommandNames(a.Command)
	if err != nil {
		return nothingToRemember(reasonUnreadableCommand)
	}
	var missing []string
	for _, name := range names {
		if !slices.Contains(effective.Terminal.AllowedCommands, name) && !slices.Contains(missing, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nothingToRemember(reasonAlreadyAllowed)
	}
	lines := make([]string, len(missing))
	for i, name := range missing {
		lines[i] = "Allow the command: " + name
	}
	return rememberPlan{
		preview: RememberPreview{Lines: lines},
		apply: func(cfg *models.WorkspaceConfig) {
			terminal := &cfg.Guardrails.Terminal
			for _, name := range missing {
				if !slices.Contains(terminal.AllowedCommands, name) {
					terminal.AllowedCommands = append(terminal.AllowedCommands, name)
				}
			}
		},
	}
}

// planFilesystem remembers the path, and its extension when that is not allowed yet.
func planFilesystem(args string, effective models.AgentGuardrailsConfig) rememberPlan {
	var a struct {
		Path string `json:"path"`
	}
	if proxy.DecodeToolArgs(args, &a) != nil || a.Path == "" {
		return nothingToRemember(reasonNothingToRemember)
	}
	ext := filepath.Ext(a.Path)
	newExt := ext != "" && !slices.ContainsFunc(effective.FileSystem.AllowedExtensions, func(e string) bool { return strings.EqualFold(e, ext) })
	lines := []string{"Allow the path: " + a.Path}
	if newExt {
		lines = append(lines, "Allow the file type: "+ext)
	}
	return rememberPlan{
		preview: RememberPreview{Lines: lines},
		apply: func(cfg *models.WorkspaceConfig) {
			fs := &cfg.Guardrails.FileSystem
			if !slices.Contains(fs.AllowedPaths, a.Path) {
				fs.AllowedPaths = append(fs.AllowedPaths, a.Path)
			}
			if newExt && !slices.Contains(fs.AllowedExtensions, ext) {
				fs.AllowedExtensions = append(fs.AllowedExtensions, ext)
			}
		},
	}
}

// planNetwork stops blocking the host, when the workspace is what blocks it: a
// workspace cannot lift the global policy's blocked domains.
func planNetwork(args string, effective models.AgentGuardrailsConfig, own *models.AgentGuardrailsConfig) rememberPlan {
	var a struct {
		URL string `json:"url"`
	}
	if proxy.DecodeToolArgs(args, &a) != nil || a.URL == "" {
		return nothingToRemember(reasonNothingToRemember)
	}
	host := tools.ExtractHost(a.URL)
	switch {
	case host == "":
		return nothingToRemember(reasonNothingToRemember)
	case own != nil && slices.Contains(own.Network.BlockedDomains, host):
		return rememberPlan{
			preview: RememberPreview{Lines: []string{"Stop blocking the domain: " + host}},
			apply: func(cfg *models.WorkspaceConfig) {
				cfg.Guardrails.Network.BlockedDomains = slices.DeleteFunc(cfg.Guardrails.Network.BlockedDomains, func(d string) bool { return d == host })
			},
		}
	case slices.Contains(effective.Network.BlockedDomains, host):
		return nothingToRemember(host + " is blocked by the global policy, which a workspace cannot lift.")
	}
	return nothingToRemember("Another network rule blocked it, not a blocked domain. Nothing to remember.")
}

func planSearch(effective models.AgentGuardrailsConfig) rememberPlan {
	if effective.Search.Enabled {
		return nothingToRemember("Web search is already on for this workspace. Nothing to remember.")
	}
	return rememberPlan{
		preview: RememberPreview{Lines: []string{"Turn on web search for this workspace"}},
		apply:   func(cfg *models.WorkspaceConfig) { cfg.Guardrails.Search.Enabled = true },
	}
}

// planCommunication is the one remember that changes more than the step: it
// stops asking for approval of every notification in the workspace.
func planCommunication(effective models.AgentGuardrailsConfig) rememberPlan {
	if effective.Communication.Enabled && !effective.Communication.RequireReview {
		return nothingToRemember("Notifications are already on without approval. Nothing to remember.")
	}
	return rememberPlan{
		preview: RememberPreview{Lines: []string{"Turn on notifications for this workspace and stop asking for approval of each one"}},
		apply: func(cfg *models.WorkspaceConfig) {
			cfg.Guardrails.Communication.Enabled = true
			cfg.Guardrails.Communication.RequireReview = false
		},
	}
}
