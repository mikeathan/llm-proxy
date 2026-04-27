package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// TaskExecutor executes automations.
type TaskExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
}

type ExecuteRequest struct {
	WorkspaceID    string
	AutomationName string
	TaskFile       string
	TaskContent    string
	Strategy       ExecutionStrategy
	State          *models.AgentState
	Model          string // Optional model override
}

type ExecuteResponse struct {
	Output string
	Error  error
	State  *models.AgentState
}

// LLMServiceProvider is the minimal interface needed by LLMTaskExecutor.
// It avoids an import cycle by not depending on the api or nodeherder packages.
type LLMServiceProvider interface {
	ClientProvider() proxy.LLMClientProvider
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	Logger() logging.Logger
	ToolProvider() assistant.ToolProvider
	Engine() assistant.Engine
	GuardrailEngine() *guardrails.GuardrailEngine
	ProcessLogger(workspaceID string) logging.Logger
	Persistence() *persistence.WorkspaceManager
	Events() *EventBus
}

// DefaultTaskExecutor is a placeholder that marks execution as running.
// Kept for cases where LLM execution is not required.
type DefaultTaskExecutor struct{}

func NewDefaultTaskExecutor() *DefaultTaskExecutor {
	return &DefaultTaskExecutor{}
}

func (e *DefaultTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	resp.State.SetRunning(req.AutomationName)
	resp.Output = "dispatcher placeholder output (Phase 3: wire LLM)"

	return resp, nil
}

// LLMTaskExecutor is a TaskExecutor that uses an LLM client for execution.
type LLMTaskExecutor struct {
	svc LLMServiceProvider
}

func NewLLMTaskExecutor(svc LLMServiceProvider) TaskExecutor {
	return &LLMTaskExecutor{svc: svc}
}

func (e *LLMTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	startTime := time.Now()
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	// Mark as running
	resp.State.SetRunning(req.AutomationName)

	// Get LLM client
	var client proxy.Client
	var err error
	if req.Model != "" {
		client, err = e.svc.GetClientForModel(ctx, req.Model)
	} else {
		clientProvider := e.svc.ClientProvider()
		client, err = clientProvider.GetClient(ctx)
	}

	if err != nil {
		errStr := fmt.Sprintf("failed to get llm client: %v", err)
		resp.State.LastError = errStr
		resp.State.SetRunning("")
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime), nil)
		return resp, fmt.Errorf("failed to get llm client: %w", err)
	}

	// Initialize the unified Agent
	procLog := e.svc.ProcessLogger(req.WorkspaceID)
	procLog.Info("Automation execution started", "workspace", req.WorkspaceID, "automation", req.AutomationName)

	var capturedEvents []any
	agent := assistant.NewAgent(client, e.svc.ToolProvider(), e.svc.Engine(), assistant.AgentOptions{
		Logger:      procLog,
		MaxSteps:    20,
		Guardrails:  e.svc.GuardrailEngine(),
		WorkspaceID: req.WorkspaceID,
		Observer: func(ev assistant.AgentEvent) {
			capturedEvents = append(capturedEvents, ev)
			e.svc.Events().Publish(req.WorkspaceID, ev)
		},
	})

	// Build task prompt from automation info
	prompt := e.buildPrompt(req)

	history := []proxy.Message{
		{Role: "user", Content: prompt},
	}

	// Execute via Agent Loop
	finalReply, fullHistory, agErr := agent.Execute(ctx, history)
	if agErr != nil {
		errStr := fmt.Sprintf("agent execution failed: %v", agErr)
		resp.State.LastError = errStr
		resp.State.SetRunning("")
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime), capturedEvents)
		return resp, fmt.Errorf("agent execution failed: %w", agErr)
	}

	var runResult = finalReply
	var runError string

	// Collect any tool calls for result summary if no final reply content (rare but possible)
	if finalReply == "" && len(fullHistory) > 1 {
		toolCount := 0
		for _, msg := range fullHistory {
			toolCount += len(msg.ToolCalls)
		}
		if toolCount > 0 {
			runResult = fmt.Sprintf("Agent executed %d tool call(s) but returned no final summary.", toolCount)
		} else {
			runResult = "Agent completed the task but returned an empty response."
		}
	}

	// Prepare formatted output
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	header := fmt.Sprintf("[%s] ▶ Executing automation `%s` in workspace `%s`...\nReading task file: `%s`\n\n",
		timestamp, req.AutomationName, req.WorkspaceID, req.TaskFile)

	output := strings.TrimSpace(runResult)
	
	// Smart Unwrapper: If the model wrapped the entire response in a markdown code block, 
	// strip it so our UI can actually render the markdown structure (headers, tables).
	if strings.HasPrefix(output, "```") && strings.HasSuffix(output, "```") {
		lines := strings.Split(output, "\n")
		if len(lines) >= 2 {
			// Remove first and last lines (the backticks)
			output = strings.Join(lines[1:len(lines)-1], "\n")
			output = strings.TrimSpace(output)
		}
	}

	fullOutput := fmt.Sprintf("%s### Final Report\n\n%s", header, output)

	resp.Output = fullOutput
	resp.State.LastOutput = fullOutput
	runResult = fullOutput

	resp.State.SetRunning("")

	// Push a concluding message to the UI stream to clear "thinking..."
	e.svc.Events().Publish(req.WorkspaceID, assistant.AgentEvent{
		Type: assistant.EventMessage,
		Payload: proxy.Message{
			Role:    "system",
			Content: "✔ Execution complete.",
		},
	})

	// Record to history
	e.recordRun(req, resp.State, runResult, runError, time.Since(startTime), capturedEvents)

	return resp, nil
}

