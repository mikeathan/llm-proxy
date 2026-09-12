package guardrails

import (
	"context"
	"errors"
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

// ErrNetworkDisabled is the security-boundary denial raised when agent network
// is OFF for the current context: the host switch is explicitly OFF, or the
// run's resolved scope (automation grant) is none (plan D1/R4). Consumers must
// treat it as synchronous and non-approvable (never the approval flow) and
// overrides must never re-enable past it. Classified via errors.Is in the agent
// loop (isGuardrailSecurityBoundary).
var ErrNetworkDisabled = errors.New("agent network is denied by security policy")

// hostNetworkGatedTools are the agent tools whose execution requires agent
// egress. When the host network switch is off they are hidden from the schema
// (DisabledToolNames) and hard-denied (ValidateToolCall) regardless of workspace
// overrides or persisted approvals. notify_user (connector sends) and
// internet_search are agent egress and are included; inbound webhook receipt is
// server-side and unaffected.
var hostNetworkGatedTools = []string{
	models.ToolNetworkFetch,
	models.ToolNetworkScan,
	models.ToolNetworkInfo,
	models.ToolInternetSearch,
	models.ToolNotifyUser,
}

// hostNetworkGatedSet mirrors hostNetworkGatedTools for O(1) lookup.
var hostNetworkGatedSet = func() map[string]bool {
	set := make(map[string]bool, len(hostNetworkGatedTools))
	for _, n := range hostNetworkGatedTools {
		set[n] = true
	}
	return set
}()

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

	// hostNetworkAllowed reports whether the host sandboxing.network switch
	// permits agent network. nil ⇒ no host gate (network governed purely by the
	// guardrail tier). The gate sits OUTSIDE the MergeWith override stack: when
	// it reports false, network-gated tools are schema-hidden and hard-denied
	// regardless of workspace overrides or persisted approvals (plan D1/R4).
	hostNetworkAllowed func() bool

	// searchAvailable reports whether an internet-search provider is usable
	// (provider registered and its key present). nil ⇒ no gate (search
	// availability governed purely by the Search.Enabled tier), so engines built
	// without this predicate keep their previous behaviour. When it reports
	// false, internet_search is hidden from the schema.
	searchAvailable func() bool
}

// SetHostNetworkAllowed installs the host-level network allowance provider
// (e.g. a closure over AppContext host settings). A nil provider leaves the
// host gate inert — call with func() bool { return cfg.NetworkAllowed() } from
// the composition root.
func (e *GuardrailEngine) SetHostNetworkAllowed(allowed func() bool) {
	e.hostNetworkAllowed = allowed
}

// SetSearchAvailable installs the live internet-search availability provider
// (a closure over AppContext config + secrets, returned by initSearchTools). A
// nil provider leaves the gate inert — call from the composition root.
func (e *GuardrailEngine) SetSearchAvailable(available func() bool) {
	e.searchAvailable = available
}

// searchUnavailable reports an explicit "no provider configured" state. A nil
// predicate leaves the gate inert (existing behaviour unchanged).
func (e *GuardrailEngine) searchUnavailable() bool {
	return e.searchAvailable != nil && !e.searchAvailable()
}

// searchDisabled reports whether internet_search cannot run: its guardrail tier
// is disabled or no usable provider is configured. One helper feeds every schema
// surface so availability and policy cannot drift.
func (e *GuardrailEngine) searchDisabled(cfg models.AgentGuardrailsConfig) bool {
	return !cfg.Search.Enabled || e.searchUnavailable()
}

// hostNetworkOff reports an explicit host-level network denial. Undecided host
// config (nil pointer ⇒ legacy allowed) is NOT off — only an explicit false
// trips the gate (models.HostSandboxingConfig.NetworkAllowed semantics).
func (e *GuardrailEngine) hostNetworkOff() bool {
	return e.hostNetworkAllowed != nil && !e.hostNetworkAllowed()
}

// networkDenied reports whether agent network is denied for the current run
// context: an explicitly stamped run scope of none (automation grant) wins over
// the host switch; otherwise the host switch decides.
func (e *GuardrailEngine) networkDenied(ctx context.Context) bool {
	if s, ok := models.RunNetworkScopeFrom(ctx); ok {
		return s == models.NetworkScopeNone
	}
	return e.hostNetworkOff()
}

