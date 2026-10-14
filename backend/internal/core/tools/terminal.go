package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/process"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

// Sentinel errors for shell PGID queries.
var (
	ErrShellPoolNotAvailable = errors.New("shell pool not available")
	ErrNoActiveShellSession  = errors.New("no active shell session")
)

// TerminalTools provides tools for executing local shell commands.
type TerminalTools struct {
	configProvider func(ctx context.Context) models.TerminalGuardrailsConfig
	pathResolver   func(workspaceID string) string
	shellPool      shell.ShellProvider
	observer       StreamObserver
	// networkOn reports the run's OS network state for the shell pool key (plan
	// D8). nil ⇒ true (pre-change behavior). Wired from host settings (chat
	// runs) and overridden per-run by the stamped network scope (automation
	// grants, Phase 0.4).
	networkOn func() bool
	// egressEnv returns HTTP(S)_PROXY/NO_PROXY entries when the host egress
	// proxy is enabled (nil = proxy off). Applied to network-on shells only.
	egressEnv func() []string
	// sandboxProv is the OS confinement provider applied at spawn: Landlock on
	// Linux kernels ≥5.13 (FS jail; TCP deny for network-off children on ABI
	// v4+), no-op elsewhere (plan Phase 2).
	sandboxProv sandbox.Provider
	// storageOver reports whether a workspace exceeds its max_storage_gb
	// accounting boundary (best-effort, TTL-cached). nil = check disabled.
	storageOver func(workspaceID, workspaceRoot string) bool
	regexCache  sync.Map
}

// StreamObserver is a callback to broadcast raw terminal output streams to the UI
type StreamObserver func(streamType string, chunk []byte)

// TerminalToolsDeps are the collaborators TerminalTools is wired from at the
// composition root. Required fields must be provided; the optional seams stay
// nil to preserve the pre-sandboxing behavior (network on, no proxy env, no OS
// confinement, no storage boundary). Grouping them keeps construction atomic —
// a TerminalTools can no longer be left half-configured by a forgotten setter.
type TerminalToolsDeps struct {
	ConfigProvider func(ctx context.Context) models.TerminalGuardrailsConfig
	PathResolver   func(workspaceID string) string

	ShellPool shell.ShellProvider // nil ⇒ executeLocal fallback only
	Observer  StreamObserver
	// NetworkOn reports the run's OS network state for the shell pool key (D8).
	// nil ⇒ true (pre-change behavior).
	NetworkOn func() bool
	// EgressEnv returns HTTP(S)_PROXY/NO_PROXY entries when the host egress
	// proxy is enabled (nil = proxy off). Applied to network-on shells only.
	EgressEnv func() []string
	// Sandbox is the OS confinement provider applied at spawn (Landlock on
	// Linux; nil = no OS confinement).
	Sandbox sandbox.Provider
	// StorageOver reports whether a workspace exceeds its max_storage_gb
	// accounting boundary (best-effort, TTL-cached). nil = check disabled.
	StorageOver func(workspaceID, workspaceRoot string) bool
}

// NewTerminalTools initializes a Terminal executable tool from its deps.
func NewTerminalTools(deps TerminalToolsDeps) *TerminalTools {
	return &TerminalTools{
		configProvider: deps.ConfigProvider,
		pathResolver:   deps.PathResolver,
		shellPool:      deps.ShellPool,
		observer:       deps.Observer,
		networkOn:      deps.NetworkOn,
		egressEnv:      deps.EgressEnv,
		sandboxProv:    deps.Sandbox,
		storageOver:    deps.StorageOver,
		regexCache:     sync.Map{},
	}
}

// networkOnValue resolves the effective network state for this run, defaulting
// to on (nil provider) to preserve pre-plan behavior.
func (t *TerminalTools) networkOnValue() bool {
	if t.networkOn != nil {
		return t.networkOn()
	}
	return true
}

// ShellPGID returns the negated process group ID for the active shell
// session in the given workspace. Returns an error when no shell pool
// is configured or no active session exists.
func (t *TerminalTools) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	if t.shellPool == nil {
		return 0, ErrShellPoolNotAvailable
	}
	pgid, ok := t.shellPool.PGID(workspaceID)
	if !ok {
		return 0, fmt.Errorf("%w for workspace %s", ErrNoActiveShellSession, workspaceID)
	}
	return pgid, nil
}

