package assistant

import (
	"context"
	"encoding/json"
	"errors"
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

// Tests for tool_exec.go (guardrail gate, timeout/approval behaviour, result
// bookkeeping) and the truncated-write salvage path.

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

// The host-network hard denial (plan D1/R4) must classify as a security
// boundary so resolveGuardrail denies it synchronously — never routed into the
// approval flow (a click-allow bypass / unattended-stall regression).
func TestIsGuardrailSecurityBoundary_HostNetworkDenial(t *testing.T) {
	if !isGuardrailSecurityBoundary(guardrails.ErrNetworkDisabled) {
		t.Error("raw sentinel must classify as a security boundary")
	}
	if !isGuardrailSecurityBoundary(fmt.Errorf("wrapped: %w", guardrails.ErrNetworkDisabled)) {
		t.Error("wrapped sentinel must classify via errors.Is")
	}
	// Legacy string-based classification must keep working.
	if !isGuardrailSecurityBoundary(errors.New("security violation: path access denied")) {
		t.Error("legacy string classification regressed")
	}
	// A policy-level (approvable-tier) denial must NOT classify as security.
	if isGuardrailSecurityBoundary(errors.New("network tools are disabled by guardrails policy")) {
		t.Error("approvable-tier denial must not classify as a security boundary")
	}
}

func TestExtractTruncatedJSONField(t *testing.T) {
	longBody := strings.Repeat("x", salvageMinContentLen) + " report tail"
	tests := []struct {
		name  string
		raw   string
		field string
		want  string
		// wantContains checks substring when full equality is awkward
		wantContains string
		wantPrefix   string
	}{
		{
			name:         "network-scan truncated no-space colon",
			raw:          `{"content":"# Network Reconnaissance Report\n**Task ID:** network-recon\n\n| Host | Port |\n|------|------|\n| 192.168.50.10 | 22 |\n\n### Hardening\n- Enable NLA\n- Implement IP-based`,
			field:        "content",
			wantPrefix:   "# Network Reconnaissance Report",
			wantContains: "Implement IP-based",
		},
		{
			name:  "spaced colon complete",
			raw:   `{"content": "hello spaced"}`,
			field: "content",
			want:  "hello spaced",
		},
		{
			name:  "space around colon",
			raw:   `{"content" : "hello pad"}`,
			field: "content",
			want:  "hello pad",
		},
		{
			name:  "escaped quotes and newlines",
			raw:   `{"content":"Service says \"No banner\"\nand done"}`,
			field: "content",
			want:  "Service says \"No banner\"\nand done",
		},
		{
			name:  "complete object stops at closing quote",
			raw:   `{"path":"report.md","content":"full body here","extra":1}`,
			field: "content",
			want:  "full body here",
		},
		{
			name:  "path field from complete json",
			raw:   `{"path":"report.md","content":"x"}`,
			field: "path",
			want:  "report.md",
		},
		{
			name:  "missing field",
			raw:   `{"path":"report.md"}`,
			field: "content",
			want:  "",
		},
		{
			name:  "empty raw",
			raw:   "",
			field: "content",
			want:  "",
		},
		{
			name:  "field name only as other key substring",
			raw:   `{"mycontent":"nope","path":"p"}`,
			field: "content",
			want:  "",
		},
		{
			name:         "markdown table unicode",
			raw:          `{"content":"# R\n| Hōst | Port |\n|------|------|\n| café | 22 |\n- Implement IP-based`,
			field:        "content",
			wantPrefix:   "# R",
			wantContains: "café",
		},
		{
			name:  "trailing backslash truncated escape",
			raw:   `{"content":"hello world\`,
			field: "content",
			want:  "hello world",
		},
		{
			name:         "long truncated body",
			raw:          `{"content":"` + longBody,
			field:        "content",
			wantPrefix:   "xxx",
			wantContains: "report tail",
		},
		{
			name:  "tabs and escaped slash-ish",
			raw:   `{"content":"a\tb\\nc"}`,
			field: "content",
			want:  "a\tb\\nc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTruncatedJSONField(tt.raw, tt.field)
			if tt.want != "" && got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("prefix: got %q want prefix %q", clip(got, 60), tt.wantPrefix)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Fatalf("contains %q missing in %q", tt.wantContains, clip(got, 80))
			}
			if tt.want == "" && tt.wantPrefix == "" && tt.wantContains == "" && got != "" {
				t.Fatalf("want empty, got %q", got)
			}
		})
	}
}

