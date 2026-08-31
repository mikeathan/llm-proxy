package assistant

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// newSlowGuardrail returns a GuardrailEngine whose configProvider blocks for
// delay before returning. When blockSecrets is true and the tool args contain a
// secret, ValidateToolCall returns a violation after the delay — used to drive
// the timeout path (the delay exceeds GuardrailTimeout).
func newSlowGuardrail(delay time.Duration, blockSecrets bool) *guardrails.GuardrailEngine {
	return guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		if delay > 0 {
			time.Sleep(delay)
		}
		return models.AgentGuardrailsConfig{
			Global: models.GlobalGuardrailsConfig{BlockSecrets: blockSecrets},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)
}

func secretToolCall() proxy.ToolCall {
	return proxy.ToolCall{
		ID:       "call_secret",
		Type:     "function",
		Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"x","content":"sk-12345678901234567890123456789012"}`},
	}
}

func TestAppendToolResult_ReusesMarshaledContent(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	history := []proxy.Message{{Role: proxy.UserRole, Content: "x"}}
	tc := proxy.ToolCall{ID: "c1", Function: proxy.FunctionCall{Name: "test_tool"}}
	big := strings.Repeat("y", 20000)

	got := agent.appendToolResult(&history, tc, map[string]string{"big": big})

	last := history[len(history)-1]
	if last.Content != got {
		t.Error("returned content must equal the stored history content (no second marshal)")
	}
	if len(last.Content) > 9000 {
		t.Errorf("expected truncation (~8KB cap), got len %d", len(last.Content))
	}
}

func TestGuardrailTimeout_FailOpen(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         30 * time.Millisecond,
			GuardrailTimeoutBehavior: "fail-open",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(200*time.Millisecond, true),
			Logger:     logging.NewNopLogger(),
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if stopBatch {
		t.Error("fail-open: expected tool to proceed (stopBatch=false)")
	}
	if execErr != nil {
		t.Errorf("fail-open: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 1 {
		t.Errorf("fail-open: expected tool to execute, got %d calls", engine.Calls)
	}
}

func TestGuardrailTimeout_FailClosed(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         30 * time.Millisecond,
			GuardrailTimeoutBehavior: "fail-closed",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(200*time.Millisecond, true),
			Logger:     logging.NewNopLogger(),
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if !stopBatch {
		t.Error("fail-closed: expected tool to be denied (stopBatch=true)")
	}
	if execErr != nil {
		t.Errorf("fail-closed: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 0 {
		t.Errorf("fail-closed: expected tool NOT to execute, got %d calls", engine.Calls)
	}
}

func TestGuardrailTimeout_WithinLimit(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         5 * time.Second, // far exceeds the fast eval
			GuardrailTimeoutBehavior: "fail-open",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(0, false), // fast, no violation
			Logger:     logging.NewNopLogger(),
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	tc := proxy.ToolCall{
		ID:       "call_ok",
		Type:     "function",
		Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{}`},
	}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), tc, &history, &mu)

	if stopBatch {
		t.Error("within-limit: expected tool to proceed")
	}
	if execErr != nil {
		t.Errorf("within-limit: expected no error, got %v", execErr)
	}
	if engine.Calls != 1 {
		t.Errorf("within-limit: expected tool to execute, got %d calls", engine.Calls)
	}
}

