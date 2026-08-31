package tools

import (
	"context"
	"errors"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRedactBlockedPaths verifies terminal OUTPUT is scrubbed of blocked-path
// references — both user-configured sensitive files and internal invariant
// paths like the sandbox runtime dir.  Recursive commands ("find .",
// "du -sh .", "ls -la") emit those paths even though the input-side guardrail
// blocks explicit operands — the output must not leak them either.  Both lists
// flow through the same merged mechanism (effectiveBlockedFilenames) and the
// same path-segment matching, so adding an internal path later needs no new
// code here.
func TestRedactBlockedPaths(t *testing.T) {
	userBlocked := []string{".env", "id_rsa"}
	internal := internalBlockedPaths
	merged := effectiveBlockedFilenames(userBlocked)

	cases := []struct {
		name    string
		in      string
		blocked []string
		want    string
	}{
		{"find traversal output", "./.sandbox/.npm/_cacache/content-v2/x\nAGENTS.md\n", internal, "AGENTS.md\n"},
		{"find with prefix", "drwxr-xr-x .sandbox\nnotes.txt\n", internal, "notes.txt\n"},
		{"du output", "12\t./.sandbox\n4\t./docs\n", internal, "4\t./docs\n"},
		{"ls -la with sandbox", "total 16\ndrwxr-xr-x .sandbox\n-rw-r--r-- file.txt\n", internal, "total 16\n-rw-r--r-- file.txt\n"},
		{"deep sandbox path", "workspace-1/.sandbox/tmp/x\nkeep.txt\n", internal, "keep.txt\n"},
		{"no sandbox untouched", "hello\nworld\n", internal, "hello\nworld\n"},
		{"empty output", "", internal, ""},
		{"user blocked filename redacted via merged list", ".env.production\nconfig.yml\n", merged, "config.yml\n"},
		{"internal path redacted via merged list", ".sandbox/x\nkeep\n", merged, "keep\n"},
		{"user list alone does not catch internal path", ".sandbox/x\n.env\n", userBlocked, ".sandbox/x\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactBlockedPaths(c.in, c.blocked); got != c.want {
				t.Fatalf("redactBlockedPaths(%q, %v) = %q, want %q", c.in, c.blocked, got, c.want)
			}
		})
	}
}

