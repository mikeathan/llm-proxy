package guardrails

import (
	"context"
	"fmt"
	"llm-proxy/internal/core"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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
	readConfig     func(workspaceID string) (*models.WorkspaceConfig, error) // cached reader (O3)
	regexCache     sync.Map

	// overrideCache stores in-memory guardrail approvals per (workspaceID, toolName)
	// so that "Allow & Remember" decisions are effective immediately without waiting
	// for the workspace config file write to propagate.  Key format: "workspaceID/toolName".
	// Entries are reaped after overrideTTL (PL-5) by the cache's background reaper.
	// Cleared on server restart (config file is the durable source).
	overrideCache *core.TTLCache[string, struct{}]

	// reaperInterval is the override-cache reaper cadence. The reaper is started
	// lazily on the first override write so engines that never persist overrides
	// (e.g. NewAgent's nil-safety fallback engine) do not leak a goroutine.
	reaperInterval time.Duration
}

// defaultOverrideTTL and defaultReaperInterval bound override-cache growth.
const (
	defaultOverrideTTL    = 30 * time.Minute
	defaultReaperInterval = time.Minute
)

// NewGuardrailEngine creates a new validation engine
func NewGuardrailEngine(provider func() models.AgentGuardrailsConfig, resolver storage.Resolver, persistence *persistence.WorkspaceManager, readConfig func(workspaceID string) (*models.WorkspaceConfig, error)) *GuardrailEngine {
	return newGuardrailEngine(provider, resolver, persistence, readConfig, defaultOverrideTTL, defaultReaperInterval)
}

func newGuardrailEngine(provider func() models.AgentGuardrailsConfig, resolver storage.Resolver, persistence *persistence.WorkspaceManager, readConfig func(workspaceID string) (*models.WorkspaceConfig, error), overrideTTL, reaperInterval time.Duration) *GuardrailEngine {
	e := &GuardrailEngine{
		configProvider: provider,
		resolver:       resolver,
		persistence:    persistence,
		readConfig:     readConfig,
		regexCache:     sync.Map{},
		overrideCache:  core.NewTTLCache[string, struct{}](0, overrideTTL, nil),
		reaperInterval: reaperInterval,
	}
	return e
}

// ensureReaper starts the override-cache reaper on the first override write.
// Construction never starts a goroutine: engines that never persist or mark an
// override (notably NewAgent's nil-safety fallback engine) would otherwise
// leak one reaper goroutine per instance (Constitution II.14).
func (e *GuardrailEngine) ensureReaper() {
	e.overrideCache.Start(e.reaperInterval)
}

// Stop terminates the override reaper goroutine. Safe to call multiple times.
func (e *GuardrailEngine) Stop() {
	e.overrideCache.Stop()
}