func (t *TerminalTools) Config(ctx context.Context) models.TerminalGuardrailsConfig {
	return t.configProvider(ctx)
}

func (t *TerminalTools) resolveWorkspace(ctx context.Context) (string, string) {
	wsID := models.GetWorkspaceID(ctx)
	var jailPath string
	if wsID != "" && t.pathResolver != nil {
		jailPath = t.pathResolver(wsID)
	}
	return wsID, jailPath
}

// Validate checks if a command is allowed based on the provided configuration.
func (t *TerminalTools) Validate(ctx context.Context, command string) error {
	_, jailPath := t.resolveWorkspace(ctx)
	// The guardrail engine (validateTerminal) is the live enforcement gate and
	// supplies the filesystem blocked-filenames list. This wrapper is a
	// secondary check and intentionally passes none.
	return ValidateTerminalCommand(command, t.configProvider(ctx), nil, &t.regexCache, jailPath)
}

// ValidateTerminalCommand is a standalone-like validator that checks a command against guardrails.
func ValidateTerminalCommand(command string, cfg models.TerminalGuardrailsConfig, blockedFilenames []string, cache *sync.Map, jailPath string, effectiveCwd ...string) error {
	if !cfg.Enabled {
		return fmt.Errorf("terminal tools are disabled in configuration")
	}

	// Read the command the way the shell does. A command the shell could not
	// parse (an unterminated quote or heredoc, a stray `fi`) is never run, and
	// the commands found in the tree — loop bodies, `$(...)`, backticks, `&`
	// jobs included — are what the whitelist and the shell-launcher check judge,
	// so validation and execution cannot read the command differently.
	whitelisted := len(cfg.AllowedCommands) > 0
	commands, err := readCommands(command, whitelisted)
	if err != nil {
		return err
	}

	// Normalize whitespace: trim and collapse internal spaces to prevent bypasses like "rm  -rf",
	// while PRESERVING newlines — multi-line commands are real shell programs (batched commands,
	// heredocs, scripts) and the guardrail must validate the same command that executes.
	cleanCmd := collapseWhitespacePreserveNewlines(command)
	if cleanCmd == "" {
		return fmt.Errorf("empty command")
	}

	cwd := ""
	if len(effectiveCwd) > 0 {
		cwd = effectiveCwd[0]
	}
	logging.Debug("validating terminal command", "command", cleanCmd, "jailPath", jailPath, "effectiveCwd", cwd)

	// 1. Check Blocked Patterns
	if err := checkBlockedPatterns(cleanCmd, cfg.BlockedPatterns, cache); err != nil {
		logging.Debug("guardrail blocked", "rule", "blocked_pattern", "error", err.Error())
		return err
	}

	// 2. Check Blocked Paths (explicit path operands targeting sensitive files
	//    or the sandbox runtime directory). Mirrors the filesystem tool's blocked
	//    filename list so the terminal cannot read what read_file rejects.
	if err := checkBlockedPaths(cleanCmd, blockedFilenames, jailPath); err != nil {
		logging.Debug("guardrail blocked", "rule", "blocked_path", "error", err.Error())
		return err
	}

	// 3. Check Whitelist (every command the shell would run)
	if whitelisted {
		if err := assertAllowedCommand(commandNames(commands), cfg.AllowedCommands); err != nil {
			logging.Debug("guardrail blocked", "rule", "whitelist", "error", err.Error())
			return err
		}
	}

	// 3b. Recurse into shell launchers (bash/sh -c): the embedded argument is a
	// real shell program, so `bash -c '<disallowed>'` would otherwise bypass
	// the whitelist. The inner command is validated with the full
	// pipeline (patterns, paths, whitelist, path security).
	if err := validateShellLaunchers(commands, whitelisted, cfg, blockedFilenames, cache, jailPath, cwd); err != nil {
		logging.Debug("guardrail blocked", "rule", "shell_launcher", "error", err.Error())
		return err
	}

	// 4. Block Absolute Paths and Parent Traversal (Jail Escape Prevention)
	if err := checkPathSecurity(cleanCmd, jailPath, cfg.AllowedExternalPaths, cwd); err != nil {
		logging.Debug("guardrail blocked", "rule", "path_security", "error", err.Error())
		return err
	}
	return nil
}