func TestExtractToolArgField(t *testing.T) {
	t.Run("valid json prefers unmarshal", func(t *testing.T) {
		raw := `{"path":"report.md","content":"hello world report body that is long enough"}`
		if got := extractToolArgField(raw, "path"); got != "report.md" {
			t.Fatalf("path: got %q", got)
		}
		if got := extractToolArgField(raw, "content"); !strings.HasPrefix(got, "hello") {
			t.Fatalf("content: got %q", got)
		}
	})
	t.Run("invalid falls back to truncated extract", func(t *testing.T) {
		raw := `{"content":"# Report\nline2 and more text that continues`
		got := extractToolArgField(raw, "content")
		if !strings.HasPrefix(got, "# Report") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("non-string field type ignored", func(t *testing.T) {
		raw := `{"content":123}`
		if got := extractToolArgField(raw, "content"); got != "" {
			t.Fatalf("want empty for non-string, got %q", got)
		}
	})
}

func TestSalvageTruncatedWrite_PersistsWhenPathPresent(t *testing.T) {
	long := strings.Repeat("y", salvageMinContentLen) + "\n# Report body"
	engine := &MockEngine{Result: "File written successfully"}
	agent := &Agent{
		deps: AgentRuntimeDeps{
			Engine: engine,
			Logger: logging.NewNopLogger(),
			// Guardrails nil → skip path policy; tests Engine persist wiring only.
		},
	}
	tc := proxy.ToolCall{
		ID:   "call_w",
		Type: "function",
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"path":"out/report.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("expected salvage handled")
	}
	if !strings.Contains(report, "Report body") {
		t.Fatalf("unexpected report: %q", clip(report, 60))
	}
	if engine.Calls != 1 {
		t.Fatalf("expected 1 engine call, got %d", engine.Calls)
	}
	if engine.LastCall.Function.Name != models.ToolFileWrite {
		t.Fatalf("tool=%q", engine.LastCall.Function.Name)
	}
	if !strings.Contains(engine.LastCall.Function.Arguments, "out/report.md") {
		t.Fatalf("path missing in args: %q", engine.LastCall.Function.Arguments)
	}
	if !strings.Contains(engine.LastCall.Function.Arguments, "Report body") {
		t.Fatalf("content missing in args: %q", clip(engine.LastCall.Function.Arguments, 80))
	}
}

func TestSalvageTruncatedWrite_NoPathSkipsEngine(t *testing.T) {
	long := strings.Repeat("z", salvageMinContentLen) + " content only"
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled || !strings.Contains(report, "content only") {
		t.Fatalf("handled=%v report=%q", handled, clip(report, 40))
	}
	if engine.Calls != 0 {
		t.Fatalf("engine must not run without path, calls=%d", engine.Calls)
	}
}

func TestSalvageTruncatedWrite_PersistFailStillHandled(t *testing.T) {
	long := strings.Repeat("q", salvageMinContentLen) + " still complete"
	engine := &MockEngine{Err: fmt.Errorf("disk full")}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"path":"r.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("persist failure must still handle salvage")
	}
	if !strings.Contains(report, "still complete") {
		t.Fatalf("report=%q", clip(report, 40))
	}
	if engine.Calls != 1 {
		t.Fatalf("expected engine attempt, got %d", engine.Calls)
	}
}

func TestSalvageTruncatedWrite_AppendUsesAppendTool(t *testing.T) {
	long := strings.Repeat("a", salvageMinContentLen) + " append tail"
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileAppend,
			Arguments: `{"path":"log.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	_, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("expected handled")
	}
	if engine.LastCall.Function.Name != models.ToolFileAppend {
		t.Fatalf("expected append_file, got %q", engine.LastCall.Function.Name)
	}
}

func TestTrySalvageWriteContent(t *testing.T) {
	long := strings.Repeat("y", salvageMinContentLen)
	longPlus := long + " tail"

	tests := []struct {
		name    string
		tool    string
		args    string
		wantLen int // 0 = no salvage; >0 = min length of salvaged
		wantHas string
	}{
		{
			name: "write short no salvage",
			tool: models.ToolFileWrite,
			args: `{"content":"too short"}`,
		},
		{
			name:    "write exact min length",
			tool:    models.ToolFileWrite,
			args:    `{"content":"` + long,
			wantLen: salvageMinContentLen,
		},
		{
			name: "write one under min",
			tool: models.ToolFileWrite,
			args: `{"content":"` + strings.Repeat("z", salvageMinContentLen-1),
		},
		{
			name:    "write long truncated",
			tool:    models.ToolFileWrite,
			args:    `{"content":"` + longPlus,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
		{
			name:    "append long truncated",
			tool:    models.ToolFileAppend,
			args:    `{"content":"` + longPlus,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
		{
			name: "scan tool never salvages",
			tool: models.ToolNetworkScan,
			args: `{"content":"` + longPlus,
		},
		{
			name: "read tool never salvages",
			tool: models.ToolFileRead,
			args: `{"content":"` + longPlus,
		},
		{
			name:    "valid complete write still extracts if long enough",
			tool:    models.ToolFileWrite,
			args:    `{"path":"r.md","content":"` + longPlus + `"}`,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := proxy.ToolCall{Function: proxy.FunctionCall{Name: tt.tool, Arguments: tt.args}}
			got := trySalvageWriteContent(tc)
			if tt.wantLen == 0 {
				if got != "" {
					t.Fatalf("want no salvage, got len=%d", len(got))
				}
				return
			}
			if len(strings.TrimSpace(got)) < tt.wantLen {
				t.Fatalf("got len=%d want >= %d", len(got), tt.wantLen)
			}
			if tt.wantHas != "" && !strings.Contains(got, tt.wantHas) {
				t.Fatalf("missing %q in salvaged", tt.wantHas)
			}
		})
	}
}

func TestHandleToolCallParseError_CapsSyntaxStreak(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	err := fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error: missing closing quote"}}`)

	for i := 1; i < sessionMaxSyntaxParseRetries; i++ {
		if s.handleToolCallParseError(err) {
			t.Fatalf("should not give up on attempt %d", i)
		}
	}
	if !s.handleToolCallParseError(err) {
		t.Fatal("should give up after sessionMaxSyntaxParseRetries")
	}
	if s.syntaxParseStreak != sessionMaxSyntaxParseRetries {
		t.Fatalf("streak=%d want %d", s.syntaxParseStreak, sessionMaxSyntaxParseRetries)
	}
}

func TestHandleToolCallParseError_NonSyntaxDoesNotCapSameWay(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	// Length-style server error (not missing closing quote / unexpected end alone in isJSONSyntaxError path)
	// isJSONSyntaxError also matches "unexpected end" — use a generic tool-call parse error without those.
	err := fmt.Errorf(`llm completion failed: LLM chat error 500: Failed to parse tool call arguments as JSON: invalid character`)

	// Non-syntax path should never return giveUp=true from syntax cap
	for i := 0; i < sessionMaxSyntaxParseRetries+2; i++ {
		if s.handleToolCallParseError(err) {
			t.Fatalf("non-syntax path must not give up via syntax cap (i=%d)", i)
		}
	}
	if s.syntaxParseStreak != 0 {
		t.Fatalf("syntax streak should stay 0, got %d", s.syntaxParseStreak)
	}
	if s.totalErrorStreak == 0 {
		t.Fatal("totalErrorStreak should increase on non-syntax path")
	}
}

func TestResetParseErrorState_ClearsSyntaxStreak(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	err := fmt.Errorf(`Failed to parse tool call arguments as JSON: missing closing quote`)
	_ = s.handleToolCallParseError(err)
	if s.syntaxParseStreak == 0 {
		t.Fatal("expected streak > 0")
	}
	s.resetParseErrorState()
	if s.syntaxParseStreak != 0 {
		t.Fatalf("reset should clear syntaxParseStreak, got %d", s.syntaxParseStreak)
	}
}

func TestBestAvailableAnswer(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.UserRole, Content: "go"},
		{Role: proxy.AssistantRole, Content: "short"},
		{Role: proxy.ToolRole, Content: "data"},
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
	})
	got := s.bestAvailableAnswer()
	if !strings.Contains(got, "sufficiently long") {
		t.Fatalf("got %q", got)
	}

	s2 := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "tiny"},
	})
	if s2.bestAvailableAnswer() != "" {
		t.Fatal("short assistant content must not count")
	}
}

