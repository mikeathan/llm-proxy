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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create tool with a provider that returns the test config
			term := NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
				return tt.config
			})

			ctx := context.Background()

			// For allowed commands that we don't actually want to run on the test machine,
			// we skip the actual execution if it's not an error case.
			// But since 'ls' is safe, we let it run check if it's expected to pass.

			res, err := term.ExecuteCommand(ctx, tt.command)

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