// readCommands parses a command into the commands the shell would run. A command the
// shell would reject is always an error; one whose commands cannot all be known
// before it runs is an error only when there is an allowed list to check names
// against.
func readCommands(command string, whitelisted bool) ([]shellCommand, error) {
	commands, err := shellCommands(command)
	if err != nil && !whitelisted && errors.Is(err, errUnreadable) {
		return commands, nil
	}
	return commands, err
}

func checkBlockedPatterns(command string, patterns []string, cache *sync.Map) error {
	for _, pattern := range patterns {
		var re *regexp.Regexp
		if cache != nil {
			if val, ok := cache.Load(pattern); ok {
				re = val.(*regexp.Regexp)
			}
		}

		if re == nil {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				continue
			}
			if cache != nil {
				cache.Store(pattern, re)
			}
		}

		if re.MatchString(command) {
			return fmt.Errorf("command contains blocked pattern: %s", pattern)
		}
	}
	return nil
}

// checkBlockedPaths rejects commands whose explicit path operands target a
// blocked sensitive file or an internal invariant path (sandbox runtime
// directory). Only explicit tokens the model writes are inspected —
// environment-driven resolution (HOME=.../.sandbox, path_extensions) is
// untouched, so the sandbox runtime and package managers keep working. The
// error uses the "path access denied" prefix so the guardrail engine
// classifies it as a silent policy block (no allow/deny prompt), matching the
// filesystem tool.
func checkBlockedPaths(command string, blocked []string, jailPath string) error {
	// The effective list merges user-configured blocked filenames with the
	// internal invariant paths (.sandbox) — one gate, one source of truth.
	blocked = effectiveBlockedFilenames(blocked)
	for _, tok := range strings.Fields(command) {
		tok = strings.Trim(tok, `"'`)
		if tok == "" {
			continue
		}
		rel := tok
		if filepath.IsAbs(rel) && jailPath != "" {
			if p, err := filepath.Rel(jailPath, rel); err == nil && !strings.HasPrefix(p, "..") {
				rel = p
			}
		}
		rel = strings.TrimPrefix(rel, "./")
		if m := blockedPathEntry(rel, blocked); m != "" {
			return fmt.Errorf("path access denied: access to sensitive file '%s' is blocked", m)
		}
	}
	return nil
}

