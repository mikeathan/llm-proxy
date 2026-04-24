package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/sandbox"
	"llm-proxy/models"
)

// TerminalTools provides tools for executing local shell commands.
type TerminalTools struct {
	configProvider func(ctx context.Context) models.TerminalGuardrailsConfig
	pathResolver   func(workspaceID string) string
	sandboxPool    SandboxProvider
	observer       StreamObserver
	regexCache     sync.Map 
}

type SandboxProvider interface {
	GetOrCreate(ctx context.Context, workspaceID string, hostPath string) (sandbox.Sandbox, error)
	Recycle(ctx context.Context, workspaceID string)
	ListSessions() []models.SandboxSessionView
	Shutdown()
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

// SetSandboxProvider injects the Orchestrator for secure execution
func (t *TerminalTools) SetSandboxProvider(pool SandboxProvider, observer StreamObserver) {
	t.sandboxPool = pool
	t.observer = observer
}

func (t *TerminalTools) Config(ctx context.Context) models.TerminalGuardrailsConfig {
	return t.configProvider(ctx)
}

// Validate checks if a command is allowed based on the provided configuration.
func (t *TerminalTools) Validate(ctx context.Context, command string) error {
	return ValidateTerminalCommand(command, t.configProvider(ctx), &t.regexCache)
}

// ValidateTerminalCommand is a standalone-like validator that checks a command against guardrails.
func ValidateTerminalCommand(command string, cfg models.TerminalGuardrailsConfig, cache *sync.Map) error {
	if !cfg.Enabled {
		return fmt.Errorf("terminal tools are disabled in configuration")
	}

	// Normalize whitespace: trim and collapse internal spaces to prevent bypasses like "rm  -rf"
	cleanCmd := strings.Join(strings.Fields(command), " ")
	if cleanCmd == "" {
		return fmt.Errorf("empty command")
	}

	// 1. Check Blocked Patterns FIRST
	for _, pattern := range cfg.BlockedPatterns {
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

	// 3. Block Absolute Paths (Jail Escape Prevention)
	// We block any argument starting with "/" or containing " /" to prevent
	// the agent from accessing files outside the workspace via host binaries.
	if strings.Contains(cleanCmd, " /") || strings.HasPrefix(cleanCmd, "/") {
		return fmt.Errorf("security violation: absolute paths are not permitted in terminal commands")
	}

	return nil
}

func (t *TerminalTools) ExecuteCommand(ctx context.Context, command string) (string, error) {
	cfg := t.configProvider(ctx)
	if err := t.Validate(ctx, command); err != nil {
		return "", err
	}

	// Apply hard timeout from manifest/config
	execCtx := ctx
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	if t.sandboxPool != nil {
		return t.executeSandboxed(execCtx, command, cfg)
	}

	return t.executeLocal(execCtx, command, cfg)
}

func (t *TerminalTools) executeSandboxed(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig) (string, error) {
	wsID := models.GetWorkspaceID(ctx)
	hostPath := ""
	if t.pathResolver != nil {
		hostPath = t.pathResolver(wsID)
	}

	sb, err := t.sandboxPool.GetOrCreate(ctx, wsID, hostPath)
	if err != nil {
		return "", fmt.Errorf("failed to get/create sandbox for workspace %s: %w", wsID, err)
	}

	// Since we are running in sh, we pass the command
	cmdArgs := []string{"sh", "-c", command}
	outStream, errStream, err := sb.Execute(ctx, cmdArgs)
	if err != nil {
		return "", fmt.Errorf("sandbox execution failed: %w", err)
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
	result := buf.String()

	// Apply character limit to output
	if cfg.MaxOutputSize > 0 && len(result) > cfg.MaxOutputSize {
		result = result[:cfg.MaxOutputSize] + "\n... (output truncated by guardrails)"
	}

	return result, nil
}

func (t *TerminalTools) executeLocal(ctx context.Context, command string, cfg models.TerminalGuardrailsConfig) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	
	result := string(out)
	
	if cfg.MaxOutputSize > 0 && len(result) > cfg.MaxOutputSize {
		result = result[:cfg.MaxOutputSize] + "\n... (output truncated by guardrails)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %ds", cfg.TimeoutSeconds)
		}
		return result, fmt.Errorf("execution failed: %w", err)
	}

	return result, nil
}