// TestResolveFallbackAnswer_NoSynthesis ensures a run that completes work via a
// successful write but returns no final text falls back to bestAvailableAnswer
// (""), and crucially does NOT dump the written file's contents as the report.
func TestResolveFallbackAnswer_SynthesizesSummaryForCompletedWrite(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	writeArgs, _ := json.Marshal(map[string]string{
		"path":    "ts-dashboard/app.ts",
		"content": strings.Repeat("x", salvageMinContentLen) + " const x = 1;",
	})
	s := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{
			ID:       "c1",
			Function: proxy.FunctionCall{Name: models.ToolFileWrite, Arguments: string(writeArgs)},
		}}},
		{Role: proxy.ToolRole, ToolCallID: "c1", Content: `"File written successfully"`},
	})
	got := s.resolveFallbackAnswer()
	if got != "" {
		t.Fatalf("expected empty fallback (no synthesized summary), got %q", clip(got, 80))
	}
	if strings.Contains(got, "const x = 1;") {
		t.Fatalf("must not dump file content, got %q", clip(got, 80))
	}
}

func TestResolveFallbackAnswer_Priority(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}

	// Last substantive non-tool assistant text wins (tool-call messages skipped).
	s := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
		{
			Role:    proxy.AssistantRole,
			Content: "I will write the file now.",
			ToolCalls: []proxy.ToolCall{{
				Function: proxy.FunctionCall{
					Name:      models.ToolFileWrite,
					Arguments: `{"content":"` + strings.Repeat("z", salvageMinContentLen),
				},
			}},
		},
	})
	if got := s.resolveFallbackAnswer(); !strings.Contains(got, "sufficiently long") {
		t.Fatalf("expected last substantive text, got %q", clip(got, 60))
	}

	// Text-only fallback.
	s2 := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
	})
	if got := s2.resolveFallbackAnswer(); !strings.Contains(got, "sufficiently long") {
		t.Fatalf("text fallback: got %q", got)
	}

	// Empty history.
	s3 := newRunSession(agent, nil, nil)
	if got := s3.resolveFallbackAnswer(); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestHandleTurnError_GiveUpWhenNoFallback(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, []proxy.Message{
		{
			Role: proxy.AssistantRole,
			ToolCalls: []proxy.ToolCall{{
				Function: proxy.FunctionCall{
					Name:      models.ToolFileWrite,
					Arguments: `{"content":"` + strings.Repeat("q", salvageMinContentLen),
				},
			}},
		},
	})
	s.syntaxParseStreak = sessionMaxSyntaxParseRetries - 1
	s.starvationCount = 0
	err := fmt.Errorf(`llm completion failed: Failed to parse tool call arguments as JSON: missing closing quote`)

	done, reply, outErr := s.handleTurnError(err)
	if !done {
		t.Fatalf("expected done=true, got done=%v reply=%q err=%v", done, reply, outErr)
	}
	if reply != "" {
		t.Fatalf("expected empty reply (no salvage), got %q", clip(reply, 60))
	}
	if outErr == nil {
		t.Fatal("expected stall error when no fallback answer is available")
	}
}

