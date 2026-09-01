package tools

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// SecretPatterns are regexes for common API-key shapes. They are the single
// source of truth for both input-side validation (guardrails scan tool
// arguments) and output-side scrubbing (terminal results are redacted before
// they reach the agent).
var SecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
	regexp.MustCompile(`AKIA[a-zA-Z0-9]{16}`),
	regexp.MustCompile(`AIza[a-zA-Z0-9_-]{35}`),
}

// RedactSecrets replaces secret-shaped substrings with a placeholder so
// sensitive values cannot leak out through tool output.
func RedactSecrets(s string) string {
	for _, re := range SecretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// IsSecurePath checks if a path is within the allowed roots, resolving symlinks to prevent jail escape.
func IsSecurePath(path string, allowedRoots []string) (string, error) {
	if len(allowedRoots) == 0 {
		return "", fmt.Errorf("security violation: no allowed roots defined")
	}

	absLocal := resolveCanonical(path)
	
	for _, root := range allowedRoots {
		absRoot := resolveCanonical(root)

		// Robust prefix check: handle same path or subpath correctly
		if absLocal == absRoot || strings.HasPrefix(absLocal, absRoot+string(filepath.Separator)) {
			return absLocal, nil
		}
	}

	return "", fmt.Errorf("path access denied: outside of authorized workspaces")
}

// blockedFilename returns the first blocked-filename entry that matches the
// given path's basename, or nil when none match. Matching mirrors the
// filesystem tool's semantics: an entry blocks a basename that equals it or
// starts with it (so "id_rsa" also blocks "id_rsa.pub").
func blockedFilename(path string, blocked []string) *string {
	name := filepath.Base(path)
	for _, b := range blocked {
		if name == b || strings.HasPrefix(name, b) {
			return &b
		}
	}
	return nil
}

// blockedPathEntry returns the blocked entry that a path targets, or nil.
// The terminal tool matches explicit path operands (a path, a subpath, or any
// segment) so "du -sh .sandbox", "find ./.sandbox", and "cat .sandbox/tmp/x"
// are all rejected without blocking legit directory names.
func blockedPathEntry(path string, blocked []string) string {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if match := blockedFilename(seg, blocked); match != nil {
			return *match
		}
	}
	return ""
}

// internalBlockedPaths are internal invariant paths agents must never access —
// currently the sandbox runtime directory (.sandbox).  SINGLE SOURCE OF TRUTH:
// every guardrail surface (filesystem validation, directory listings, terminal
// input, terminal output) enforces this list — merged with the user-configured
// blocked filenames via effectiveBlockedFilenames and matched through the same
// blockedFilename / blockedPathEntry helpers.  Adding an internal path here
// covers all tools at once; no per-tool code is ever added for a new invariant.
var internalBlockedPaths = []string{".sandbox"}

// effectiveBlockedFilenames returns the user-configured blocked filenames plus
// the internal invariant paths — the single merged list every guardrail
// surface enforces.
func effectiveBlockedFilenames(user []string) []string {
	if len(internalBlockedPaths) == 0 {
		return user
	}
	merged := make([]string, 0, len(user)+len(internalBlockedPaths))
	merged = append(merged, user...)
	merged = append(merged, internalBlockedPaths...)
	return merged
}

// resolveCanonical resolves as much of the path as possible using EvalSymlinks.
// For non-existent paths, it recursively resolves parents until an existing directory is found.
func resolveCanonical(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	current := abs
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			if current == abs {
				return resolved
			}
			rel, _ := filepath.Rel(current, abs)
			return filepath.Join(resolved, rel)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return abs
}