func (e *LLMTaskExecutor) recordRun(req ExecuteRequest, state *models.AgentState, output, errStr string, duration time.Duration, events []any) {
	if state == nil {
		return
	}

	run := models.AutomationRun{
		ID:             generateRunID(),
		WorkspaceID:    req.WorkspaceID,
		AutomationName: req.AutomationName,
		Timestamp:      time.Now(),
		Output:         output,
		Error:          errStr,
		DurationMs:     duration.Milliseconds(),
		Model:          req.Model,
		Events:         events,
	}

	// Add to full history (capped to last 50 for performance)
	state.History = append(state.History, run)
	if len(state.History) > 50 {
		state.History = state.History[len(state.History)-50:]
	}

	// Update per-automation latest run
	if state.LastRuns == nil {
		state.LastRuns = make(map[string]*models.AutomationRun)
	}
	state.LastRuns[req.AutomationName] = &run
}

func generateRunID() string {
	// Simple unique-ish ID without importing full UUID package if not strictly needed
	// but I checked go.mod and it's there.
	// For now, use timestamp + nano for simplicity if I want to avoid adding the import
	// but let's just use UUID if I can.
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

func (e *LLMTaskExecutor) buildPrompt(req ExecuteRequest) string {
	procLog := e.svc.ProcessLogger(req.WorkspaceID)

	// Try to load custom workspace rules if they exist
	rules, err := e.svc.Persistence().ReadTaskFile(req.WorkspaceID, "rules.md")
	if err != nil || rules == "" {
		rules = prompts.DefaultRules
	}

	// Calculate robust relative path from Current Working Directory to Workspaces Dir
	// This ensures the agent uses paths that the backend can resolve from its current execution root.
	absWs := e.svc.Persistence().BaseDir()
	cwd, _ := os.Getwd()
	relWs, err := filepath.Rel(cwd, absWs)
	if err != nil {
		relWs = absWs
	}
	relWs = filepath.Clean(relWs)

	procLog.Debug("Automation path resolution", "abs_ws", absWs, "cwd", cwd, "rel_ws", relWs)

	// Safely format rules to avoid %!(EXTRA) errors if the file doesn't contain verbs
	formattedRules := rules
	formattedRules = strings.ReplaceAll(formattedRules, "{{REL_WS}}", relWs)
	formattedRules = strings.ReplaceAll(formattedRules, "{{WORKSPACE_ID}}", req.WorkspaceID)

	return fmt.Sprintf(prompts.AutomationTaskPrompt,
		formattedRules,
		req.WorkspaceID, relWs, req.WorkspaceID, req.TaskFile, req.TaskContent)
}

// ============================================================================
// Pulse Logic (Smart Skip)
// ============================================================================

const heartbeatOKMarker = "HEARTBEAT_OK"

func isHeartbeatOK(output string) bool {
	return strings.Contains(output, heartbeatOKMarker)
}

// ApplyPulseLogic applies the Smart Skip logic to suppress noisy heartbeat output.
func ApplyPulseLogic(resp *ExecuteResponse) {
	if isHeartbeatOK(resp.Output) {
		resp.Output = ""
		resp.State.LastPulse = time.Now()
	}
}