func TestTerminalTools_Guardrails(t *testing.T) {
	tests := []struct {
		name             string
		config           models.TerminalGuardrailsConfig
		blockedFilenames []string
		command          string
		wantErr          bool
		errContains      string
	}{
		{
			name: "Allowed command works",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"ls", "echo"},
			},
			command: "ls -la",
			wantErr: false,
		},
		{
			name: "Whitelisted command rejection",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"ls"},
			},
			command:     "rm -rf /",
			wantErr:     true,
			errContains: "not in the allowed whitelist",
		},
		{
			name: "Blocked pattern rejection (command chaining)",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"ls"},
				BlockedPatterns: []string{";", "\\|"},
			},
			command:     "ls; rm -rf /",
			wantErr:     true,
			errContains: "blocked pattern",
		},
		{
			name: "Tools disabled",
			config: models.TerminalGuardrailsConfig{
				Enabled: false,
			},
			command:     "ls",
			wantErr:     true,
			errContains: "disabled",
		},
		{
			name: "Empty command",
			config: models.TerminalGuardrailsConfig{
				Enabled: true,
			},
			command:     "   ",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name: "Whitespace normalization bypass",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				BlockedPatterns: []string{"rm -rf"},
			},
			command:     "rm  -rf /", // Double space
			wantErr:     true,
			errContains: "blocked pattern",
		},
		{
			name: "Blocked path: sandbox dir via du",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"du"},
			},
			command:     "du -sh .sandbox",
			wantErr:     true,
			errContains: "path access denied: access to sensitive file",
		},
		{
			name: "Blocked path: sandbox subpath via find",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"find"},
			},
			command:     "find .sandbox -type f",
			wantErr:     true,
			errContains: "path access denied",
		},
		{
			name: "Blocked path: explicit ./sandbox prefix",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"du"},
			},
			command:     "du -sh ./.sandbox",
			wantErr:     true,
			errContains: "path access denied",
		},
		{
			name: "Blocked path: sensitive .env via cat",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"cat"},
			},
			blockedFilenames: []string{".env"},
			command:          "cat .env",
			wantErr:          true,
			errContains:      "sensitive file",
		},
		{
			name: "Blocked path: ssh key basename",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"cat"},
			},
			blockedFilenames: []string{".ssh", "id_rsa"},
			command:          "cat .ssh/id_rsa",
			wantErr:          true,
			errContains:      "sensitive file",
		},
		{
			name: "Blocked path: allowed glob does not trigger",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"du"},
			},
			command: "du -sh *",
			wantErr: false,
		},
		{
			name: "Blocked path: git status unaffected",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"git"},
			},
			command: "git status",
			wantErr: false,
		},
		{
			name: "Blocked path: no blocked filenames configured",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"cat"},
			},
			command: "cat .env",
			wantErr: false,
		},
		{
			name: "Blocked path: non-blocked dot dir allowed",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"cat"},
			},
			command: "cat .gitignore",
			wantErr: false,
		},
		{
			name: "Blocked path: npm cache under sandbox blocked",
			config: models.TerminalGuardrailsConfig{
				Enabled:         true,
				AllowedCommands: []string{"ls"},
			},
			command:     "ls .sandbox/node_modules",
			wantErr:     true,
			errContains: "path access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTerminalCommand(tt.command, tt.config, tt.blockedFilenames, nil, "")
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error '%v' does not contain '%s'", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoadTerminalManifest(t *testing.T) {
	var manifest models.TerminalGuardrailsConfig
	err := LoadManifest("terminal", &manifest)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	// Basic checks against terminal.json content
	if len(manifest.AllowedCommands) == 0 {
		t.Error("Manifest loaded with zero allowed commands")
	}
	if len(manifest.BlockedPatterns) == 0 {
		t.Error("Manifest loaded with zero blocked patterns")
	}
	// Shell interpreters must be allowlisted: the smoke-test template runs
	// scripts via `sh`, and node/python/go are already allowlisted arbitrary-
	// code interpreters — excluding sh/bash only blocks legit script
	// execution without adding security.
	for _, cmd := range []string{"sh", "bash", "printf"} {
		if !slices.Contains(manifest.AllowedCommands, cmd) {
			t.Errorf("manifest must allowlist %q", cmd)
		}
	}
	// Read-only inspection utilities used by everyday agent tasks (counting
	// lines/files, sorting, filtering) must be allowlisted — the 2026-08-31
	// assistant run was blocked on `wc` mid-command and stalled 5 minutes in
	// the approval wait for a read-only count.
	for _, cmd := range []string{"wc", "ls", "find", "sort", "grep", "head", "tail", "cat", "du"} {
		if !slices.Contains(manifest.AllowedCommands, cmd) {
			t.Errorf("manifest must allowlist read-only inspection command %q", cmd)
		}
	}
}

// TestValidateTerminalCommand_RunWorkspaceScript verifies the smoke-test's
// Step 4 works: `sh smoke-test-dir/hello.txt` is allowed once sh is on the
// allowlist.
func TestValidateTerminalCommand_RunWorkspaceScript(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{
		Enabled:         true,
		AllowedCommands: []string{"chmod", "sh", "ls"},
	}
	if err := ValidateTerminalCommand("chmod +x smoke-test-dir/hello.txt && sh smoke-test-dir/hello.txt", cfg, nil, nil, ""); err != nil {
		t.Errorf("script execution must validate when sh is allowlisted: %v", err)
	}
	if err := ValidateTerminalCommand("sh smoke-test-dir/hello.txt", cfg, nil, nil, ""); err != nil {
		t.Errorf("`sh smoke-test-dir/hello.txt` must validate: %v", err)
	}
}

