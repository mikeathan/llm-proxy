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
	"strings"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
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

	// Fail closed on syntactically broken commands: an unterminated quote or
	// heredoc makes the tail of the command opaque to the whitelist (the
	// scanner would otherwise swallow it into one segment, letting a
	// disallowed command — e.g. `rm -rf /` after `cat <<-EOF` — bypass the
	// allowlist). The segments from this scan are also reused for the
	// whitelist below so validation and the segment analysis never diverge.
	segments, balanced := scanCommandSegments(command)
	if !balanced {
		return fmt.Errorf("command has an unterminated quote or heredoc")
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

	// 3. Check Whitelist (on each segment of a chained command)
	if len(cfg.AllowedCommands) > 0 {
		if err := assertAllowedCommand(segments, cfg.AllowedCommands); err != nil {
			logging.Debug("guardrail blocked", "rule", "whitelist", "error", err.Error())
			return err
		}
	}

	// 4. Block Absolute Paths and Parent Traversal (Jail Escape Prevention)
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
	segments, _ := scanCommandSegments(command)
	return segments
}

// commandDelimiters are the chain separators splitCommandSegments honors,
// longest first. Newline is a shell command separator too.
var commandDelimiters = [][]byte{[]byte("&&"), []byte("||"), []byte(";"), []byte("|"), []byte("\n")}

