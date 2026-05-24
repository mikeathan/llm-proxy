package tools

import (
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalTools_Guardrails(t *testing.T) {
	tests := []struct {
		name        string
		config      models.TerminalGuardrailsConfig
		command     string
		wantErr     bool
		errContains string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTerminalCommand(tt.command, tt.config, nil, "")
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
	err := LoadManifest("", "terminal", &manifest)
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
		name             string
		command          string
		allowedExternal  []string
		jailPath         string
		effectiveCwd     string
		wantErr          bool
		errContains      string
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
			err := ValidateTerminalCommand(tt.command, cfg, nil, tt.jailPath, tt.effectiveCwd)
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