// redactBlockedPaths strips lines that reference a blocked path (user-configured
// sensitive files or internal invariant paths like the sandbox runtime
// directory) from terminal output.  The input-side guardrail (checkBlockedPaths)
// already rejects explicit blocked operands, but a recursive traversal ("find .",
// "du -sh .", "ls -la", "tree") emits blocked paths in its OUTPUT — the same
// leak, so those lines are removed before the result reaches the agent.
// Every whitespace-delimited token is checked with the same path-segment
// semantics as the input gate, so "./.sandbox/...", "du ... .sandbox", and
// "drwxr-xr-x .sandbox" are all caught.
func redactBlockedPaths(output string, blocked []string) string {
	if len(blocked) == 0 || !strings.Contains(output, ".") {
		return output
	}
	lines := strings.Split(output, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if lineReferencesBlockedPath(line, blocked) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// lineReferencesBlockedPath reports whether any whitespace token in the line
// targets a blocked path segment.
func lineReferencesBlockedPath(line string, blocked []string) bool {
	for _, tok := range strings.Fields(line) {
		tok = strings.Trim(tok, `"'(),;`)
		if blockedPathEntry(tok, blocked) != "" {
			return true
		}
	}
	return false
}

// scrubOutput applies the same invariants the input gate enforces to terminal
// OUTPUT before it reaches the agent: blocked-path references (recursive
// traversals emit them even without an explicit operand) and secret-shaped
// strings (allowlisted commands like `env` can dump them).
func scrubOutput(output string, blocked []string) string {
	return RedactSecrets(redactBlockedPaths(output, blocked))
}

// assertAllowedCommand checks that every command the shell would run is in the
// allowed list.
func assertAllowedCommand(names []string, allowed []string) error {
	for _, name := range names {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("command '%s' is not in the allowed whitelist", name)
		}
	}
	return nil
}

// alwaysSafeAbsolutePaths are literal paths that carry no security or data
// exposure regardless of the workspace jail, so they are exempt from the
// absolute-path allowlist. Currently only /dev/null — the universal output
// sink (writes are discarded, reads return EOF). It has no subpaths, so
// permitting the literal path does NOT open up /dev/* (/dev/random,
// /dev/urandom, block devices, etc. remain blocked). This is an implicit
// always-allowed set (not config): /dev/null redirects are a standard idiom
// that should never require an allowedexternalpaths entry.
var alwaysSafeAbsolutePaths = []string{"/dev/null"}

// checkPathSecurity blocks absolute paths outside the jail and parent
// traversal.
func checkPathSecurity(command string, jailPath string, allowedExternal []string, effectiveCwd string) error {
	// 1. Block Absolute Paths unless they're within the workspace jail or explicitly allowed.
	//    buildAllowedRoots includes jailPath + allowedExternal, ensuring the workspace
	//    root is always trusted without requiring explicit AllowedExternalPaths config.
	if strings.Contains(command, " /") || strings.HasPrefix(command, "/") {
		absPaths := extractAbsolutePaths(command)
		roots := buildAllowedRoots(jailPath, allowedExternal)
		if len(roots) == 0 {
			return fmt.Errorf("security violation: absolute paths are not permitted in terminal commands")
		}
		for _, absPath := range absPaths {
			if isAlwaysSafeAbsolutePath(absPath) {
				continue
			}
			if _, err := IsSecurePath(absPath, roots); err != nil {
				return fmt.Errorf("security violation: absolute path '%s' is outside allowed paths", absPath)
			}
		}
	}

	// 2. Dynamic Path Validation: Allow '..' only if it stays within allowed roots.
	//    Uses effectiveCwd (the actual execution directory) as the resolution base
	//    so that '../tsconfig.json' from ts-logic-test/ resolves to the workspace root
	//    (within jail) rather than above the jail.
	if strings.Contains(command, "..") {
		roots := buildAllowedRoots(jailPath, allowedExternal)
		if len(roots) == 0 {
			return fmt.Errorf("security violation: parent directory traversal ('..') is not permitted in this context")
		}

		words := strings.Fields(command)
		for _, word := range words {
			if !strings.Contains(word, "..") {
				continue
			}

			cleanWord := strings.Trim(word, `"'`)

			// Try resolving against the workspace root (jailPath) first.
			targetPath := filepath.Join(jailPath, cleanWord)
			if _, err := IsSecurePath(targetPath, roots); err == nil {
				continue
			}

			// Then try the effective CWD — the directory where the command
			// will actually execute (e.g. after a "cd subdir &&" prefix).
			// This handles paths like ../config.json from within a subdirectory
			// that the model explicitly changes into within the command.
			if effectiveCwd != "" && effectiveCwd != jailPath {
				targetPath = filepath.Join(effectiveCwd, cleanWord)
				if _, err := IsSecurePath(targetPath, roots); err == nil {
					continue
				}
			}

			return fmt.Errorf("security violation: path '%s' escapes the authorized workspace jail", cleanWord)
		}
	}
	return nil
}

func buildAllowedRoots(jailPath string, allowedExternal []string) []string {
	roots := make([]string, 0, len(allowedExternal)+1)
	if jailPath != "" {
		roots = append(roots, jailPath)
	}
	roots = append(roots, allowedExternal...)
	return roots
}

// isAlwaysSafeAbsolutePath reports whether an absolute path is in the implicit
// always-allowed set (see alwaysSafeAbsolutePaths). The literal comparison is
// deliberate: only the exact safe path is exempt, so no subpath or sibling in
// the same namespace gains access.
func isAlwaysSafeAbsolutePath(p string) bool {
	for _, safe := range alwaysSafeAbsolutePaths {
		if p == safe {
			return true
		}
	}
	return false
}

func extractAbsolutePaths(command string) []string {
	var paths []string
	for _, word := range strings.Fields(command) {
		word = strings.Trim(word, `"'`)
		if strings.HasPrefix(word, "/") {
			paths = append(paths, word)
		}
	}
	return paths
}

func (t *TerminalTools) ExecuteCommand(ctx context.Context, command string, cwd string) (string, error) {
	cfg := t.configProvider(ctx)
	wsID, jailPath := t.resolveWorkspace(ctx)

	// Workspace disk accounting (max_storage_gb, best-effort). Blocked BEFORE
	// any spawn; message labels it as accounting, never a hard quota.
	if wsID != "" && t.storageOver != nil && t.storageOver(wsID, jailPath) {
		return "", fmt.Errorf("workspace storage accounting limit exceeded (max_storage_gb); free space and retry")
	}

	logging.Debug("ExecuteCommand: start", "command", command, "cwd", cwd, "jailPath", jailPath)

	// 1. Resolve CWD first — the effective execution directory is needed by
	//    the path security check so that '..' paths are resolved against the
	//    actual CWD (e.g. cd subdir &&) rather than always against the jail root.
	finalCwd, err := t.resolveCwd(cwd, jailPath, cfg.AllowedExternalPaths)
	if err != nil {
		logging.Debug("ExecuteCommand: resolveCwd failed", "cwd", cwd, "jailPath", jailPath, "error", err.Error())
		return "", err
	}
	logging.Debug("ExecuteCommand: CWD resolved", "finalCwd", finalCwd)

	// 2. Sanitize and Normalize Command (path forgiveness)
	cleanCmd, err := t.sanitizeCommand(ctx, command, cfg, jailPath)
	if err != nil {
		logging.Debug("ExecuteCommand: sanitize failed", "error", err.Error())
		return "", err
	}
	logging.Debug("ExecuteCommand: command sanitized", "cleanCmd", cleanCmd)
	shellName := t.resolveShell(cfg)

	// 3. Apply hard timeout
	execCtx := ctx
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	if t.shellPool != nil {
		// Use a subshell for the CWD to ensure the command runs in the right place
		// without permanently moving the persistent terminal's session directory.
		// We use the absolute finalCwd to avoid relative path accumulation bugs.
		// The trailing newline before ")" keeps a trailing shell comment (or a
		// heredoc terminator) from swallowing the closing paren.
		finalCmd := cleanCmd
		if finalCwd != "" {
			finalCmd = fmt.Sprintf("(cd %q && %s\n)", finalCwd, cleanCmd)
		}
		logging.Debug("ExecuteCommand: executing via shell pool", "finalCmd", finalCmd)
		return t.executeShell(execCtx, finalCmd, cfg, wsID, jailPath)
	}

	logging.Debug("ExecuteCommand: executing locally", "shell", shellName, "command", cleanCmd, "cwd", finalCwd)
	return t.executeLocal(execCtx, shellName, cleanCmd, finalCwd, cfg, jailPath)
}

// sanitizeCommand handles path forgiveness only. The command is NOT
// whitespace-mangled here: heredoc bodies and string literals are content and
// must reach the shell verbatim. The guardrail engine validates the command
// (its own collapsed copy) before execution; structure (lines/segments) is
// identical between the two because both preserve newlines.
func (t *TerminalTools) sanitizeCommand(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig, jailPath string) (string, error) {
	// Path Forgiveness: Strip redundant workspace prefixes.
	// Replaces the full absolute jail path first, then the relative-from-CWD
	// prefix (e.g. "data/workspaces/workspace-1/") as a secondary fallback.
	if jailPath != "" {
		sep := string(filepath.Separator)
		jailSlash := jailPath + sep

		command = strings.ReplaceAll(command, jailSlash, "./")
		command = strings.ReplaceAll(command, jailPath, ".")

		cwd, _ := os.Getwd()
		if relPrefix, err := filepath.Rel(cwd, jailPath); err == nil {
			prefixWithSlash := relPrefix + sep
			command = strings.ReplaceAll(command, prefixWithSlash, "./")
			command = strings.ReplaceAll(command, relPrefix, ".")
		}
	}
	return strings.TrimSpace(command), nil
}

// collapseWhitespacePreserveNewlines normalizes horizontal whitespace (spaces,
// tabs) to single spaces within each line while preserving the line structure
// of the command. Collapsing newlines would merge distinct commands into one
// ("uname -a\ndate -u" → "uname -a date -u"), breaking both the command and
// the guardrail whitelist checks. Interior blank lines are kept
// (heredoc bodies depend on them); leading/trailing blank lines are dropped.
// Used by the VALIDATION side only — the executed command is never collapsed
// (see sanitizeCommand).
func collapseWhitespacePreserveNewlines(command string) string {
	lines := strings.Split(command, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (t *TerminalTools) executeShell(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig, wsID string, jailPath string) (string, error) {
	// Fail fast on syntactically broken commands instead of wedging the
	// persistent shell: an unterminated quote/heredoc would swallow the
	// completion sentinel and stall until the tool timeout, corrupting the
	// shared session. This is a defense-in-depth guard — the guardrail engine
	// already rejects unbalanced commands, but not every execution path is
	// guaranteed to pass through it.
	if _, err := parseShell(command); err != nil {
		return "", err
	}
	policy := shell.WorkspacePolicy{
		AllowedEnvVars: cfg.AllowedEnvVars,
		PathExtensions: cfg.PathExtensions,
		NetworkOn:      t.networkOnValue(),
		IdleTimeout:    time.Duration(cfg.SessionIdleTimeoutSeconds) * time.Second,
	}
	// A stamped run scope (automation grant, D8) is authoritative for the shell
	// pool key; absent (chat) falls back to the configured provider (host).
	if s, ok := models.RunNetworkScopeFrom(ctx); ok {
		policy.NetworkOn = s.NetworkOn()
	}
	// Egress-proxy env applies only to network-on shells (off shells have no
	// network to proxy).
	if policy.NetworkOn && t.egressEnv != nil {
		if vars := t.egressEnv(); len(vars) > 0 {
			policy.ProxyEnv = vars
		}
	}
	policy.Sandbox = t.sandboxProv
	ts, err := t.shellPool.GetOrCreate(ctx, wsID, jailPath, policy)
	if err != nil {
		return "", fmt.Errorf("failed to get/create shell session for workspace %s: %w", wsID, err)
	}
	logging.Debug("executeShell: running command", "wsID", wsID, "jailPath", jailPath, "command", command)

	// For persistent sessions, we send the command directly to the shell's stdin.
	// This allows environment variables and state to persist across calls.
	outStream, errStream, execErr := ts.Execute(ctx, []string{command})
	if execErr != nil {
		// If the underlying shell process has exited (e.g. killed by a prior
		// context cancellation), recycle the stale session and retry once.
		if strings.Contains(execErr.Error(), "shell process exited unexpectedly") {
			// Use a background context so Cleanup doesn't propagate
			// a cancelled context but add a short timeout so we never
			// block the agent turn waiting for process exit.
			recycleCtx, recycleCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer recycleCancel()
			t.shellPool.Recycle(recycleCtx, wsID)
			var getErr error
			ts, getErr = t.shellPool.GetOrCreate(ctx, wsID, jailPath, policy)
			if getErr != nil {
				return "", fmt.Errorf("failed to recreate shell session: %w", getErr)
			}
			outStream, errStream, execErr = ts.Execute(ctx, []string{command})
		}
	}

	// We use a combined buffer to ensure we capture both stdout and stderr in order
	var buf bytes.Buffer
	var wg sync.WaitGroup

	readAndTee := func(r io.ReadCloser, streamType string) {
		if r == nil {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.Close()
			b := make([]byte, 1024)
			for {
				n, err := r.Read(b)
				if n > 0 {
					buf.Write(b[:n])
					if t.observer != nil {
						t.observer(streamType, b[:n])
					}
				}
				if err != nil {
					break
				}
			}
		}()
	}

	readAndTee(outStream, "stdout")
	readAndTee(errStream, "stderr")

	wg.Wait()
	output := t.truncateOutput(scrubOutput(buf.String(), internalBlockedPaths), cfg.MaxOutputSize)
	if execErr != nil {
		logging.Debug("executeShell: error", "output_len", len(output), "error", execErr.Error())
		return output, fmt.Errorf("shell execution failed: %w", execErr)
	}
	if output == "" {
		logging.Debug("executeShell: success, no output")
		return "[Command executed successfully with no output]", nil
	}
	logging.Debug("executeShell: success", "output_len", len(output))
	return output, nil
}

// newCommand creates an exec.Cmd via the shared process.AgentCommand helper
// (Setpgid isolation is centralized there — plan R1).
func (t *TerminalTools) newCommand(ctx context.Context, shell, command, cwd string) *exec.Cmd {
	cmd := process.AgentCommand(ctx, shell, "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	return cmd
}

// executeLocal runs a one-shot command without the pooled shell (used when no
// shell provider is configured or as the fallback path). Since R1 it receives
// the workspace root so the child gets the SAME env floor as pooled shells
// (plan Risk 4 — previously it inherited the backend env, leaking real HOME
// when filesystem confinement was off). workspaceRoot == "" (no workspace)
// keeps the historical inherit-backend-env behavior.
func (t *TerminalTools) executeLocal(ctx context.Context, shellName, command, cwd string, cfg models.TerminalGuardrailsConfig, workspaceRoot string) (string, error) {
	cmd := t.newCommand(ctx, shellName, command, cwd)
	if workspaceRoot != "" {
		cmd.Env = shell.BuildSandboxEnv(workspaceRoot, cfg.AllowedEnvVars, cfg.PathExtensions)
	}
	// OS confinement for one-shots: Landlock on Linux (workspace-rooted
	// one-shots); no-op elsewhere or for rootless one-shots.
	if t.sandboxProv != nil {
		if err := t.sandboxProv.Wrap(cmd, sandbox.WorkspaceView{RootPath: workspaceRoot, NetworkOn: t.networkOnValue()}); err != nil {
			return "", fmt.Errorf("failed to sandbox local command: %w", err)
		}
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = process.KillGroup(cmd, syscall.SIGKILL)
		case <-done:
		}
	}()

	logging.Debug("executeLocal: running", "shell", shellName, "command", command, "cwd", cwd)
	out, err := cmd.CombinedOutput()
	result := t.truncateOutput(scrubOutput(string(out), internalBlockedPaths), cfg.MaxOutputSize)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logging.Debug("executeLocal: timed out", "output_len", len(result))
			return result, fmt.Errorf("command timed out after %ds", cfg.TimeoutSeconds)
		}
		logging.Debug("executeLocal: error", "output_len", len(result), "error", err.Error())
		return result, fmt.Errorf("execution failed: %w", err)
	}

	if result == "" {
		logging.Debug("executeLocal: success, no output")
		return "[Command executed successfully with no output]", nil
	}
	logging.Debug("executeLocal: success", "output_len", len(result))
	return result, nil
}

func (t *TerminalTools) resolveShell(cfg models.TerminalGuardrailsConfig) string {
	if cfg.DefaultShell != "" {
		return cfg.DefaultShell
	}
	return "bash"
}

func (t *TerminalTools) resolveCwd(cwd, jailPath string, allowedExternal []string) (string, error) {
	if jailPath == "" {
		logging.Debug("resolveCwd: no jailPath", "cwd", cwd)
		return "", nil
	}
	if cwd == "" {
		logging.Debug("resolveCwd: no cwd, defaulting to jailPath", "jailPath", jailPath)
		return jailPath, nil
	}

	targetPath := filepath.Join(jailPath, cwd)
	roots := buildAllowedRoots(jailPath, allowedExternal)
	resolved, err := IsSecurePath(targetPath, roots)
	if err != nil {
		logging.Debug("resolveCwd: rejected", "cwd", cwd, "jailPath", jailPath, "resolvedPath", targetPath, "error", err.Error())
		return "", fmt.Errorf("security violation: cwd '%s' escapes authorized workspace", cwd)
	}

	if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() {
		logging.Debug("resolveCwd: resolved path does not exist, falling back to jailPath", "cwd", cwd, "jailPath", jailPath, "resolvedPath", resolved)
		return jailPath, nil
	}

	logging.Debug("resolveCwd: resolved", "cwd", cwd, "jailPath", jailPath, "finalCwd", resolved)
	return resolved, nil
}

func (t *TerminalTools) truncateOutput(result string, maxSize int) string {
	if maxSize > 0 && len(result) > maxSize {
		half := maxSize / 2
		return result[:half] + "\n\n... (output truncated by guardrails) ...\n\n" + result[len(result)-half:]
	}
	return result
}