// TestValidateTerminalCommand_WcInChain verifies a read-only `wc` piped at the
// end of an inspection chain validates once wc is allowlisted — the exact
// shape that stalled the 2026-08-31 assistant run in a 5-minute approval wait.
func TestValidateTerminalCommand_WcInChain(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{
		Enabled:         true,
		AllowedCommands: []string{"echo", "find", "sort", "wc"},
	}
	cmd := `echo "=== node_modules file count ===" && find ./node_modules -type f | wc -l`
	if err := ValidateTerminalCommand(cmd, cfg, nil, nil, ""); err != nil {
		t.Errorf("read-only wc chain must validate: %v", err)
	}
	// wc still denied when NOT allowlisted — the allowlist stays the gate.
	cfg2 := models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"echo", "find"}}
	if err := ValidateTerminalCommand(cmd, cfg2, nil, nil, ""); err == nil {
		t.Error("wc chain must be rejected when wc is not allowlisted")
	}
}

func TestCheckBlockedPaths_AbsoluteJailPath(t *testing.T) {
	jail := filepath.Join(os.TempDir(), "workspace-health-ws")
	sandboxAbs := filepath.Join(jail, ".sandbox")

	tests := []struct {
		name    string
		command string
		blocked []string
		wantErr bool
	}{
		{"absolute sandbox inside jail", "du -sh " + sandboxAbs, nil, true},
		{"absolute sandbox subpath inside jail", "find " + filepath.Join(sandboxAbs, "tmp") + " -type f", nil, true},
		{"absolute outside jail untouched by block", "du -sh /var/log", nil, false},
		{"absolute sensitive file inside jail", "cat " + filepath.Join(jail, ".env"), []string{".env"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkBlockedPaths(tt.command, tt.blocked, jail)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.command)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.command, err)
			}
		})
	}
}