func TestGuardrailTimeout_NormalViolationStillDenied(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         5 * time.Second, // no deadline, real violation
			GuardrailTimeoutBehavior: "fail-open",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(0, true), // fast, secret triggers violation
			Logger:     logging.NewNopLogger(),
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if !stopBatch {
		t.Error("normal violation: expected tool to be denied (stopBatch=true)")
	}
	if execErr != nil {
		t.Errorf("normal violation: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 0 {
		t.Errorf("normal violation: expected tool NOT to execute, got %d calls", engine.Calls)
	}
}

// TestGuardrailApprovalWait_TimeoutDenies proves the human approval wait is
// bounded (Constitution II.10 / SPEC guardrails) and honors the per-model
// GuardrailApprovalTimeout: when no decision arrives before the bound, the call
// is treated as denied, the violation is recorded, and the run continues — it
// cannot stall indefinitely.
func TestGuardrailApprovalWait_TimeoutDenies(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	store := NewGuardrailDecisionStore()
	var events []AgentEvent
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         5 * time.Second, // validation bound; distinct from the approval bound
			GuardrailTimeoutBehavior: "fail-closed",
			GuardrailApprovalTimeout: 40 * time.Millisecond,
		},
		deps: AgentRuntimeDeps{
			Engine:      engine,
			Guardrails:  newSlowGuardrail(0, true), // fast eval, secret triggers violation
			Logger:      logging.NewNopLogger(),
			OnGuardrail: NewGuardrailDecisionCallback(store, func(ev AgentEvent) { events = append(events, ev) }, ChannelAutomation),
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	start := time.Now()
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if !stopBatch {
		t.Error("approval timeout: expected tool to be denied (stopBatch=true)")
	}
	if execErr != nil {
		t.Errorf("approval timeout: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 0 {
		t.Errorf("approval timeout: expected tool NOT to execute, got %d calls", engine.Calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("approval timeout: wait was not bounded by the approval bound, took %v", elapsed)
	}

	// The UI banner must be cleared: an invalidated event with reason "timeout".
	timeoutInvalidated := false
	for _, ev := range events {
		if ev.Type == EventGuardrailInvalidated {
			if p, ok := ev.Payload.(GuardrailInvalidatedPayload); ok && p.Reason == "timeout" {
				timeoutInvalidated = true
			}
		}
	}
	if !timeoutInvalidated {
		t.Error("expected a guardrail_invalidated event with reason 'timeout' after the approval wait expired")
	}
}

// TestGuardrailApprovalWait_ResolvedAllows proves an approval that arrives
// within the bound allows the tool to proceed (the wait itself is not a
// denial — only expiry is).
func TestGuardrailApprovalWait_ResolvedAllows(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			GuardrailTimeout:         5 * time.Second,
			GuardrailTimeoutBehavior: "fail-open",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(0, true), // fast eval, secret triggers violation
			Logger:     logging.NewNopLogger(),
			OnGuardrail: func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error) {
				return GuardrailDecision{Allow: true, Persist: false}, nil
			},
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if stopBatch {
		t.Error("resolved approval: expected tool to proceed (stopBatch=false)")
	}
	if execErr != nil {
		t.Errorf("resolved approval: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 1 {
		t.Errorf("resolved approval: expected tool to execute, got %d calls", engine.Calls)
	}
}

// TestGuardrailApproval_AutomationDeniesImmediately proves Constitution II.10:
// unattended automation runs have no interactive user, so a non-security
// guardrail violation must be denied immediately (fed back to the model as a
// policy block) instead of waiting for an approval prompt that never comes.
// Regression: the workspace-health-test run stalled for the 5-minute approval
// bound on an `xargs` whitelist violation and then aborted with a misleading
// "context deadline exceeded" when the run's 10-minute deadline expired.
func TestGuardrailApproval_AutomationDeniesImmediately(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		config: AgentConfig{
			WorkspaceID:              "ws1",
			Channel:                  ChannelAutomation,
			GuardrailTimeout:         5 * time.Second, // fast eval; violation triggers denial
			GuardrailTimeoutBehavior: "fail-open",
		},
		deps: AgentRuntimeDeps{
			Engine:     engine,
			Guardrails: newSlowGuardrail(0, true), // fast eval, secret triggers violation
			Logger:     logging.NewNopLogger(),
			OnGuardrail: func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error) {
				t.Fatal("automation must not wait for a guardrail approval")
				return GuardrailDecision{}, nil
			},
		},
	}

	history := []proxy.Message{{Role: proxy.UserRole, Content: "read it"}}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), secretToolCall(), &history, &mu)

	if !stopBatch {
		t.Error("automation violation: expected tool to be denied (stopBatch=true)")
	}
	if execErr != nil {
		t.Errorf("automation violation: expected no exec error, got %v", execErr)
	}
	if engine.Calls != 0 {
		t.Errorf("automation violation: expected tool NOT to execute, got %d calls", engine.Calls)
	}
	// The denial must be fed back to the model with hard policy guidance so it
	// adapts instead of stalling the unattended run.
	last := history[len(history)-1]
	if !strings.Contains(last.Content, "blocked by security policy") {
		t.Errorf("automation violation: expected policy denial guidance in tool result, got: %s", last.Content)
	}
}

