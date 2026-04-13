package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
	"regexp"
	"strings"
)

// GuardrailEngine evaluates tool calls against configured boundaries.
type GuardrailEngine struct {
	configProvider func() models.AgentGuardrailsConfig
}

func NewGuardrailEngine(provider func() models.AgentGuardrailsConfig) *GuardrailEngine {
	return &GuardrailEngine{configProvider: provider}
}

// ValidateToolCall checks a tool call against global and category-specific safety rules.
func (e *GuardrailEngine) ValidateToolCall(ctx context.Context, call proxy.ToolCall) error {
	cfg := e.configProvider()

	// 1. Global Guardrails (Sensitive Data)
	if err := e.validateGlobal(call, cfg.Global); err != nil {
		return err
	}

	// 2. Category-Specific Guardrails
	switch call.Function.Name {
	case "execute_terminal_command":
		return e.validateTerminal(call, cfg.Terminal)
	case "internet_search":
		return e.validateSearch(call, cfg.Search)
	case "notify_user":
		return e.validateCommunication(call, cfg.Communication)
	case "list_directory", "read_file", "write_file":
		return e.validateFileSystem(call, cfg.FileSystem)
	}

	return nil
}

func (e *GuardrailEngine) validateGlobal(call proxy.ToolCall, cfg models.GlobalGuardrailsConfig) error {
	if cfg.BlockSecrets {
		// regex to find common API key formats (sk-..., AKIA..., etc)
		secretPatterns := []string{
			`sk-[a-zA-Z0-9]{32,}`,
			`AKIA[a-zA-Z0-9]{16}`,
			`AIza[a-zA-Z0-9_-]{35}`,
		}
		for _, p := range secretPatterns {
			re := regexp.MustCompile(p)
			if re.MatchString(call.Function.Arguments) {
				return fmt.Errorf("guardrail violation: sensitive data detected in tool arguments")
			}
		}
	}
	return nil
}

func (e *GuardrailEngine) validateTerminal(call proxy.ToolCall, cfg models.TerminalGuardrailsConfig) error {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	return tools.ValidateTerminalCommand(args.Command, cfg)
}

func (e *GuardrailEngine) validateSearch(call proxy.ToolCall, cfg models.SearchGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("internet search is disabled by guardrails policy")
	}

	if cfg.MaxQueryLen > 0 && len(call.Function.Arguments) > cfg.MaxQueryLen {
		return fmt.Errorf("search query too long (max %d characters)", cfg.MaxQueryLen)
	}

	// Check for blocked domains in arguments
	for _, site := range cfg.BlockedSites {
		if strings.Contains(call.Function.Arguments, site) {
			return fmt.Errorf("guardrail violation: search query contains blocked domain '%s'", site)
		}
	}
	return nil
}

func (e *GuardrailEngine) validateCommunication(call proxy.ToolCall, cfg models.CommunicationGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("communication tools are disabled by guardrails policy")
	}
	
	if cfg.RequireReview {
		return fmt.Errorf("guardrail check: manual approval required for this communication")
	}
	return nil
}

func (e *GuardrailEngine) validateFileSystem(call proxy.ToolCall, cfg models.FileSystemGuardrailsConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("filesystem tools are disabled by guardrails policy")
	}

	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse path: %w", err)
	}

	_, err := tools.IsSecurePath(args.Path, cfg.AllowedPaths)
	return err
}