func TestExtractBaseCommands(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected []string
	}{
		{"single command", "ls -la", []string{"ls"}},
		{"chained with &&", "chmod +x file && sh file", []string{"chmod", "sh"}},
		{"chained with ;", "echo hello; ls", []string{"echo", "ls"}},
		{"chained with |", "cat file | grep foo", []string{"cat", "grep"}},
		{"chained with ||", "make || echo failed", []string{"make", "echo"}},
		{"mixed chain", "mkdir -p dir && echo test > dir/file && sh dir/file", []string{"mkdir", "echo", "sh"}},
		{"find with escaped semicolon", "find . -type f -exec ls -lh {} \\; 2>/dev/null", []string{"find"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBaseCommands(tt.command)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("at index %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestValidateTerminalCommand_AllowedExternalPaths(t *testing.T) {
	tests := []struct {
		name            string
		command         string
		allowedExternal []string
		jailPath        string
		effectiveCwd    string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "absolute path within allowed external is permitted",
			command:         "ls /home/user/projects/other",
			allowedExternal: []string{"/home/user/projects"},
			wantErr:         false,
		},
		{
			name:            "absolute path outside allowed external is blocked",
			command:         "ls /etc/passwd",
			allowedExternal: []string{"/home/user/projects"},
			wantErr:         true,
			errContains:     "outside allowed paths",
		},
		{
			name:            "absolute path blocked when no external paths configured",
			command:         "ls /tmp",
			allowedExternal: nil,
			wantErr:         true,
			errContains:     "absolute paths are not permitted",
		},
		{
			name:            "relative path allowed when no external paths",
			command:         "ls -la",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			wantErr:         false,
		},
		{
			name:            "absolute path within jailPath allowed without external paths",
			command:         "ls /workspace/123/ts-logic-test",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			wantErr:         false,
		},
		{
			name:            "absolute path in chained command within jail allowed",
			command:         "ls /workspace/123/ts-logic-test && cat /workspace/123/ts-logic-test/app.js",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			wantErr:         false,
		},
		{
			name:            "absolute path outside jailPath is blocked",
			command:         "ls /etc/passwd",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			wantErr:         true,
			errContains:     "outside allowed paths",
		},
		{
			name:            ".. path within jail from subdirectory allowed",
			command:         "cat ../tsconfig.json",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			effectiveCwd:    "/workspace/123/ts-logic-test",
			wantErr:         false,
		},
		{
			name:            ".. path within jail from subdirectory in chain allowed",
			command:         "echo ok && cat ../tsconfig.json",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			effectiveCwd:    "/workspace/123/ts-logic-test",
			wantErr:         false,
		},
		{
			name:            ".. path outside jail blocked even with effectiveCwd",
			command:         "cat ../../../../etc/passwd",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			effectiveCwd:    "/workspace/123/ts-logic-test",
			wantErr:         true,
			errContains:     "escapes the authorized workspace jail",
		},
		{
			name:            ".. without effectiveCwd still blocked outside jail",
			command:         "cat ../tsconfig.json",
			allowedExternal: nil,
			jailPath:        "/workspace/123",
			wantErr:         true,
			errContains:     "escapes the authorized workspace jail",
		},
		{
			name:            ".. traversal to allowed external path is permitted",
			command:         "cat ../../../home/user/projects/other/file.txt",
			allowedExternal: []string{"/home/user/projects"},
			jailPath:        "/workspace/123",
			wantErr:         false,
		},
		{
			name:            ".. traversal outside all allowed roots is blocked",
			command:         "cat ../../../etc/passwd",
			allowedExternal: []string{"/home/user/projects"},
			jailPath:        "/workspace/123",
			wantErr:         true,
			errContains:     "escapes the authorized workspace jail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.TerminalGuardrailsConfig{
				Enabled:              true,
				AllowedCommands:      []string{"ls", "cat", "echo"},
				AllowedExternalPaths: tt.allowedExternal,
			}
			err := ValidateTerminalCommand(tt.command, cfg, nil, nil, tt.jailPath, tt.effectiveCwd)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error '%v' does not contain '%s'", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestResolveCwd_NonExistentCwdFallsBack(t *testing.T) {
	tmpDir := t.TempDir()

	tt := &TerminalTools{}

	t.Run("nonexistent cwd falls back to jailPath", func(t *testing.T) {
		result, err := tt.resolveCwd("nonexistent-subdir", tmpDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != tmpDir {
			t.Errorf("expected fallback to jailPath %q, got %q", tmpDir, result)
		}
	})

	t.Run("existing cwd resolves correctly", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "existing-dir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		result, err := tt.resolveCwd("existing-dir", tmpDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected, _ := filepath.EvalSymlinks(subDir)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("empty cwd returns jailPath", func(t *testing.T) {
		result, err := tt.resolveCwd("", tmpDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != tmpDir {
			t.Errorf("expected jailPath %q, got %q", tmpDir, result)
		}
	})

	t.Run("empty jailPath returns empty", func(t *testing.T) {
		result, err := tt.resolveCwd("some-dir", "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("cwd escaping jailPath is rejected", func(t *testing.T) {
		_, err := tt.resolveCwd("../../etc", tmpDir, nil)
		if err == nil {
			t.Fatal("expected security error but got nil")
		}
		if !strings.Contains(err.Error(), "security violation") {
			t.Errorf("expected security violation error, got: %v", err)
		}
	})
}

func TestHasExternalAccess(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{}
	if cfg.HasExternalAccess() {
		t.Error("expected HasExternalAccess to be false for empty config")
	}
	cfg.AllowedExternalPaths = []string{"/home/user/projects"}
	if !cfg.HasExternalAccess() {
		t.Error("expected HasExternalAccess to be true with external paths configured")
	}
}

func TestAssertAllowedCommand(t *testing.T) {
	tests := []struct {
		name     string
		segments []string
		allowed  []string
		wantErr  bool
	}{
		{name: "allowed command", segments: []string{"ls -la"}, allowed: []string{"ls"}, wantErr: false},
		{name: "disallowed command", segments: []string{"rm -rf /"}, allowed: []string{"ls"}, wantErr: true},
		{name: "empty segment", segments: []string{""}, allowed: []string{"ls"}, wantErr: false},
		{name: "shell comment", segments: []string{"# Step 1: list directory"}, allowed: []string{"ls"}, wantErr: false},
		{name: "comment before command", segments: []string{"# Step 6\nls -la"}, allowed: []string{"ls"}, wantErr: false},
		{name: "comment then disallowed", segments: []string{"# comment\nrm -rf"}, allowed: []string{"ls"}, wantErr: true},
		{name: "mixed chain", segments: []string{"ls -la", "echo hello"}, allowed: []string{"ls", "echo"}, wantErr: false},
		{name: "mixed chain one bad", segments: []string{"ls -la", "rm -rf"}, allowed: []string{"ls"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := assertAllowedCommand(tt.segments, tt.allowed)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMergeWithAllowedExternalPaths(t *testing.T) {
	base := &models.AgentGuardrailsConfig{
		Terminal: models.TerminalGuardrailsConfig{
			AllowedExternalPaths: []string{"/existing/path"},
		},
	}
	override := &models.AgentGuardrailsConfig{
		Terminal: models.TerminalGuardrailsConfig{
			AllowedExternalPaths: []string{"/new/path", "/existing/path"},
		},
	}
	base.MergeWith(override)
	if len(base.Terminal.AllowedExternalPaths) != 2 {
		t.Errorf("expected 2 merged paths, got %d: %v", len(base.Terminal.AllowedExternalPaths), base.Terminal.AllowedExternalPaths)
	}
	found := make(map[string]bool)
	for _, p := range base.Terminal.AllowedExternalPaths {
		found[p] = true
	}
	if !found["/existing/path"] || !found["/new/path"] {
		t.Errorf("expected both paths after merge, got %v", base.Terminal.AllowedExternalPaths)
	}
}

func TestNewCommand_ProcessGroupIsolation(t *testing.T) {
	tt := &TerminalTools{}
	tmpDir := t.TempDir()

	cmd := tt.newCommand(context.Background(), "bash", "sleep 60", tmpDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start failed: %v", err)
	}
	defer cmd.Wait()

	if cmd.Process == nil {
		t.Fatal("Process not started")
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid failed: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Errorf("Setpgid not isolated: pid=%d pgid=%d (expected pgid==pid for new group)", cmd.Process.Pid, pgid)
	}

	// Clean shutdown — kill the process group so test doesn't leak
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Wait()
}

func TestNewCommand_KillsOnContextCancel(t *testing.T) {
	tt := &TerminalTools{}
	tmpDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := tt.newCommand(ctx, "bash", "sleep 60", tmpDir)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("cmd.Start failed: %v", err)
	}

	// Kill process group on cancel (matching executeLocal behaviour)
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected non-nil error after cancellation and SIGKILL")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cmd.Wait did not return within 2s of SIGKILL — process not killed")
	}
}

func TestTerminalTools_ShellPGID_NoShellPool(t *testing.T) {
	tt := &TerminalTools{}
	pgid, err := tt.ShellPGID(context.Background(), "any-workspace")
	if err == nil {
		t.Error("expected error when no shell pool configured")
	}
	if !errors.Is(err, ErrShellPoolNotAvailable) {
		t.Errorf("expected ErrShellPoolNotAvailable, got %v", err)
	}
	if pgid != 0 {
		t.Errorf("expected 0 pgid, got %d", pgid)
	}
}

func TestCollapseWhitespacePreserveNewlines(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "single line collapsed",
			command: "ls   -la   .",
			want:    "ls -la .",
		},
		{
			name:    "multi-line preserved",
			command: "uname -a\ndate -u +%Y-%m-%dT%H:%M:%SZ\necho \"terminal-tool-works\"",
			want:    "uname -a\ndate -u +%Y-%m-%dT%H:%M:%SZ\necho \"terminal-tool-works\"",
		},
		{
			name:    "multi-line with extra spaces per line",
			command: "echo   hi\n   uname   -a   ",
			want:    "echo hi\nuname -a",
		},
		{
			name:    "interior blank lines kept (heredoc body)",
			command: "cat <<EOF\nline1\n\nline3\nEOF",
			want:    "cat <<EOF\nline1\n\nline3\nEOF",
		},
		{
			name:    "leading and trailing blank lines dropped",
			command: "\n\nls -la\n\n",
			want:    "ls -la",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseWhitespacePreserveNewlines(tt.command)
			if got != tt.want {
				t.Errorf("collapseWhitespacePreserveNewlines(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSplitCommandSegments_MultiLine(t *testing.T) {
	// A newline is a shell command separator: each line becomes its own
	// segment so the whitelist is enforced per command, not per blob.
	got := SplitCommandSegments("uname -a\ndate -u +%Y-%m-%dT%H:%M:%SZ\necho \"terminal-tool-works\"")
	want := []string{"uname -a", "date -u +%Y-%m-%dT%H:%M:%SZ", `echo "terminal-tool-works"`}
	if len(got) != len(want) {
		t.Fatalf("expected %d segments %v, got %d %v", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSplitCommandSegments_HeredocStaysOneSegment(t *testing.T) {
	got := SplitCommandSegments("cat <<EOF\nhello\nEOF")
	if len(got) != 1 {
		t.Fatalf("heredoc should stay a single segment, got %d: %v", len(got), got)
	}
	if got[0] != "cat <<EOF\nhello\nEOF" {
		t.Errorf("unexpected heredoc segment %q", got[0])
	}
}

// TestSplitCommandSegments_HeredocMarkerSyntaxes verifies every heredoc marker
// form terminates the heredoc so commands after it are scanned as separate
// segments (a non-terminated heredoc would swallow them and bypass the
// whitelist).
func TestSplitCommandSegments_HeredocMarkerSyntaxes(t *testing.T) {
	cmds := []string{
		"cat <<EOF\nbody\nEOF\nrm -rf /",
		"cat <<-EOF\n\tbody\nEOF\nrm -rf /",
		"cat <<'EOF'\nbody\nEOF\nrm -rf /",
		"cat <<\"EOF\"\nbody\nEOF\nrm -rf /",
		"cat <<\\EOF\nbody\nEOF\nrm -rf /",
	}
	for _, c := range cmds {
		got := SplitCommandSegments(c)
		if len(got) != 2 {
			t.Errorf("expected 2 segments for %q, got %d: %v", c, len(got), got)
			continue
		}
		if got[1] != "rm -rf /" {
			t.Errorf("expected trailing command split out for %q, got %q", c, got[1])
		}
	}
}

// TestSplitCommandSegments_HereStringNotHeredoc verifies "<<<" (here-string)
// does not open heredoc mode, so a chained command after it is still split.
func TestSplitCommandSegments_HereStringNotHeredoc(t *testing.T) {
	got := SplitCommandSegments("cat <<<\"hello\" && rm -rf /")
	if len(got) != 2 || got[1] != "rm -rf /" {
		t.Fatalf("expected here-string not to swallow the chain, got %v", got)
	}
}

// TestScanCommandSegments_Balance verifies unterminated quotes and heredocs
// are reported as unbalanced (fail-closed for the whitelist).
func TestScanCommandSegments_Balance(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		balanced bool
	}{
		{"plain", "ls -la", true},
		{"balanced quotes", "echo 'a b' && echo \"c d\"", true},
		{"unclosed single quote", "echo 'oops", false},
		{"unclosed double quote", "echo \"oops", false},
		{"unterminated heredoc", "cat <<EOF\nbody", false},
		{"terminated heredoc", "cat <<EOF\nbody\nEOF", true},
		{"here-string", "cat <<<\"x\" && ls", true},
		{"backslash escaped", "find . -exec ls {} \\;", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, balanced := scanCommandSegments(tt.command)
			if balanced != tt.balanced {
				t.Errorf("scanCommandSegments(%q) balanced = %v, want %v", tt.command, balanced, tt.balanced)
			}
		})
	}
}

// TestValidateTerminalCommand_HeredocBypassClosed verifies the whitelist
// bypass via non-terminated heredocs is closed: disallowed commands after any
// heredoc marker form (or a here-string) are blocked, and unterminated
// heredocs/quotes are rejected outright.
func TestValidateTerminalCommand_HeredocBypassClosed(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{
		Enabled:         true,
		AllowedCommands: []string{"cat"},
	}
	cmds := []string{
		"cat <<EOF\nbody\nEOF\nrm -rf /",
		"cat <<-EOF\n\tbody\nEOF\nrm -rf /",
		"cat <<<\"x\" && rm -rf /",
		"cat <<EOF\nbody",           // unterminated heredoc itself
		"cat <<EOF\nbody\nrm -rf /", // missing terminator: rm swallowed before
		"echo \"oops",               // unterminated quote
	}
	for _, c := range cmds {
		if err := ValidateTerminalCommand(c, cfg, nil, nil, ""); err == nil {
			t.Errorf("expected validation error for %q", c)
		}
	}
}

// TestValidateTerminalCommand_LegitHeredocAllowed verifies well-formed
// heredocs still pass validation when every command is allowlisted.
func TestValidateTerminalCommand_LegitHeredocAllowed(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{
		Enabled:         true,
		AllowedCommands: []string{"cat", "echo"},
	}
	cmds := []string{
		"cat <<EOF\nline one\n  indented line\nEOF",
		"cat <<-EOF\n\tbody\nEOF\necho done",
		"cat <<'EOF'\nbody\nEOF",
	}
	for _, c := range cmds {
		if err := ValidateTerminalCommand(c, cfg, nil, nil, ""); err != nil {
			t.Errorf("expected %q to validate, got %v", c, err)
		}
	}
}

// TestSanitizeCommandPreservesHeredocBody verifies sanitizeCommand no longer
// whitespace-mangles heredoc bodies or string literals (they are content).
func TestSanitizeCommandPreservesHeredocBody(t *testing.T) {
	tools := &TerminalTools{}
	cmd := "cat <<EOF\n  indented line\nline with  double spaces\nEOF"
	got, err := tools.sanitizeCommand(context.Background(), cmd, models.TerminalGuardrailsConfig{}, "")
	if err != nil {
		t.Fatalf("sanitizeCommand: %v", err)
	}
	if got != cmd {
		t.Errorf("heredoc body must reach the shell verbatim:\n got %q\nwant %q", got, cmd)
	}
}

func TestValidateTerminalCommand_MultiLineWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "all lines allowlisted",
			command: "uname -a\ndate -u +%Y-%m-%dT%H:%M:%SZ\necho \"terminal-tool-works\"",
			wantErr: false,
		},
		{
			name:    "second line disallowed command is blocked",
			command: "ls -la\nrm -rf /",
			wantErr: true,
		},
		{
			name:    "disallowed command merged via && still blocked",
			command: "ls -la && rm -rf /",
			wantErr: true,
		},
	}
	cfg := models.TerminalGuardrailsConfig{
		Enabled:         true,
		AllowedCommands: []string{"ls", "uname", "date", "echo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTerminalCommand(tt.command, cfg, nil, nil, "")
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.command)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.command, err)
			}
		})
	}
}