// scanCommandSegments decomposes a chained bash command into its individual
// executable parts while respecting shell syntax: quotes, heredocs, and
// escaped delimiters. It also reports whether the command's shell syntax is
// balanced at EOF (no unterminated quote or heredoc).
//
// Balanced reporting matters because an unterminated heredoc or quote makes
// the tail of the command opaque to the whitelist: the scanner would swallow
// it into one segment, and a disallowed command after it (e.g. `rm -rf /`
// following `cat <<-EOF`) would bypass the allowlist. Callers must treat
// unbalanced commands as invalid rather than guessing at segment boundaries.
func scanCommandSegments(command string) (segments []string, balanced bool) {
	chars := []byte(command)
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inHeredoc := false
	heredocMarker := ""
	heredocStripTabs := false

	flush := func() {
		if current.Len() > 0 {
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}

	for i := 0; i < len(chars); {
		c := chars[i]

		// Handle Quotes
		if c == '\'' && !inDoubleQuote && !inHeredoc {
			inSingleQuote = !inSingleQuote
			current.WriteByte(c)
			i++
			continue
		}
		if c == '"' && !inSingleQuote && !inHeredoc {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(c)
			i++
			continue
		}

		// Handle Heredoc Start (e.g., <<EOF, <<-EOF, <<'EOF', <<"EOF",
		// <<\EOF). "<<<" is a here-string, not a heredoc — it must not open
		// heredoc mode, otherwise a `&&` after it would be swallowed and a
		// disallowed command would slip past the per-segment whitelist. The
		// char-before check keeps the SECOND '<' of "<<<" from re-triggering
		// the heredoc branch on the here-string's remaining "<".
		if !inSingleQuote && !inDoubleQuote && !inHeredoc && c == '<' &&
			i+1 < len(chars) && chars[i+1] == '<' &&
			(i == 0 || chars[i-1] != '<') &&
			(i+2 >= len(chars) || chars[i+2] != '<') {
			if marker, stripTabs, ok := parseHeredocMarker(chars, i+2); ok {
				inHeredoc = true
				heredocMarker = marker
				heredocStripTabs = stripTabs
			}
		}

		// Handle Heredoc End: a newline whose next line is the marker word
		// (optionally tab-prefixed for <<-). The terminating newline stays in
		// the segment so the marker line remains attached to the heredoc.
		if inHeredoc && c == '\n' {
			if heredocLineEnds(chars[i+1:], heredocMarker, heredocStripTabs) {
				inHeredoc = false
				heredocMarker = ""
				heredocStripTabs = false
				current.WriteByte(c)
				i++
				continue
			}
		}

		// Handle backslash-escaped delimiters (e.g., \; in find -exec)
		if c == '\\' && i+1 < len(chars) && !inSingleQuote && !inDoubleQuote && !inHeredoc {
			next := chars[i+1]
			if next == ';' || next == '|' {
				current.WriteByte(c)
				current.WriteByte(next)
				i += 2
				continue
			}
			if i+2 < len(chars) && ((chars[i+1] == '&' && chars[i+2] == '&') || (chars[i+1] == '|' && chars[i+2] == '|')) {
				current.WriteByte(c)
				current.WriteByte(chars[i+1])
				current.WriteByte(chars[i+2])
				i += 3
				continue
			}
		}

		// Split on delimiters only if outside quotes/heredocs.
		if !inSingleQuote && !inDoubleQuote && !inHeredoc {
			isDelim := false
			for _, d := range commandDelimiters {
				if bytes.HasPrefix(chars[i:], d) {
					flush()
					i += len(d)
					isDelim = true
					break
				}
			}
			if isDelim {
				continue
			}
		}

		current.WriteByte(c)
		i++
	}

	flush()
	return segments, !inSingleQuote && !inDoubleQuote && !inHeredoc
}

// parseHeredocMarker extracts the terminator word from a heredoc opening
// positioned just after "<<" (start = index of the char following "<<").
// Handles the tab-strip flag (<<-EOF), quoting (<<'EOF', <<"EOF") and
// escaping (<<\EOF). Returns ok=false when no marker word is present.
func parseHeredocMarker(chars []byte, start int) (marker string, stripTabs bool, ok bool) {
	j := start
	if j < len(chars) && chars[j] == '-' {
		stripTabs = true
		j++
	}
	// Quoted or escaped markers: bash strips the quotes/backslash and
	// terminates on the bare word, so the marker ends at the closing quote.
	quoted := false
	if j < len(chars) && (chars[j] == '\'' || chars[j] == '"' || chars[j] == '\\') {
		quoted = chars[j] != '\\'
		j++
	}
	markerStart := j
	for j < len(chars) {
		b := chars[j]
		if quoted && (b == '\'' || b == '"') {
			break
		}
		if b == ' ' || b == '\t' || b == '\n' || b == ';' || b == '&' || b == '|' {
			break
		}
		j++
	}
	if j == markerStart {
		return "", false, false
	}
	return string(chars[markerStart:j]), stripTabs, true
}

// heredocLineEnds reports whether rest begins with the heredoc terminator
// marker as its own word (followed by end-of-input or a separator). For
// tab-stripped heredocs (<<-), leading tabs before the marker are allowed.
func heredocLineEnds(rest []byte, marker string, stripTabs bool) bool {
	if stripTabs {
		rest = bytes.TrimLeft(rest, "\t")
	}
	if !bytes.HasPrefix(rest, []byte(marker)) {
		return false
	}
	after := rest[len(marker):]
	return len(after) == 0 || after[0] == '\n' || after[0] == '\r' || after[0] == ' ' || after[0] == '\t'
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
		// The trailing newline before ")" keeps a trailing shell comment (or a
		// heredoc terminator) from swallowing the closing paren.
		finalCmd := cleanCmd
		if finalCwd != "" {
			finalCmd = fmt.Sprintf("(cd %q && %s\n)", finalCwd, cleanCmd)
		}
		logging.Debug("ExecuteCommand: executing via shell pool", "finalCmd", finalCmd)
		return t.executeShell(execCtx, finalCmd, cfg, wsID, jailPath)
	}

	logging.Debug("ExecuteCommand: executing locally", "shell", shell, "command", cleanCmd, "cwd", finalCwd)
	return t.executeLocal(execCtx, shell, cleanCmd, finalCwd, cfg)
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
// the guardrail's per-segment whitelist checks. Interior blank lines are kept
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
	if _, balanced := scanCommandSegments(command); !balanced {
		return "", fmt.Errorf("command has an unterminated quote or heredoc")
	}
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
	output := t.truncateOutput(redactBlockedPaths(buf.String(), internalBlockedPaths), cfg.MaxOutputSize)
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

// newCommand creates an exec.Cmd with process group isolation (Setpgid)
// already applied. Separated for testability of the isolation behavior.
func (t *TerminalTools) newCommand(ctx context.Context, shell, command, cwd string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func (t *TerminalTools) executeLocal(ctx context.Context, shell, command, cwd string, cfg models.TerminalGuardrailsConfig) (string, error) {
	cmd := t.newCommand(ctx, shell, command, cwd)

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	logging.Debug("executeLocal: running", "shell", shell, "command", command, "cwd", cwd)
	out, err := cmd.CombinedOutput()
	result := t.truncateOutput(redactBlockedPaths(string(out), internalBlockedPaths), cfg.MaxOutputSize)

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
