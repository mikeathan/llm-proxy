package assistant

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// The approval prompt tells the operator what "Allow and remember" will save:
// the payload carries the plan's lines, taken from the same plan the save applies.
func TestGuardrailApprovalPayload_SaysWhatRememberWillSave(t *testing.T) {
	engine := guardrails.NewGuardrailEngine(
		func() models.AgentGuardrailsConfig {
			return models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls"}}}
		},
		storage.NewPathResolver("", "", ""), nil, nil,
	)
	defer engine.Stop()

	var seen GuardrailBlockedPayload
	agent := &Agent{
		config: AgentConfig{WorkspaceID: "ws1", GuardrailApprovalTimeout: time.Second},
		deps: AgentRuntimeDeps{
			Guardrails: engine,
			Logger:     logging.NewNopLogger(),
			OnGuardrail: func(_ context.Context, p GuardrailBlockedPayload) (GuardrailDecision, error) {
				seen = p
				return GuardrailDecision{Allow: false}, nil
			},
		},
	}
	call := proxy.ToolCall{ID: "c", Function: proxy.FunctionCall{Name: models.ToolTerminalExecute, Arguments: `{"command":"for f in a; do wc -l $f; done"}`}}
	var denial guardrailDenialReason
	agent.awaitGuardrailApproval(context.Background(), call, errors.New("blocked"), &denial)

	if want := []string{"Allow the command: wc"}; !slices.Equal(seen.Remember, want) {
		t.Errorf("Remember = %v, want %v", seen.Remember, want)
	}
	if seen.RememberReason != "" {
		t.Errorf("unexpected RememberReason %q", seen.RememberReason)
	}
}

func TestGuardrailApprovalPayload_SaysWhyNothingCanBeRemembered(t *testing.T) {
	var seen GuardrailBlockedPayload
	agent := &Agent{
		config: AgentConfig{WorkspaceID: "ws1", GuardrailApprovalTimeout: time.Second},
		deps: AgentRuntimeDeps{
			Logger: logging.NewNopLogger(),
			OnGuardrail: func(_ context.Context, p GuardrailBlockedPayload) (GuardrailDecision, error) {
				seen = p
				return GuardrailDecision{}, nil
			},
		},
	}
	call := proxy.ToolCall{ID: "c", Function: proxy.FunctionCall{Name: models.ToolTerminalExecute, Arguments: `{"command":"ls"}`}}
	var denial guardrailDenialReason
	agent.awaitGuardrailApproval(context.Background(), call, errors.New("blocked"), &denial)
	if len(seen.Remember) != 0 || seen.RememberReason == "" {
		t.Errorf("want no lines and a reason, got %v / %q", seen.Remember, seen.RememberReason)
	}
}
