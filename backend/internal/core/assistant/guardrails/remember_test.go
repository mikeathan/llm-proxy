package guardrails

import (
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// What "Allow and remember" will do is planned once: the prompt shows the plan's
// lines and the save applies the same plan, so they cannot disagree.

func rememberEngine(t *testing.T, global models.AgentGuardrailsConfig, ws *models.WorkspaceConfig) *GuardrailEngine {
	t.Helper()
	tmp := t.TempDir()
	e := newGuardrailEngine(
		func() models.AgentGuardrailsConfig { return global },
		storage.NewPathResolver(tmp, tmp, tmp),
		persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp)),
		func(string) (*models.WorkspaceConfig, error) { return ws, nil },
		0, 0,
	)
	t.Cleanup(e.Stop)
	return e
}

func terminalArgs(command string) string { return `{"command":` + quote(command) + `}` }

func TestPreviewRemember_Terminal_ListsOnlyWhatIsMissing(t *testing.T) {
	global := models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls", "grep"}}}
	ws := &models.WorkspaceConfig{Guardrails: &models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{AllowedCommands: []string{"git"}}}}
	e := rememberEngine(t, global, ws)

	got := e.PreviewRemember("ws", "terminal", terminalArgs("ls | grep x && git status && wc -l f && wc -c f && sort f"))
	want := []string{"Allow the command: wc", "Allow the command: sort"}
	if !slices.Equal(got.Lines, want) {
		t.Errorf("lines = %v, want %v", got.Lines, want)
	}
	if got.Reason != "" {
		t.Errorf("unexpected reason %q", got.Reason)
	}
}

func TestPreviewRemember_Terminal_SaysWhyNothingCanBeRemembered(t *testing.T) {
	global := models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls"}}}
	e := rememberEngine(t, global, &models.WorkspaceConfig{})
	for name, command := range map[string]string{
		"unreadable":  "echo $(",
		"not literal": "$CMD y",
		"all allowed": "ls -la",
		"empty":       "",
	} {
		got := e.PreviewRemember("ws", "terminal", terminalArgs(command))
		if len(got.Lines) != 0 || got.Reason == "" {
			t.Errorf("%s: want no lines and a reason, got %+v", name, got)
		}
	}
}

func TestPreviewRemember_NeverOffersShellSyntax(t *testing.T) {
	e := rememberEngine(t, models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls"}}}, &models.WorkspaceConfig{})
	got := e.PreviewRemember("ws", "terminal", terminalArgs("for f in *.md; do wc -l $f; done"))
	if text := strings.Join(got.Lines, "|"); strings.Contains(text, "for") || strings.Contains(text, "do") || strings.Contains(text, "done") {
		t.Errorf("shell syntax offered: %v", got.Lines)
	}
}

func TestPreviewRemember_Communication_SaysItStopsAskingApproval(t *testing.T) {
	e := rememberEngine(t, models.AgentGuardrailsConfig{}, &models.WorkspaceConfig{})
	got := e.PreviewRemember("ws", "communication", `{}`)
	if len(got.Lines) == 0 || !strings.Contains(strings.ToLower(strings.Join(got.Lines, " ")), "approval") {
		t.Errorf("lines must say approval is switched off, got %v", got.Lines)
	}
}

func TestPreviewRemember_OtherStepsHaveNothingToRemember(t *testing.T) {
	e := rememberEngine(t, models.AgentGuardrailsConfig{}, &models.WorkspaceConfig{})
	got := e.PreviewRemember("ws", "general", `{}`)
	if len(got.Lines) != 0 || got.Reason == "" {
		t.Errorf("want a reason and no lines, got %+v", got)
	}
}

// Every file tool remembers its path; the category comes from models' one table. edit_file_block was once labelled
// "general" by a hand-written copy, so remembering its approval saved nothing.
func TestPreviewRemember_EveryFileToolRemembersItsPath(t *testing.T) {
	e := rememberEngine(t, models.AgentGuardrailsConfig{}, &models.WorkspaceConfig{})
	for _, tool := range models.ToolsInCategory(models.CategoryFileSystem) {
		got := e.PreviewRemember("ws", models.CategoryOf(tool), `{"path":"notes/plan.md"}`)
		if !slices.Contains(got.Lines, "Allow the path: notes/plan.md") {
			t.Errorf("%s: lines = %v, want the path remembered", tool, got.Lines)
		}
	}
}

func TestPersistOverride_SavesExactlyWhatWasPreviewed(t *testing.T) {
	global := models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls"}}}
	ws := &models.WorkspaceConfig{}
	e := rememberEngine(t, global, ws)
	args := terminalArgs(`ls && for f in a; do wc -l $f; done | sort`)

	preview := e.PreviewRemember("ws", "terminal", args)
	if err := e.PersistOverride("ws", "terminal", "run_terminal", args); err != nil {
		t.Fatal(err)
	}
	saved := ws.Guardrails.Terminal.AllowedCommands
	var previewed []string
	for _, line := range preview.Lines {
		previewed = append(previewed, strings.TrimPrefix(line, "Allow the command: "))
	}
	if !slices.Equal(saved, previewed) || !slices.Equal(saved, []string{"wc", "sort"}) {
		t.Errorf("saved %v, previewed %v, want [wc sort] for both", saved, previewed)
	}
}

func TestPersistOverride_Terminal_DoesNotRepeatWhatIsAlreadyAllowed(t *testing.T) {
	global := models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"ls", "grep"}}}
	ws := &models.WorkspaceConfig{}
	e := rememberEngine(t, global, ws)
	if err := e.PersistOverride("ws", "terminal", "run_terminal", terminalArgs("ls | grep x | wc -l")); err != nil {
		t.Fatal(err)
	}
	if got := ws.Guardrails.Terminal.AllowedCommands; !slices.Equal(got, []string{"wc"}) {
		t.Errorf("saved %v, want only [wc]", got)
	}
}