func TestScanJSONStringBody(t *testing.T) {
	if got := scanJSONStringBody(`hello\"world" rest`); got != `hello\"world` {
		t.Fatalf("got %q", got)
	}
	if got := scanJSONStringBody(`no close`); got != `no close` {
		t.Fatalf("truncated got %q", got)
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TestHandleToolTurn_SalvagePersistsReportAsText verifies Fix A: when a tool
// call's args are truncated so the report is salvaged, the persisted history
// entry must carry the salvaged Content AND have ToolCalls cleared (otherwise
// the report is lost on session reopen and the frontend renders a blank
// tool-call card).
func TestHandleToolTurn_SalvagePersistsReportAsText(t *testing.T) {
	long := strings.Repeat("r", salvageMinContentLen) + " final report body"
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{})
	s := newRunSession(agent, context.Background(), nil)

	turnMsg := proxy.Message{
		Role: proxy.AssistantRole,
		ToolCalls: []proxy.ToolCall{{
			ID: "call_salvage",
			Function: proxy.FunctionCall{
				Name:      models.ToolFileWrite,
				Arguments: `{"content":"` + long, // truncated: no closing brace, no path
			},
		}},
	}

	done, reply, err := s.handleToolTurn(turnMsg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("expected done=true after salvage")
	}
	if !strings.Contains(reply, "final report body") {
		t.Fatalf("reply missing report: %q", clip(reply, 80))
	}

	if len(s.history) == 0 {
		t.Fatal("history not recorded")
	}
	last := s.history[len(s.history)-1]
	if last.Content != reply {
		t.Fatalf("persisted content = %q, want reply %q", clip(last.Content, 80), clip(reply, 80))
	}
	if len(last.ToolCalls) != 0 {
		t.Fatalf("persisted ToolCalls must be cleared after salvage, got %d", len(last.ToolCalls))
	}
}

// TestTruncateHistory_PreservesFirstUserMessage verifies Fix B: truncation for
// oversized history must never drop the original user task, or the persisted
// session renders blank on reopen.
func TestTruncateHistory_PreservesFirstUserMessage(t *testing.T) {
	big := strings.Repeat("x", MaxHistoryChars) // exceeds budget alone
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "list all files and report"},
		{Role: proxy.AssistantRole, Content: big},
		{Role: proxy.ToolRole, Content: "result"},
	}
	out := TruncateHistory(history, MaxHistoryChars)

	foundUser := false
	for _, m := range out {
		if m.Role == proxy.UserRole && m.Content == "list all files and report" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatalf("first user message dropped by truncation; out=%d msgs", len(out))
	}
}

