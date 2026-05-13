package tools

import (
	"context"
	"llm-proxy/models"
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
			// Create tool with a provider that returns the test config
			term := NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
				return tt.config
			}, nil)

			ctx := context.Background()

			// For allowed commands that we don't actually want to run on the test machine,
			// we skip the actual execution if it's not an error case.
			// But since 'ls' is safe, we let it run check if it's expected to pass.

			res, err := term.ExecuteCommand(ctx, tt.command, "")

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
				if res == "" && tt.command == "ls" {
					// result might be empty in some test envs, but should not error
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

func TestSanitizeCommandSkipsValidationWhenGuardrailApproved(t *testing.T) {
	tt := &TerminalTools{
		configProvider: func(ctx context.Context) models.TerminalGuardrailsConfig {
			return models.TerminalGuardrailsConfig{
				Enabled:        true,
				AllowedCommands: []string{"echo"},
			}
		},
	}

	// Without the guardrail-approved marker, "ls" should be rejected.
	ctx := context.Background()
	_, err := tt.sanitizeCommand(ctx, "ls", tt.configProvider(ctx), "")
	if err == nil {
		t.Error("expected validation to reject 'ls' without approval marker")
	}

	// With the guardrail-approved marker, the check is skipped and command passes.
	ctx = models.WithGuardrailApproved(context.Background())
	clean, err := tt.sanitizeCommand(ctx, "ls", tt.configProvider(ctx), "")
	if err != nil {
		t.Errorf("expected validation to skip with approval marker, got: %v", err)
	}
	if clean != "ls" {
		t.Errorf("expected clean command 'ls', got %q", clean)
	}
}