// mergedCfg returns the guardrail config with the workspace override stack
// merged (settings default + {workspace}/config.yaml overrides). Single source
// for validation, schema resolution, and run-scope resolution so they never
// diverge.
func (e *GuardrailEngine) mergedCfg(workspaceID string) models.AgentGuardrailsConfig {
	cfg := e.configProvider()
	if workspaceID != "" && e.readConfig != nil {
		if wsCfg, err := e.readConfig(workspaceID); err == nil && wsCfg.Guardrails != nil {
			cfg.MergeWith(wsCfg.Guardrails)
		}
	}
	return cfg
}

// ResolveRunScope resolves the effective per-run network scope for an agent
// run: host ceiling (L0) ∩ workspace guardrails (L1) overridden by an explicit
// automation grant (L2). Used by the executor to stamp the run context.
func (e *GuardrailEngine) ResolveRunScope(workspaceID string, grant models.NetworkScope) models.NetworkScope {
	hostAllowed := !e.hostNetworkOff()
	return e.mergedCfg(workspaceID).Network.EffectiveScope(hostAllowed, grant)
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
	// Security hard gate FIRST — before the override fast path and the MergeWith
	// stack: overrides must never re-enable a network-gated tool when the run
	// scope or the host switch denies network (plan D1/R4).
	if err := e.securityHardGates(ctx, call); err != nil {
		return err
	}

	// Fast path: check in-memory override cache first — avoids the file I/O race
	// between PersistOverride writing and this function reading the updated config.
	if workspaceID != "" && e.hasOverride(workspaceID, call.Function.Name) {
		return nil
	}

	cfg := e.mergedCfg(workspaceID)
	// 1. Global Guardrails (Sensitive Data)
	if err := e.validateGlobal(call, cfg.Global); err != nil {
		return err
	}

	// 2. Category-Specific Guardrails
	return e.validateCategory(ctx, call, cfg, workspaceID)
}

// securityHardGates holds the non-approvable, override-proof denials that must
// fire before any config merge: the host/run network ceiling (D1/R4) and the
// scope-forbidden egress tools (lan ⇔ internet-only, internet_only ⇔ local
// network). The forbidden sets come from scopeForbiddenTools — the same source
// the schema uses — so availability and denial can never drift.
func (e *GuardrailEngine) securityHardGates(ctx context.Context, call proxy.ToolCall) error {
	if e.networkDenied(ctx) && hostNetworkGatedSet[call.Function.Name] {
		return ErrNetworkDisabled
	}
	if scope, ok := models.RunNetworkScopeFrom(ctx); ok {
		for _, forbidden := range scopeForbiddenTools(scope) {
			if forbidden == call.Function.Name {
				return fmt.Errorf("%w: %s is not permitted at this run's network scope (%s)", ErrNetworkDisabled, call.Function.Name, scope)
			}
		}
	}
	return nil
}

// validateCategory dispatches a call to its category validator against the
// merged (workspace-tier) config.
func (e *GuardrailEngine) validateCategory(ctx context.Context, call proxy.ToolCall, cfg models.AgentGuardrailsConfig, workspaceID string) error {
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
		// An explicitly stamped run scope (automation grant) replaces the
		// workspace L1 net flags for this run: it may loosen (grant in a
		// none-workspace) within the host ceiling. Scope mapping is centralized
		// in models.WithRunScope — scope none is already hard-denied above
		// (securityHardGates), lan/internet validate against the scope-derived
		// config so lan cannot reach the internet.
		if s, ok := models.RunNetworkScopeFrom(ctx); ok {
			if vcfg, scoped := cfg.Network.WithRunScope(s); scoped {
				return e.validateNetwork(call, vcfg)
			}
		}
		return e.validateNetwork(call, cfg.Network)
	}
	return nil
}