// TestTruncateHistory_PersistedCeilingPreservesToolCalls verifies the Bug 2 fix:
// the persisted-session ceiling is MaxPersistedHistoryChars (256KB), not the
// small MaxHistoryChars (12KB). A multi-tool history that exceeds the OLD 12KB
// bound but stays under the new ceiling must be persisted in full — every tool
// call/result retained — so a reload shows the complete reasoning trail instead
// of dropping earlier tool calls.
func TestTruncateHistory_PersistedCeilingPreservesToolCalls(t *testing.T) {
	// Total content well above the old 12KB bound but under the 256KB ceiling.
	reasoning := strings.Repeat("r", 8*1024)
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "list all files and report"},
	}
	toolCount := 6
	for i := 0; i < toolCount; i++ {
		history = append(history,
			proxy.Message{
				Role:             proxy.AssistantRole,
				Content:          "",
				ReasoningContent: reasoning,
				ToolCalls: []proxy.ToolCall{{
					ID:       "tc-1",
					Type:     "function",
					Function: proxy.FunctionCall{Name: "list_directory", Arguments: `{"path":"/tmp"}`},
				}},
			},
			proxy.Message{Role: proxy.ToolRole, Content: "result", ToolCallID: "tc-1"},
		)
	}

	out := TruncateHistory(history, MaxPersistedHistoryChars)

	if len(out) != len(history) {
		t.Fatalf("persisted history truncated: got %d msgs, want %d (full) — tool calls lost", len(out), len(history))
	}
	toolMsgs := 0
	for _, m := range out {
		if m.Role == proxy.ToolRole {
			toolMsgs++
		}
	}
	if toolMsgs != toolCount {
		t.Errorf("expected %d tool messages preserved, got %d", toolCount, toolMsgs)
	}
}

