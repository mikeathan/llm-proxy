package tools

import (
	"context"
	"fmt"
	"llm-proxy/models"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// TerminalTools provides tools for executing local shell commands.
type TerminalTools struct {
	configProvider func() models.TerminalGuardrailsConfig
}

func NewTerminalTools(provider func() models.TerminalGuardrailsConfig) *TerminalTools {
	return &TerminalTools{configProvider: provider}
}

// Validate checks if a command is allowed based on the provided configuration.
func (t *TerminalTools) Validate(command string) error {
	return ValidateTerminalCommand(command, t.configProvider())
}

// ValidateTerminalCommand is a standalone validator that checks a command against guardrails.
func ValidateTerminalCommand(command string, cfg models.TerminalGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("terminal tools are disabled in configuration")
	}

	cleanCmd := strings.TrimSpace(command)
	if cleanCmd == "" {
		return fmt.Errorf("empty command")
	}

	// 1. Check Blocked Patterns FIRST
	for _, pattern := range cfg.BlockedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue 
		}
		if re.MatchString(cleanCmd) {
			return fmt.Errorf("command contains blocked pattern: %s", pattern)
		}
	}

	// 2. Check Whitelist
	if len(cfg.AllowedCommands) > 0 {
		baseCmd := strings.Split(cleanCmd, " ")[0]
		allowed := false
		for _, a := range cfg.AllowedCommands {
			if a == baseCmd {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command '%s' is not in the allowed whitelist", baseCmd)
		}
	}

	return nil
}

func (t *TerminalTools) ExecuteCommand(ctx context.Context, command string) (string, error) {
	cfg := t.configProvider()
	if err := t.Validate(command); err != nil {
		return "", err
	}

	// Apply hard timeout from manifest/config
	execCtx := ctx
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	
	result := string(out)
	
	// Apply character limit to output
	if cfg.MaxOutputSize > 0 && len(result) > cfg.MaxOutputSize {
		result = result[:cfg.MaxOutputSize] + "\n... (output truncated by guardrails)"
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %ds", cfg.TimeoutSeconds)
		}
		return result, fmt.Errorf("execution failed: %w", err)
	}

	return result, nil
}
