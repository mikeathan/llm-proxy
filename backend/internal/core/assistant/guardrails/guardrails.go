package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var secretRegexes = []*regexp.Regexp{
	regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
	regexp.MustCompile(`AKIA[a-zA-Z0-9]{16}`),
	regexp.MustCompile(`AIza[a-zA-Z0-9_-]{35}`),
}

// GuardrailEngine evaluates tool calls against configured boundaries.
type GuardrailEngine struct {
	configProvider func() models.AgentGuardrailsConfig
	resolver       storage.Resolver
	persistence    *persistence.WorkspaceManager
	regexCache     sync.Map
}

// NewGuardrailEngine creates a new validation engine
func NewGuardrailEngine(provider func() models.AgentGuardrailsConfig, resolver storage.Resolver, persistence *persistence.WorkspaceManager) *GuardrailEngine {
	return &GuardrailEngine{
		configProvider: provider,
		resolver:       resolver,
		persistence:    persistence,
		regexCache:     sync.Map{},
	}
}

// ValidateToolCall checks a tool call against global and category-specific safety rules.
func (e *GuardrailEngine) ValidateToolCall(ctx context.Context, call proxy.ToolCall, workspaceID string) error {
	cfg := e.configProvider()

	// Load and merge workspace-specific overrides
	if workspaceID != "" && e.persistence != nil {
		if wsCfg, err := e.persistence.ReadConfig(workspaceID); err == nil && wsCfg.Guardrails != nil {
			cfg.MergeWith(wsCfg.Guardrails)
		}
	}

	// 1. Global Guardrails (Sensitive Data)
	if err := e.validateGlobal(call, cfg.Global); err != nil {
		return err
	}

	// 2. Category-Specific Guardrails
	switch call.Function.Name {
	case models.ToolTerminalExecute:
		return e.validateTerminal(call, cfg.Terminal, workspaceID)
	case models.ToolInternetSearch:
		return e.validateSearch(call, cfg.Search)
	case models.ToolNotifyUser:
		return e.validateCommunication(call, cfg.Communication)
	case models.ToolDirectoryList, models.ToolFileRead, models.ToolFileWrite, models.ToolFileAppend:
		return e.validateFileSystem(call, cfg.FileSystem, workspaceID)
	case models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo:
		return e.validateNetwork(call, cfg.Network)
	}

	return nil
}

func (e *GuardrailEngine) validateGlobal(call proxy.ToolCall, cfg models.GlobalGuardrailsConfig) error {
	if cfg.BlockSecrets {
		for _, re := range secretRegexes {
			if re.Match([]byte(call.Function.Arguments)) {
				return fmt.Errorf("guardrail violation: sensitive data detected in tool arguments")
			}
		}
	}

	// Check manual user-defined patterns
	for _, p := range cfg.UserBlocked {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			continue // Skip invalid regex
		}
		if re.Match([]byte(call.Function.Arguments)) {
			return fmt.Errorf("guardrail violation: blocked pattern detected (%s)", p)
		}
	}
	return nil
}

func (e *GuardrailEngine) validateTerminal(call proxy.ToolCall, cfg models.TerminalGuardrailsConfig, workspaceID string) error {
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return fmt.Errorf("missing tool arguments: 'command' field is required")
	}

	var args struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("malformed JSON arguments: %w", err)
	}

	jailPath := ""
	if workspaceID != "" {
		jailPath = e.resolver.WorkspaceDir(workspaceID)
	}

	// Compute effective CWD: if the model specified a subdirectory, the
	// guardrail path check needs it to correctly resolve '..' paths.
	effectiveCwd := jailPath
	if args.Cwd != "" && jailPath != "" {
		effectiveCwd = filepath.Join(jailPath, args.Cwd)
	}

	return tools.ValidateTerminalCommand(args.Command, cfg, &e.regexCache, jailPath, effectiveCwd)
}