// ---------------------------------------------------------------------------
// Terminal tool-failure policy (tool-error-classification)
// ---------------------------------------------------------------------------

// newToolPolicyAgent builds a minimal agent for the terminal-failure policy
// tests: a tool-name-agnostic guardrail engine that allows everything, the given
// channel, and a fresh run session so disabled/streak state is exercisable.
func newToolPolicyAgent(channel EventChannel, engine Engine) *Agent {
	a := &Agent{
		config: AgentConfig{WorkspaceID: "ws1", Channel: channel},
		deps: AgentRuntimeDeps{
			Engine: engine,
			Guardrails: guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
				return models.AgentGuardrailsConfig{
					Search:        models.SearchGuardrailsConfig{Enabled: true},
					Communication: models.CommunicationGuardrailsConfig{Enabled: true},
				}
			}, storage.NewPathResolver("", "", ""), nil, nil),
			Logger: logging.NewNopLogger(),
		},
	}
	a.runS = newRunSession(a, context.Background(), nil)
	return a
}

func policyToolCall(name string) proxy.ToolCall {
	return proxy.ToolCall{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: name, Arguments: `{}`}}
}

func lastToolContent(history []proxy.Message) string {
	return history[len(history)-1].Content
}

func TestToolPolicy_TerminalEssentialAutomationFails(t *testing.T) {
	engine := &MockEngine{Err: fmt.Errorf("auth: %w", models.ErrToolUnavailable)}
	agent := newToolPolicyAgent(ChannelAutomation, engine)

	history := []proxy.Message{}
	var mu sync.Mutex
	stopBatch, execErr := agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu)

	if stopBatch {
		t.Fatal("terminal tool failure is not a guardrail denial")
	}
	if !agent.toolFailureIsRunFatal(execErr) {
		t.Fatalf("automation must treat a terminal essential failure as run-fatal, got %v", execErr)
	}
	if _, disabled := agent.toolFailure.disabled["test_tool"]; !disabled {
		t.Error("terminal tool must be disabled for the rest of the run")
	}
	if !strings.Contains(lastToolContent(history), "TOOL UNAVAILABLE") {
		t.Errorf("expected the actionable directive as the tool result, got %q", lastToolContent(history))
	}
}

func TestToolPolicy_TerminalEssentialChatDisablesAndContinues(t *testing.T) {
	engine := &MockEngine{Err: fmt.Errorf("auth: %w", models.ErrToolUnavailable)}
	agent := newToolPolicyAgent(ChannelAssistant, engine)

	history := []proxy.Message{}
	var mu sync.Mutex
	_, execErr := agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu)
	if execErr == nil {
		t.Fatal("expected the terminal error to be returned to the caller")
	}
	if agent.toolFailureIsRunFatal(execErr) {
		t.Error("chat must never treat a terminal failure as run-fatal")
	}

	// A second call to the disabled tool short-circuits: no engine hit, clean result.
	before := engine.Calls
	stopBatch, err2 := agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu)
	if stopBatch || err2 != nil {
		t.Fatalf("short-circuit must be a clean recorded result, got stop=%v err=%v", stopBatch, err2)
	}
	if engine.Calls != before {
		t.Error("a disabled tool must not reach the engine again")
	}
	if !strings.Contains(lastToolContent(history), "TOOL UNAVAILABLE") {
		t.Errorf("expected the unavailable directive on short-circuit, got %q", lastToolContent(history))
	}
}