func (e *GuardrailEngine) validateGlobal(call proxy.ToolCall, cfg models.GlobalGuardrailsConfig) error {
	if cfg.BlockSecrets {
		// Secret patterns live in tools.SecretPatterns — the same source the
		// terminal tool uses to scrub secret-shaped strings from output.
		for _, re := range tools.SecretPatterns {
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
	cfg := e.mergedCfg(workspaceID)
	var disabled []string
	add := func(name string, disabledByPolicy bool) {
		// Override skip mirrors ValidateToolCall's fast path exactly:
		// in-memory overrides only count for a non-empty workspaceID.
		if disabledByPolicy && (workspaceID == "" || !e.hasOverride(workspaceID, name)) {
			disabled = append(disabled, name)
		}
	}
	add(models.ToolNotifyUser, !cfg.Communication.Enabled)
	add(models.ToolInternetSearch, e.searchDisabled(cfg))
	add(models.ToolNetworkFetch, !cfg.Network.Enabled)
	add(models.ToolNetworkScan, !cfg.Network.Enabled)
	add(models.ToolNetworkInfo, !cfg.Network.Enabled)
	if e.hostNetworkOff() {
		// Host-level hard gate OUTSIDE the override stack: unlike the policy adds
		// above, an in-memory/persisted override must NOT keep the tool visible
		// when the host switch is off (plan D1/R4). Append unconditionally.
		for _, name := range hostNetworkGatedTools {
			if !containsString(disabled, name) {
				disabled = append(disabled, name)
			}
		}
	}
	return disabled
}

// DisabledToolNamesForScope resolves schema availability for a run with an
// explicitly resolved scope (automation grant, plan §4.4). Scope none hides
// every egress tool unconditionally (overrides cannot win). A lan scope shows
// the core LAN-able tools (fetch/scan/info) regardless of the workspace L1
// network gate (loosening within the host ceiling) but hides the internet-only
// tools (internet_search, notify_user — connector sends are internet egress);
// an internet scope additionally exposes those when their L1 tier is enabled.
// Scope-forbidden tools are hard-gated: overrides never re-expose them.
// Inherit/unknown delegates to DisabledToolNames.
func (e *GuardrailEngine) DisabledToolNamesForScope(workspaceID string, scope models.NetworkScope) []string {
	if scope == models.NetworkScopeInherit || !scope.Valid() {
		return e.DisabledToolNames(workspaceID)
	}
	if scope == models.NetworkScopeNone {
		// Every egress tool is hidden and overrides can never re-expose them
		// (return a copy — callers must not mutate the shared list).
		return append([]string(nil), hostNetworkGatedTools...)
	}
	cfg := e.mergedCfg(workspaceID)
	disabled := e.scopeTierDisabled(workspaceID, scope, cfg)
	// Hard gate: an override must not keep a scope-forbidden egress tool
	// visible (scope lan forbids the internet-only tools).
	for _, name := range scopeForbiddenTools(scope) {
		if !containsString(disabled, name) {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

// scopeTierDisabled applies the L1-tier hides for a lan/internet scope,
// honoring in-memory overrides: a lan scope hides the internet-only egress
// tools (search + connector send) unless the operator's workspace tier already
// disables them. The LAN-only tools are hidden by scopeForbiddenTools for an
// internet_only scope instead.
func (e *GuardrailEngine) scopeTierDisabled(workspaceID string, scope models.NetworkScope, cfg models.AgentGuardrailsConfig) []string {
	lanScope := scope == models.NetworkScopeLan // LAN scope must not expose internet-only egress
	var disabled []string
	add := func(name string, disabledByPolicy bool) {
		if disabledByPolicy && (workspaceID == "" || !e.hasOverride(workspaceID, name)) {
			disabled = append(disabled, name)
		}
	}
	add(models.ToolInternetSearch, lanScope || e.searchDisabled(cfg))
	add(models.ToolNotifyUser, lanScope || !cfg.Communication.Enabled)
	return disabled
}

// scopeForbiddenTools returns the egress tools a resolved scope must never
// expose — even with an override: none forbids every network-gated tool, lan
// forbids the internet-only ones (search + connector send), internet_only
// forbids the local-network ones (scan + network info); internet forbids none
// (its ceiling is the host switch + L1 tiers).
func scopeForbiddenTools(scope models.NetworkScope) []string {
	switch scope {
	case models.NetworkScopeLan:
		return []string{models.ToolInternetSearch, models.ToolNotifyUser}
	case models.NetworkScopeInternetOnly:
		return []string{models.ToolNetworkScan, models.ToolNetworkInfo}
	case models.NetworkScopeNone:
		return hostNetworkGatedTools
	default:
		return nil
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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
