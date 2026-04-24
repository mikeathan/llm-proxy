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
}

// NewGuardrailEngine creates a new validation engine
func NewGuardrailEngine(provider func() models.AgentGuardrailsConfig, resolver storage.Resolver, persistence *persistence.WorkspaceManager) *GuardrailEngine {
	return &GuardrailEngine{
		configProvider: provider,
		resolver:       resolver,
		persistence:    persistence,
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
		return e.validateTerminal(call, cfg.Terminal)
	case models.ToolInternetSearch:
		return e.validateSearch(call, cfg.Search)
	case models.ToolNotifyUser:
		return e.validateCommunication(call, cfg.Communication)
	case models.ToolDirectoryList, models.ToolFileRead, models.ToolFileWrite:
		return e.validateFileSystem(call, cfg.FileSystem, workspaceID)
	case models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo:
		return e.validateNetwork(call, cfg.Network)
	}

	return nil
}

func (e *GuardrailEngine) validateGlobal(call proxy.ToolCall, cfg models.GlobalGuardrailsConfig) error {
	if cfg.BlockSecrets {
		for _, re := range secretRegexes {
			if re.MatchString(call.Function.Arguments) {
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
		if re.MatchString(call.Function.Arguments) {
			return fmt.Errorf("guardrail violation: blocked pattern detected (%s)", p)
		}
	}
	return nil
}

func (e *GuardrailEngine) validateTerminal(call proxy.ToolCall, cfg models.TerminalGuardrailsConfig) error {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	return tools.ValidateTerminalCommand(args.Command, cfg)
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
		if strings.Contains(call.Function.Arguments, site) {
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
	if !cfg.Enabled {
		return fmt.Errorf("filesystem tools are disabled by guardrails policy")
	}

	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse path: %w", err)
	}

	// 0. System Protection: Strictly block access to hidden files/folders and config files
	// Special Case: Allow "." (current directory) but block other items starting with "."
	base := filepath.Base(args.Path)
	isDot := args.Path == "." || args.Path == "./"
	
	if (!isDot && (strings.HasPrefix(base, ".") || (strings.HasPrefix(args.Path, "..")) || strings.Contains(args.Path, "/."))) ||
		base == models.ConfigFilename || base == models.StateFilename ||
		base == models.LockFilename ||
		base == models.SystemConfigFilename || base == models.SecretsFilename || base == models.RegistryFilename {
		return fmt.Errorf("path access denied: restricted system file or directory (%s)", args.Path)
	}

	// Dynamic Root: Ensure the specific workspace directory is always in the allowed roots
	allowedRoots := append([]string{}, cfg.AllowedPaths...)
	if workspaceID != "" {
		// Use the correct workspaces directory (resolved via central resolver)
		wsPath := e.resolver.WorkspaceDir(workspaceID)
		allowedRoots = append(allowedRoots, wsPath)
	}

	_, err := tools.IsSecurePath(args.Path, allowedRoots)
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
		for _, domain := range cfg.BlockedDomains {
			if strings.Contains(call.Function.Arguments, domain) {
				return fmt.Errorf("guardrail violation: URL contains blocked domain '%s'", domain)
			}
		}
	}

	return nil
}