func TestAppendToolResult_LockScope(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	history := []proxy.Message{{Role: proxy.UserRole, Content: "x"}}
	tc := proxy.ToolCall{ID: "c1", Function: proxy.FunctionCall{Name: "test_tool"}}

	start := time.Now()
	got := agent.appendToolResult(&history, tc, map[string]string{"key": "value"})
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("appendToolResult took %v — expected fast (<100ms), no lock contention", elapsed)
	}
	last := history[len(history)-1]
	if last.Content != got {
		t.Error("returned content must equal the stored history content (no second marshal)")
	}
	if last.Role != proxy.ToolRole {
		t.Errorf("expected tool role, got %s", last.Role)
	}
	if last.ToolCallID != "c1" {
		t.Errorf("expected tool call ID c1, got %s", last.ToolCallID)
	}
}

func TestAppendToolResult_ConcurrentSafe(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	var mu sync.Mutex
	history := []proxy.Message{{Role: proxy.UserRole, Content: "x"}}

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tc := proxy.ToolCall{ID: fmt.Sprintf("c%d", id), Function: proxy.FunctionCall{Name: "test_tool"}}
			mu.Lock()
			content := agent.appendToolResult(&history, tc, map[string]string{"id": fmt.Sprintf("%d", id)})
			mu.Unlock()
			if content == "" {
				errs <- fmt.Errorf("empty content for call %d", id)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if len(history) != 101 {
		t.Errorf("expected 101 messages (1 initial + 100 results), got %d", len(history))
	}
}

func TestGuardrailApprovalTimeout_DefaultAndConfig(t *testing.T) {
	// Default: 5 min (Hermes parity), applied when the option is unset.
	var opts AgentOptions
	opts.applyDefaults()
	if opts.GuardrailApprovalTimeout != 5*time.Minute {
		t.Errorf("expected default approval timeout 5m, got %v", opts.GuardrailApprovalTimeout)
	}

	// Per-model override wins over the default.
	cfg := models.ModelConfig{GuardrailApprovalTimeoutSecs: 300}
	var overridden AgentOptions
	overridden.ApplyModelConfig(cfg)
	overridden.applyDefaults()
	if overridden.GuardrailApprovalTimeout != 300*time.Second {
		t.Errorf("expected config approval timeout 300s, got %v", overridden.GuardrailApprovalTimeout)
	}
}

func TestFormatGuardrailError_NoRetryMessages(t *testing.T) {
	err := fmt.Errorf("command 'wc' in chain is not in the allowed whitelist")

	denied := formatGuardrailError(err, denialUser)
	if !strings.Contains(denied["error"], "Do NOT retry, rephrase") {
		t.Errorf("explicit-denial message missing no-retry guidance: %s", denied["error"])
	}
	if strings.Contains(denied["error"], "silence is not consent") {
		t.Errorf("explicit-denial message must not claim silence is not consent: %s", denied["error"])
	}

	timeout := formatGuardrailError(err, denialTimeout)
	if !strings.Contains(timeout["error"], "silence is not consent") {
		t.Errorf("timeout message missing consent guidance: %s", timeout["error"])
	}
	if !strings.Contains(timeout["error"], "Do NOT retry, rephrase") {
		t.Errorf("timeout message missing no-retry guidance: %s", timeout["error"])
	}

	policy := formatGuardrailError(err, denialSecurity)
	if !strings.Contains(policy["error"], "blocked by security policy") {
		t.Errorf("policy message missing policy wording: %s", policy["error"])
	}
	if !strings.Contains(policy["error"], "Do NOT retry, rephrase") {
		t.Errorf("policy message missing no-retry guidance: %s", policy["error"])
	}
}
