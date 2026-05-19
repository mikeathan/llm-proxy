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
func ValidateTerminalCommand(command string, cfg models.TerminalGuardrailsConfig, cache *sync.Map, jailPath string) error {
	if !cfg.Enabled {
		return fmt.Errorf("terminal tools are disabled in configuration")
	}

	// Normalize whitespace: trim and collapse internal spaces to prevent bypasses like "rm  -rf"
	cleanCmd := strings.Join(strings.Fields(command), " ")
	if cleanCmd == "" {
		return fmt.Errorf("empty command")
	}

	// 1. Check Blocked Patterns
	if err := checkBlockedPatterns(cleanCmd, cfg.BlockedPatterns, cache); err != nil {
		return err
	}

	// 2. Check Whitelist (on each segment of a chained command)
	if len(cfg.AllowedCommands) > 0 {
		segments := splitCommandSegments(cleanCmd)
		if err := checkWhitelist(segments, cfg.AllowedCommands); err != nil {
			return err
		}
	}

	// 3. Block Absolute Paths and Parent Traversal (Jail Escape Prevention)
	return checkPathSecurity(cleanCmd, jailPath, cfg.AllowedExternalPaths)
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

func checkWhitelist(segments []string, allowed []string) error {
	for _, seg := range segments {
		words := strings.Fields(seg)
		if len(words) == 0 {
			continue
		}
		baseCmd := words[0]
		isAllowed := false
		for _, a := range allowed {
			if a == baseCmd {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return fmt.Errorf("command '%s' in chain is not in the allowed whitelist", baseCmd)
		}
	}
	return nil
}

func checkPathSecurity(command string, jailPath string, allowedExternal []string) error {
	// 1. Block Absolute Paths unless listed in allowedExternal
	if strings.Contains(command, " /") || strings.HasPrefix(command, "/") {
		absPaths := extractAbsolutePaths(command)
		if len(allowedExternal) == 0 {
			return fmt.Errorf("security violation: absolute paths are not permitted in terminal commands")
		}
		for _, absPath := range absPaths {
			if _, err := IsSecurePath(absPath, allowedExternal); err != nil {
				return fmt.Errorf("security violation: absolute path '%s' is outside allowed external paths", absPath)
			}
		}
	}

	// 2. Dynamic Path Validation: Allow '..' only if it stays within allowed roots
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
			targetPath := filepath.Join(jailPath, cleanWord)
			if _, err := IsSecurePath(targetPath, roots); err != nil {
				return fmt.Errorf("security violation: path '%s' escapes the authorized workspace jail", cleanWord)
			}
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

	// 1. Sanitize and Validate Command
	cleanCmd, err := t.sanitizeCommand(ctx, command, cfg, jailPath)
	if err != nil {
		return "", err
	}

	// 2. Resolve and Validate CWD/Shell
	finalCwd, err := t.resolveCwd(cwd, jailPath, cfg.AllowedExternalPaths)
	if err != nil {
		return "", err
	}
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
		return t.executeShell(execCtx, finalCmd, cfg, wsID, jailPath)
	}

	return t.executeLocal(execCtx, shell, cleanCmd, finalCwd, cfg)
}

// sanitizeCommand handles path forgiveness, traversal validation, and guardrail enforcement in a single pipeline.
func (t *TerminalTools) sanitizeCommand(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig, jailPath string) (string, error) {
	// 1. Initial Guardrail Validation (Whitelist / Blocked Patterns)
	// Skip when the agent already approved this call via the guardrail decision
	// flow — running the same validation twice would reject "Allow Once" approvals.
	if !models.GetGuardrailApproved(ctx) {
		if err := ValidateTerminalCommand(command, cfg, &t.regexCache, jailPath); err != nil {
			return "", err
		}
	}

	// 2. Path Forgiveness: Strip redundant workspace prefixes
	if jailPath != "" {
		cwd, _ := os.Getwd()
		if relPrefix, err := filepath.Rel(cwd, jailPath); err == nil {
			// Normalize separators for the replacement
			sep := string(filepath.Separator)
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

	// For persistent sessions, we send the command directly to the shell's stdin.
	// This allows environment variables and state to persist across calls.
	outStream, errStream, err := ts.Execute(ctx, []string{command})
	if err != nil {
		// If the underlying shell process has exited (e.g. killed by a prior
		// context cancellation), recycle the stale session and retry once.
		if strings.Contains(err.Error(), "shell process exited unexpectedly") {
			// Use a background context so Cleanup doesn't propagate
			// a cancelled context but add a short timeout so we never
			// block the agent turn waiting for process exit.
			recycleCtx, recycleCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer recycleCancel()
			t.shellPool.Recycle(recycleCtx, wsID)
			ts, err = t.shellPool.GetOrCreate(ctx, wsID, jailPath, idleTimeout, cfg.AllowedEnvVars, cfg.PathExtensions)
			if err != nil {
				return "", fmt.Errorf("failed to recreate shell session: %w", err)
			}
			outStream, errStream, err = ts.Execute(ctx, []string{command})
		}
		if err != nil {
			return "", fmt.Errorf("shell execution failed: %w", err)
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
	return t.truncateOutput(buf.String(), cfg.MaxOutputSize), nil
}

func (t *TerminalTools) executeLocal(ctx context.Context, shell, command, cwd string, cfg models.TerminalGuardrailsConfig) (string, error) {
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.CombinedOutput()
	result := t.truncateOutput(string(out), cfg.MaxOutputSize)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %ds", cfg.TimeoutSeconds)
		}
		return result, fmt.Errorf("execution failed: %w", err)
	}

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
		return "", nil
	}
	if cwd == "" {
		return jailPath, nil
	}

	targetPath := filepath.Join(jailPath, cwd)
	roots := buildAllowedRoots(jailPath, allowedExternal)
	resolved, err := IsSecurePath(targetPath, roots)
	if err != nil {
		return "", fmt.Errorf("security violation: cwd '%s' escapes authorized workspace", cwd)
	}
	return resolved, nil
}

func (t *TerminalTools) truncateOutput(result string, maxSize int) string {
	if maxSize > 0 && len(result) > maxSize {
		half := maxSize / 2
		return result[:half] + "\n\n... (output truncated by guardrails) ...\n\n" + result[len(result)-half:]
	}
	return result
}
