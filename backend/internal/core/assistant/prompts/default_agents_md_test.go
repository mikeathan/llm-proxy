package prompts

import (
	"strings"
	"testing"

	"llm-proxy/models"
)

// The seeded AGENTS.md is a notes stub: it explains itself to the operator in comments and adds nothing to the prompt.
func TestDefaultAgentsMDIsAnEmptyNotesStub(t *testing.T) {
	if got := models.WithoutComments(DefaultAgentsMD); got != "" {
		t.Errorf("the seeded notes must add nothing to the prompt, but %q would be sent", got)
	}
	if !strings.Contains(DefaultAgentsMD, "profile") {
		t.Error("the stub should point the operator at profiles for general behaviour")
	}
	if IsLegacyDefaultNotes(DefaultAgentsMD) {
		t.Error("the new stub is not a legacy default")
	}
}

// Every AGENTS.md this app ever seeded is recognised, even with different line endings or trailing blank lines;
// an operator's own text, even a lightly edited default, is not.
func TestIsLegacyDefaultNotes(t *testing.T) {
	entries, err := legacyAgentsFiles.ReadDir("legacy")
	if err != nil || len(entries) != 3 {
		t.Fatalf("legacy files = %d, %v; want the three seeded versions", len(entries), err)
	}
	for _, entry := range entries {
		data, _ := legacyAgentsFiles.ReadFile("legacy/" + entry.Name())
		text := string(data)
		for name, variant := range map[string]string{"as is": text, "crlf": strings.ReplaceAll(text, "\n", "\r\n"), "trailing blanks": text + "\n\n  \n"} {
			if !IsLegacyDefaultNotes(variant) {
				t.Errorf("%s (%s) must be recognised as a legacy default", entry.Name(), name)
			}
		}
		if IsLegacyDefaultNotes(text + "\n- Our staging host is staging.example.com.") {
			t.Errorf("%s with the operator's own line added is their notes, not a default", entry.Name())
		}
	}
	for _, mine := range []string{"", "Keep answers short.", "# Workspace Agent\n\nMy own rules."} {
		if IsLegacyDefaultNotes(mine) {
			t.Errorf("%q is not a legacy default", mine)
		}
	}
}