func (e *GuardrailEngine) validateSearch(call proxy.ToolCall, cfg models.SearchGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("internet search is disabled by guardrails policy")
	}

	if cfg.MaxQueryLen > 0 && len(call.Function.Arguments) > cfg.MaxQueryLen {
		return fmt.Errorf("search query too long (max %d characters)", cfg.MaxQueryLen)
	}

	// Check for blocked domains in arguments
	for _, site := range cfg.BlockedSites {
		if strings.Contains(string(call.Function.Arguments), site) {
			return fmt.Errorf("guardrail violation: search query contains blocked domain '%s'", site)
		}
	}
	return nil
}

func (e *GuardrailEngine) validateCommunication(call proxy.ToolCall, cfg models.CommunicationGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("communication tools are disabled by guardrails policy")
	}

	if cfg.RequireReview {
		return fmt.Errorf("guardrail check: manual approval required for this communication")
	}
	return nil
}

func (e *GuardrailEngine) validateFileSystem(call proxy.ToolCall, cfg models.FileSystemGuardrailsConfig, workspaceID string) error {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse path: %w", err)
	}

	// Dynamic Root: Ensure the specific workspace directory is always in the allowed roots
	if workspaceID != "" {
		wsPath := e.resolver.WorkspaceDir(workspaceID)
		// Ensure it's at the beginning of the slice so relative paths resolve against it
		cfg.AllowedPaths = append([]string{wsPath}, cfg.AllowedPaths...)
	}

	isWrite := call.Function.Name == models.ToolFileWrite || call.Function.Name == models.ToolFileAppend
	_, err := tools.ValidateFileSystemPath(args.Path, isWrite, cfg)
	return err
}

func (e *GuardrailEngine) validateNetwork(call proxy.ToolCall, cfg models.NetworkGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("network tools are disabled by guardrails policy")
	}

	if call.Function.Name == models.ToolNetworkScan && !cfg.AllowLanAccess {
		return fmt.Errorf("local network scanning is blocked by guardrails policy")
	}

	if call.Function.Name == models.ToolNetworkInfo && !cfg.AllowLanAccess {
		return fmt.Errorf("local network discovery is blocked by guardrails policy")
	}

	// For fetch_url, check domains in arguments
	if call.Function.Name == models.ToolNetworkFetch {
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err == nil {
			// Reuse the same boundary check logic from the tool itself
			host := tools.ExtractHost(args.URL)
			if err := tools.ValidateDomainBoundary(host, cfg.BlockedDomains); err != nil {
				return fmt.Errorf("guardrail violation: %w", err)
			}
		}
	}

	return nil
}

// PersistOverride saves a guardrail override to the workspace config so
// future tool calls matching this category and pattern are not blocked.
func (e *GuardrailEngine) PersistOverride(workspaceID, category, toolName, args string) error {
	if e.persistence == nil || workspaceID == "" {
		return fmt.Errorf("persistence not available")
	}

	cfg, err := e.persistence.ReadConfig(workspaceID)
	if err != nil {
		return fmt.Errorf("read workspace config: %w", err)
	}
	if cfg.Guardrails == nil {
		cfg.Guardrails = &models.AgentGuardrailsConfig{}
	}

	switch category {
	case "terminal":
		var a struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(args), &a) == nil && a.Command != "" {
			for _, base := range tools.ExtractBaseCommands(a.Command) {
					cfg.Guardrails.Terminal.AllowedCommands = append(cfg.Guardrails.Terminal.AllowedCommands, base)
				}
		}

	case "filesystem":
		var a struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(args), &a) == nil && a.Path != "" {
			cfg.Guardrails.FileSystem.AllowedPaths = append(cfg.Guardrails.FileSystem.AllowedPaths, a.Path)
		}

	case "network":
		var a struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(args), &a) == nil && a.URL != "" {
			host := tools.ExtractHost(a.URL)
			if host != "" {
				filtered := make([]string, 0, len(cfg.Guardrails.Network.BlockedDomains))
				for _, d := range cfg.Guardrails.Network.BlockedDomains {
					if d != host {
						filtered = append(filtered, d)
					}
				}
				cfg.Guardrails.Network.BlockedDomains = filtered
			}
		}

	case "search":
		cfg.Guardrails.Search.Enabled = true

	case "communication":
		cfg.Guardrails.Communication.Enabled = true
		cfg.Guardrails.Communication.RequireReview = false
	}

	return e.persistence.WriteConfig(workspaceID, cfg)
}