// ValidateToolCall checks a tool call against global and category-specific safety rules.
func (e *GuardrailEngine) ValidateToolCall(ctx context.Context, call proxy.ToolCall, workspaceID string) error {
	// Fast path: check in-memory override cache first — avoids the file I/O race
	// between PersistOverride writing and this function reading the updated config.
	if workspaceID != "" && e.hasOverride(workspaceID, call.Function.Name) {
		return nil
	}

	cfg := e.configProvider()

	// Load and merge workspace-specific overrides.
	// Uses the cached reader (O3) to avoid file I/O on every tool call.
	if workspaceID != "" && e.readConfig != nil {
		if wsCfg, err := e.readConfig(workspaceID); err == nil && wsCfg.Guardrails != nil {
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
		return e.validateTerminal(call, cfg.Terminal, workspaceID, cfg.FileSystem.BlockedFilenames)
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

func (e *GuardrailEngine) validateTerminal(call proxy.ToolCall, cfg models.TerminalGuardrailsConfig, workspaceID string, blockedFilenames []string) error {
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return fmt.Errorf("missing tool arguments: 'command' field is required")
	}

	var args struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := proxy.DecodeToolArgs(call.Function.Arguments, &args); err != nil {
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

	return tools.ValidateTerminalCommand(args.Command, cfg, blockedFilenames, &e.regexCache, jailPath, effectiveCwd)
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
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return fmt.Errorf("missing tool arguments: 'path' field is required")
	}

	var args struct {
		Path string `json:"path"`
	}
	if err := proxy.DecodeToolArgs(call.Function.Arguments, &args); err != nil {
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
		if err := proxy.DecodeToolArgs(call.Function.Arguments, &args); err == nil {
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

	switch category {
	case "terminal":
		var a struct {
			Command string `json:"command"`
		}
		if proxy.DecodeToolArgs(args, &a) == nil && a.Command != "" {
			for _, base := range tools.ExtractBaseCommands(a.Command) {
				cfg.Guardrails.Terminal.AllowedCommands = append(cfg.Guardrails.Terminal.AllowedCommands, base)
			}
		}

	case "filesystem":
		var a struct {
			Path string `json:"path"`
		}
		if proxy.DecodeToolArgs(args, &a) == nil && a.Path != "" {
			cfg.Guardrails.FileSystem.AllowedPaths = append(cfg.Guardrails.FileSystem.AllowedPaths, a.Path)
			ext := filepath.Ext(a.Path)
			if ext != "" {
				alreadyAllowed := false
				for _, e := range cfg.Guardrails.FileSystem.AllowedExtensions {
					if strings.EqualFold(e, ext) {
						alreadyAllowed = true
						break
					}
				}
				if !alreadyAllowed {
					cfg.Guardrails.FileSystem.AllowedExtensions = append(cfg.Guardrails.FileSystem.AllowedExtensions, ext)
				}
			}
		}

	case "network":
		var a struct {
			URL string `json:"url"`
		}
		if proxy.DecodeToolArgs(args, &a) == nil && a.URL != "" {
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

	e.MarkOverride(workspaceID, toolName)

	return e.persistence.WriteConfig(workspaceID, cfg)
}

// DisabledToolNames returns the tool names whose category is statically
// disabled by guardrail policy (the Communication/Search/Network `Enabled`
// gates), with workspace overrides merged exactly as ValidateToolCall resolves
// them. It mirrors ValidateToolCall's hard "disabled by policy" gates only —
// RequireReview and the allowlist/blocked-domain categories (terminal,
// filesystem) are execution-time gates and are intentionally NOT covered. Tools
// with an active in-memory override are skipped (consistent with the
// ValidateToolCall fast path). This is the single source the exposed tool
// schema derives from, so no strategy or channel can observe a tool the policy
// statically disables.
func (e *GuardrailEngine) DisabledToolNames(workspaceID string) []string {
	cfg := e.configProvider()
	if workspaceID != "" && e.readConfig != nil {
		if wsCfg, err := e.readConfig(workspaceID); err == nil && wsCfg.Guardrails != nil {
			cfg.MergeWith(wsCfg.Guardrails)
		}
	}
	var disabled []string
	add := func(name string, disabledByPolicy bool) {
		// Override skip mirrors ValidateToolCall's fast path exactly:
		// in-memory overrides only count for a non-empty workspaceID.
		if disabledByPolicy && (workspaceID == "" || !e.hasOverride(workspaceID, name)) {
			disabled = append(disabled, name)
		}
	}
	add(models.ToolNotifyUser, !cfg.Communication.Enabled)
	add(models.ToolInternetSearch, !cfg.Search.Enabled)
	add(models.ToolNetworkFetch, !cfg.Network.Enabled)
	add(models.ToolNetworkScan, !cfg.Network.Enabled)
	add(models.ToolNetworkInfo, !cfg.Network.Enabled)
	return disabled
}

// hasOverride checks the in-memory override cache for a (workspaceID, toolName) pair.
// Uses toolName (e.g. "notify_user") as the key suffix so ValidateToolCall can check
// without needing to derive the category.
func (e *GuardrailEngine) hasOverride(workspaceID, toolName string) bool {
	return e.overrideCache.Contains(workspaceID + "/" + toolName)
}

// MarkOverride stores an in-memory guardrail approval for the given
// (workspaceID, toolName) so subsequent tool calls in the same session
// skip the guardrail check without waiting for the config file write.
// Uses toolName (not category) to match the key format in hasOverride.
func (e *GuardrailEngine) MarkOverride(workspaceID, toolName string) {
	e.ensureReaper()
	e.overrideCache.Put(workspaceID+"/"+toolName, struct{}{})
}
