package profile

import (
	"strings"
	"testing"

	"llm-proxy/models"
)

const validFile = `---
name: Notes
summary: Takes notes
answer_first: true
may_ask: true
max_steps: 6
tools: [filesystem, memory]
memory: on
memory_guidance: true
reminders: false
---
Write things down.
`

func TestParse(t *testing.T) {
	p, err := Parse("notes", Builtin, []byte(validFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := Profile{
		ID: "notes", Name: "Notes", Summary: "Takes notes", Source: Builtin,
		Instructions: "Write things down.", AnswerFirst: true, MayAsk: true, MaxSteps: 6,
		Tools: []string{"filesystem", "memory"}, Memory: models.MemoryModeOn, MemoryGuidance: true,
	}
	if p.ID != want.ID || p.Name != want.Name || p.Summary != want.Summary || p.Instructions != want.Instructions ||
		!p.AnswerFirst || !p.MayAsk || p.MaxSteps != 6 || p.Memory != want.Memory || !p.MemoryGuidance || p.Reminders ||
		strings.Join(p.Tools, ",") != "filesystem,memory" || p.Source != Builtin {
		t.Errorf("Parse = %+v, want %+v", p, want)
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name, id, file, want string
	}{
		{"no frontmatter", "x", "just text", "front matter"},
		{"unterminated frontmatter", "x", "---\nname: X\nbody", "front matter"},
		{"unknown field", "x", "---\nname: X\nsurprise: 1\n---\nbody", "surprise"},
		{"missing name", "x", "---\nsummary: s\n---\nbody", "name"},
		{"empty instructions", "x", "---\nname: X\n---\n  \n", "instructions"},
		{"unknown tool category", "x", "---\nname: X\ntools: [telepathy]\n---\nbody", "telepathy"},
		{"bad memory", "x", "---\nname: X\nmemory: maybe\n---\nbody", "memory"},
		{"steps out of range", "x", "---\nname: X\nmax_steps: 500\n---\nbody", "max_steps"},
		{"bad id", "Bad Id", "---\nname: X\n---\nbody", "id"},
		{"reminders without their category", "x", "---\nname: X\nreminders: true\ntools: [filesystem]\n---\nbody", "reminders"},
	}
	for _, tt := range tests {
		_, err := Parse(tt.id, User, []byte(tt.file))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.want)
		}
	}
}

func TestExcludedTools(t *testing.T) {
	all := Profile{Reminders: true}
	if got := all.ExcludedTools(); len(got) != 0 {
		t.Errorf("a profile that lists no categories hides nothing, got %v", got)
	}
	p := Profile{Reminders: false, Tools: []string{models.CategoryFileSystem}}
	hidden := strings.Join(p.ExcludedTools(), ",")
	for _, tool := range []string{models.ToolTerminalExecute, models.ToolInternetSearch, models.ToolMemoryUpdate, models.ToolScheduleReminder} {
		if !strings.Contains(hidden, tool) {
			t.Errorf("%s should be hidden, hidden = %s", tool, hidden)
		}
	}
	for _, tool := range []string{models.ToolFileRead, models.ToolSystemError, models.ToolApplyGuardrails, models.ToolAutomationJournal} {
		if strings.Contains(hidden, tool) {
			t.Errorf("%s must stay visible (listed or always on), hidden = %s", tool, hidden)
		}
	}
}

func TestExcludedToolsHidesRemindersWhenTheProfileTurnsThemOff(t *testing.T) {
	p := Profile{Reminders: false, Tools: []string{models.CategoryReminders}}
	if !strings.Contains(strings.Join(p.ExcludedTools(), ","), models.ToolScheduleReminder) {
		t.Error("reminders: false hides the reminder tools even when the category is listed")
	}
}
