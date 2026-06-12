package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

// TerminalTools provides tools for executing local shell commands.
type TerminalTools struct {
	configProvider func(ctx context.Context) models.TerminalGuardrailsConfig
	pathResolver   func(workspaceID string) string
	shellPool      shell.ShellProvider
	observer       StreamObserver
	regexCache     sync.Map
}

// StreamObserver is a callback to broadcast raw terminal output streams to the UI
type StreamObserver func(streamType string, chunk []byte)

// NewTerminalTools initializes a Terminal executable tool.
func NewTerminalTools(
	provider func(ctx context.Context) models.TerminalGuardrailsConfig,
	pathResolver func(workspaceID string) string,
) *TerminalTools {
	return &TerminalTools{
		configProvider: provider,
		pathResolver:   pathResolver,
		regexCache:     sync.Map{},
	}
}

// SetShellProvider injects the Orchestrator for terminal execution
func (t *TerminalTools) SetShellProvider(pool shell.ShellProvider, observer StreamObserver) {
	t.shellPool = pool
	t.observer = observer
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
	return ValidateTerminalCommand(command, t.configProvider(ctx), &t.regexCache, jailPath)
}

// ValidateTerminalCommand is a standalone-like validator that checks a command against guardrails.
func ValidateTerminalCommand(command string, cfg models.TerminalGuardrailsConfig, cache *sync.Map, jailPath string, effectiveCwd ...string) error {
	if !cfg.Enabled {
		return fmt.Errorf("terminal tools are disabled in configuration")
	}

	// Normalize whitespace: trim and collapse internal spaces to prevent bypasses like "rm  -rf"
	cleanCmd := strings.Join(strings.Fields(command), " ")
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

	// 2. Check Whitelist (on each segment of a chained command)
	if len(cfg.AllowedCommands) > 0 {
		segments := splitCommandSegments(cleanCmd)
		if err := assertAllowedCommand(segments, cfg.AllowedCommands); err != nil {
			logging.Debug("guardrail blocked", "rule", "whitelist", "error", err.Error())
			return err
		}
	}

	// 3. Block Absolute Paths and Parent Traversal (Jail Escape Prevention)
	if err := checkPathSecurity(cleanCmd, jailPath, cfg.AllowedExternalPaths, cwd); err != nil {
		logging.Debug("guardrail blocked", "rule", "path_security", "error", err.Error())
		return err
	}
	return nil
}

// SplitCommandSegments decomposes a chained bash command into its individual
// executable parts while respecting shell syntax like quotes and heredocs.
func SplitCommandSegments(command string) []string {
	return splitCommandSegments(command)
}

// ExtractBaseCommands returns the base executable names from each segment
// of a chained command.  "chmod +x file && sh file" → ["chmod", "sh"].
func ExtractBaseCommands(command string) []string {
	segments := splitCommandSegments(command)
	base := make([]string, 0, len(segments))
	for _, seg := range segments {
		words := strings.Fields(seg)
		if len(words) > 0 {
			base = append(base, words[0])
		}
	}
	return base
}

