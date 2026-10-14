package guardrails

import (
	"slices"
	"testing"

	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// Approving a command with "remember" saves the commands the shell would run
// for the workspace — never shell syntax such as for/do/done, which is not a
// command (and, as an allowed name, would let whatever follows it through).
func TestPersistOverride_Terminal_SavesCommandsNotSyntax(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	cfg := &models.WorkspaceConfig{}
	e := newGuardrailEngine(
		func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} },
		storage.NewPathResolver(tmp, tmp, tmp),
		mgr,
		func(string) (*models.WorkspaceConfig, error) { return cfg, nil },
		0, 0,
	)
	defer e.Stop()

	cases := []struct {
		name    string
		command string
		want    []string
	}{
		{"loop", `for f in *.md; do wc -l $f; done`, []string{"wc"}},
		{"chain", `ls && grep x f | sort`, []string{"ls", "grep", "sort"}},
		{"unreadable command saves nothing", `echo $(`, nil},
		{"non-literal name saves nothing", `$CMD y`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Guardrails = nil
			args := `{"command":` + quote(tc.command) + `}`
			if err := e.PersistOverride("ws", "terminal", "run_terminal", args); err != nil {
				t.Fatalf("PersistOverride: %v", err)
			}
			got := cfg.Guardrails.Terminal.AllowedCommands
			if !slices.Equal(got, tc.want) {
				t.Errorf("saved %v, want %v", got, tc.want)
			}
		})
	}
}

func quote(s string) string {
	out := []byte{'"'}
	for _, r := range s {
		if r == '"' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, string(r)...)
	}
	return string(append(out, '"'))
}