func TestToolPolicy_DeliveryFailureWarnsAndNeverFails(t *testing.T) {
	for _, channel := range []EventChannel{ChannelAssistant, ChannelAutomation} {
		engine := &MockEngine{Err: fmt.Errorf("auth: %w", models.ErrToolUnavailable)}
		agent := newToolPolicyAgent(channel, engine)

		history := []proxy.Message{}
		var mu sync.Mutex
		stopBatch, execErr := agent.executeSingleToolStep(context.Background(), policyToolCall(models.ToolNotifyUser), &history, &mu)

		if stopBatch || execErr != nil {
			t.Fatalf("channel %q: delivery failure must not error, got stop=%v err=%v", channel, stopBatch, execErr)
		}
		if len(agent.ToolWarnings()) != 1 {
			t.Fatalf("channel %q: expected 1 delivery warning, got %v", channel, agent.ToolWarnings())
		}
		if !strings.Contains(lastToolContent(history), "DELIVERY FAILED") {
			t.Errorf("channel %q: expected the delivery directive, got %q", channel, lastToolContent(history))
		}
	}
}

func TestToolPolicy_UnclassifiedErrorContinues(t *testing.T) {
	engine := &MockEngine{Err: errors.New("server returned unexpected status: 404 Not Found")}
	agent := newToolPolicyAgent(ChannelAssistant, engine)

	history := []proxy.Message{}
	var mu sync.Mutex
	_, execErr := agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu)
	if execErr == nil {
		t.Fatal("expected an error for a failed tool")
	}
	if agent.toolFailureIsRunFatal(execErr) {
		t.Error("an unclassified (input/transient) error must not be run-fatal")
	}
	if agent.toolFailure.streak != 1 {
		t.Errorf("toolFailure streak = %d, want 1", agent.toolFailure.streak)
	}
	if strings.Contains(lastToolContent(history), "TOOL UNAVAILABLE") {
		t.Errorf("unclassified errors keep the raw error result, got %q", lastToolContent(history))
	}
}

func TestToolPolicy_FailureStreakSuppressesTools(t *testing.T) {
	engine := &MockEngine{Err: errors.New("boom")}
	agent := newToolPolicyAgent(ChannelAssistant, engine)

	history := []proxy.Message{}
	var mu sync.Mutex
	var lastErr error
	for i := 0; i < toolFailureStreakLimit; i++ {
		_, lastErr = agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu)
	}
	if !errors.Is(lastErr, errToolFailureLimit) {
		t.Fatalf("expected errToolFailureLimit after %d failures, got %v", toolFailureStreakLimit, lastErr)
	}
	if !agent.toolFailure.suppressed {
		t.Error("expected suppression once the failure bound trips")
	}

	before := engine.Calls
	_, err := agent.executeSingleToolStep(context.Background(), policyToolCall("other_tool"), &history, &mu)
	if err != nil {
		t.Fatalf("suppressed tools must short-circuit cleanly, got %v", err)
	}
	if engine.Calls != before {
		t.Error("no tool should reach the engine once tools are suppressed")
	}
	if !strings.Contains(lastToolContent(history), "Too many consecutive tool failures") {
		t.Errorf("expected the suppression directive, got %q", lastToolContent(history))
	}
}

func TestToolPolicy_SuccessResetsFailureStreak(t *testing.T) {
	engine := &MockEngine{Result: "ok"}
	agent := newToolPolicyAgent(ChannelAssistant, engine)
	agent.toolFailure.streak = 2

	history := []proxy.Message{}
	var mu sync.Mutex
	if _, err := agent.executeSingleToolStep(context.Background(), policyToolCall("test_tool"), &history, &mu); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if agent.toolFailure.streak != 0 {
		t.Errorf("toolFailure streak = %d, want 0 after a successful call", agent.toolFailure.streak)
	}
}