func splitCommandSegments(command string) []string {
	var segments []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inHeredoc := false
	heredocMarker := ""

	chars := []rune(command)
	for i := 0; i < len(chars); i++ {
		c := chars[i]

		// Handle Quotes
		if c == '\'' && !inDoubleQuote && !inHeredoc {
			inSingleQuote = !inSingleQuote
			current.WriteRune(c)
			continue
		}
		if c == '"' && !inSingleQuote && !inHeredoc {
			inDoubleQuote = !inDoubleQuote
			current.WriteRune(c)
			continue
		}

		// Handle Heredoc Start (e.g., <<EOF or <<'EOF')
		if !inSingleQuote && !inDoubleQuote && !inHeredoc {
			if strings.HasPrefix(string(chars[i:]), "<<") {
				inHeredoc = true
				// Extract marker
				markerPart := strings.TrimSpace(string(chars[i+2:]))
				if strings.HasPrefix(markerPart, "'") {
					endIdx := strings.Index(markerPart[1:], "'")
					if endIdx != -1 {
						heredocMarker = markerPart[1 : endIdx+1]
					}
				} else {
					endIdx := strings.IndexAny(markerPart, " \t\n;&|")
					if endIdx != -1 {
						heredocMarker = markerPart[:endIdx]
					} else {
						heredocMarker = markerPart
					}
				}
			}
		}

		// Handle Heredoc End
		if inHeredoc && c == '\n' {
			remaining := string(chars[i+1:])
			if strings.HasPrefix(remaining, heredocMarker) {
				inHeredoc = false
				heredocMarker = ""
			}
		}

		// Handle backslash-escaped delimiters (e.g., \; in find -exec)
	if c == '\\' && i+1 < len(chars) && !inSingleQuote && !inDoubleQuote && !inHeredoc {
		next := string(chars[i+1])
		if next == ";" || next == "|" {
			current.WriteRune(c)
			current.WriteRune(chars[i+1])
			i++
			continue
		}
		if i+2 < len(chars) {
			two := string(chars[i+1]) + string(chars[i+2])
			if two == "&&" || two == "||" {
				current.WriteRune(c)
				current.WriteString(two)
				i += 2
				continue
			}
		}
	}

	// Split on delimiters only if outside quotes/heredocs
		if !inSingleQuote && !inDoubleQuote && !inHeredoc {
			isDelim := false
			// Check longest delimiters first (&&, ||)
			for _, d := range []string{"&&", "||", ";", "|"} {
				if strings.HasPrefix(string(chars[i:]), d) {
					if current.Len() > 0 {
						segments = append(segments, strings.TrimSpace(current.String()))
						current.Reset()
					}
					i += len(d) - 1
					isDelim = true
					break
				}
			}
			if isDelim {
				continue
			}
		}

		current.WriteRune(c)
	}

	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}

	return segments
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

// assertAllowedCommand checks that every command segment starts with a base
// command in the allowed list. Leading shell comments (# ...) are stripped
// before checking so model annotations like "# Step 6\ntsc ..." don't block
// the actual command.
func assertAllowedCommand(segments []string, allowed []string) error {
	for _, seg := range segments {
		words := strings.Fields(seg)
		if len(words) == 0 {
			continue
		}
		baseCmd := words[0]
		if strings.HasPrefix(baseCmd, "#") {
			_, after, ok := strings.Cut(seg, "\n")
			if !ok || strings.TrimSpace(after) == "" {
				continue
			}
			words = strings.Fields(strings.TrimSpace(after))
			if len(words) == 0 {
				continue
			}
			baseCmd = words[0]
		}
		found := false
		for _, a := range allowed {
			if a == baseCmd {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("command '%s' in chain is not in the allowed whitelist", baseCmd)
		}
	}
	return nil
}

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
	shell := t.resolveShell(cfg)

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
		finalCmd := cleanCmd
		if finalCwd != "" {
			finalCmd = fmt.Sprintf("(cd %q && %s)", finalCwd, cleanCmd)
		}
		logging.Debug("ExecuteCommand: executing via shell pool", "finalCmd", finalCmd)
		return t.executeShell(execCtx, finalCmd, cfg, wsID, jailPath)
	}

	logging.Debug("ExecuteCommand: executing locally", "shell", shell, "command", cleanCmd, "cwd", finalCwd)
	return t.executeLocal(execCtx, shell, cleanCmd, finalCwd, cfg)
}

// sanitizeCommand handles path forgiveness and normalization.
// Guardrail validation is handled by the guardrail engine before execution.
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
	return strings.Join(strings.Fields(command), " "), nil
}

func (t *TerminalTools) executeShell(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig, wsID string, jailPath string) (string, error) {
	idleTimeout := time.Duration(cfg.SessionIdleTimeoutSeconds) * time.Second
	ts, err := t.shellPool.GetOrCreate(ctx, wsID, jailPath, idleTimeout, cfg.AllowedEnvVars, cfg.PathExtensions)
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
			ts, getErr = t.shellPool.GetOrCreate(ctx, wsID, jailPath, idleTimeout, cfg.AllowedEnvVars, cfg.PathExtensions)
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
	output := t.truncateOutput(buf.String(), cfg.MaxOutputSize)
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

func (t *TerminalTools) executeLocal(ctx context.Context, shell, command, cwd string, cfg models.TerminalGuardrailsConfig) (string, error) {
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	logging.Debug("executeLocal: running", "shell", shell, "command", command, "cwd", cwd)
	out, err := cmd.CombinedOutput()
	result := t.truncateOutput(string(out), cfg.MaxOutputSize)

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
