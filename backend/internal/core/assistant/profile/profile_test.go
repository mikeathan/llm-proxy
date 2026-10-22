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
memory_save: explicit
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
		Tools: []string{"filesystem", "memory"}, Memory: models.MemoryModeOn, MemorySave: SaveExplicit,
	}
	if p.ID != want.ID || p.Name != want.Name || p.Summary != want.Summary || p.Instructions != want.Instructions ||
		!p.AnswerFirst || !p.MayAsk || p.MaxSteps != 6 || p.Memory != want.Memory || p.MemorySave != want.MemorySave || p.Reminders ||
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
		{"bad memory_save", "x", "---\nname: X\nmemory_save: always\n---\nbody", "memory_save"},
		{"the retired memory_guidance key", "x", "---\nname: X\nmemory_guidance: true\n---\nbody", "memory_guidance"},
		{"bad memory", "x", "---\nname: X\nmemory: maybe\n---\nbody", "memory"},
		{"steps out of range", "x", "---\nname: X\nmax_steps: 500\n---\nbody", "max_steps"},
		{"bad id", "Bad Id", "---\nname: X\n---\nbody", "id"},
		{"delegation named as a tool category", "x", "---\nname: X\ntools: [delegation]\n---\nbody", "delegates"},
		{"a bad helper id", "x", "---\nname: X\ndelegates: [Web Researcher]\n---\nbody", "delegates"},
		{"a profile that delegates to itself", "x", "---\nname: X\ndelegates: [x]\n---\nbody", "itself"},
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
	if got := all.ExcludedTools(); len(got) != 1 || got[0] != models.ToolDelegate {
		t.Errorf("a profile that lists no categories hides only the delegate tool (it names no helpers), got %v", got)
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

// The delegate tool exists only for a profile that names helpers; its tools list does not govern it.
func TestExcludedToolsShowsTheDelegateToolOnlyWithHelpers(t *testing.T) {
	p, err := Parse("lead", User, []byte("---\nname: Lead\ntools: [search]\ndelegates: [web-researcher]\n---\nLead.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Delegates, ",") != "web-researcher" || strings.Contains(strings.Join(p.ExcludedTools(), ","), models.ToolDelegate) {
		t.Errorf("a profile with helpers offers the delegate tool: delegates %v, hidden %v", p.Delegates, p.ExcludedTools())
	}
	if !strings.Contains(strings.Join((Profile{Tools: []string{models.CategorySearch}}).ExcludedTools(), ","), models.ToolDelegate) {
		t.Error("a profile without helpers hides the delegate tool")
	}
}

func TestExcludedToolsHidesRemindersWhenTheProfileTurnsThemOff(t *testing.T) {
	p := Profile{Reminders: false, Tools: []string{models.CategoryReminders}}
	if !strings.Contains(strings.Join(p.ExcludedTools(), ","), models.ToolScheduleReminder) {
		t.Error("reminders: false hides the reminder tools even when the category is listed")
	}
}

func TestSaveModeSetsWhatTheRunMayDo(t *testing.T) {
	tests := []struct {
		mode                          SaveMode
		guides, captures, offersWrite bool
	}{
		{SaveModel, true, true, true},
		{"", true, true, true}, // a profile that says nothing keeps today's behaviour
		{SaveExplicit, false, true, false},
		{SaveOff, false, false, false},
	}
	for _, tt := range tests {
		p := Profile{MemorySave: tt.mode, Reminders: true}
		if p.GuidesMemorySaves() != tt.guides || p.CapturesMemory() != tt.captures {
			t.Errorf("%q: guides=%v captures=%v, want %v %v", tt.mode, p.GuidesMemorySaves(), p.CapturesMemory(), tt.guides, tt.captures)
		}
		hidden := strings.Join(p.ExcludedTools(), ",")
		writes := strings.Contains(hidden, models.ToolMemoryUpdate) || strings.Contains(hidden, models.ToolMemoryForget)
		if writes == tt.offersWrite {
			t.Errorf("%q: hidden = %q, the memory write tools should be offered=%v", tt.mode, hidden, tt.offersWrite)
		}
		if strings.Contains(hidden, models.ToolMemorySearch) {
			t.Errorf("%q: reading memory is recall, not a save, and must stay offered; hidden = %q", tt.mode, hidden)
		}
	}
}
