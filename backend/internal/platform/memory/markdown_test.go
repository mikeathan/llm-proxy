package memory

import (
	"slices"
	"strings"
	"testing"
)

func entryOf(title, content string, mut func(*MemoryEntry)) MemoryEntry {
	e := MemoryEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: title, Content: content, Priority: PriorityNormal}
	if mut != nil {
		mut(&e)
	}
	return e
}

// withoutLines clears the heading line numbers so facts compare by content.
func withoutLines(facts []Fact) []Fact {
	out := slices.Clone(facts)
	for i := range out {
		out[i].Line = 0
	}
	return out
}

func TestMarkdown_RoundTripKeepsEveryFieldAndAwkwardContent(t *testing.T) {
	nasty := "line one\n### not a heading\n---\n<!-- not metadata -->\n\\backslash at start\n\n  indented\nüñíçødé ✓"
	entries := []MemoryEntry{
		entryOf("build", "run go build ./...", func(e *MemoryEntry) { e.Tags = []string{HotTag}; e.Priority = PriorityHigh }),
		entryOf("scratch", "only this conversation", func(e *MemoryEntry) { e.MemoryType = Session }),
		entryOf("name", "Alice", func(e *MemoryEntry) {
			e.WorkspaceID = "global"
			e.MemoryType = UserProfile
			e.Tags = []string{HotTag}
			e.Priority = PriorityLow
		}),
		entryOf("nasty", nasty, nil),
	}

	facts, issues := ParseMarkdown(FormatMarkdown("workspace-1", entries))
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	want := []Fact{
		{Title: "build", Content: "run go build ./...", Scope: ScopeWorkspace, Mode: ModeAlways, Keep: KeepPermanent, Priority: PriorityHigh},
		{Title: "scratch", Content: "only this conversation", Scope: ScopeWorkspace, Mode: ModeOnDemand, Keep: KeepSession, Priority: PriorityNormal},
		{Title: "name", Content: "Alice", Scope: ScopeUser, Mode: ModeAlways, Keep: KeepPermanent, Priority: PriorityLow},
		{Title: "nasty", Content: nasty, Scope: ScopeWorkspace, Mode: ModeOnDemand, Keep: KeepPermanent, Priority: PriorityNormal},
	}
	if !slices.Equal(withoutLines(facts), want) {
		t.Errorf("round trip changed the facts:\n got %+v\nwant %+v", facts, want)
	}
}

func TestMarkdown_ExportIsReadableAndStable(t *testing.T) {
	doc := FormatMarkdown("ws", []MemoryEntry{entryOf("build", "run go build", func(e *MemoryEntry) { e.Tags = []string{HotTag} })})
	for _, want := range []string{"# Memory export — ws", "### build", "scope=workspace", "mode=always", "run go build"} {
		if !strings.Contains(doc, want) {
			t.Errorf("export missing %q:\n%s", want, doc)
		}
	}
	if FormatMarkdown("ws", nil) == "" {
		t.Error("an empty export still has a header, so the file is recognisable")
	}
	again := FormatMarkdown("ws", []MemoryEntry{entryOf("build", "run go build", func(e *MemoryEntry) { e.Tags = []string{HotTag} })})
	if doc != again {
		t.Error("export must be deterministic")
	}
}

// Operators edit the file by hand: missing metadata means the safe defaults.
func TestMarkdown_HandWrittenEntriesGetDefaults(t *testing.T) {
	doc := "# My notes\n\nsome preamble that is ignored\n\n### Deploys\nGo through staging first.\n\n### Tabs\n<!-- mode=always -->\nUse tabs.\n"
	facts, issues := ParseMarkdown(doc)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	want := []Fact{
		{Title: "Deploys", Content: "Go through staging first.", Scope: ScopeWorkspace, Mode: ModeOnDemand, Keep: KeepPermanent, Priority: PriorityNormal},
		{Title: "Tabs", Content: "Use tabs.", Scope: ScopeWorkspace, Mode: ModeAlways, Keep: KeepPermanent, Priority: PriorityNormal},
	}
	if !slices.Equal(withoutLines(facts), want) {
		t.Errorf("got %+v\nwant %+v", facts, want)
	}
}

func TestMarkdown_BadEntriesAreReportedWithTheirLineAndSkipped(t *testing.T) {
	doc := strings.Join([]string{
		"### ok",                   // line 1
		"fine",                     // 2
		"### bad priority",         // 3
		"<!-- priority=urgent -->", // 4
		"text",                     // 5
		"### bad scope",            // 6
		"<!-- scope=galaxy -->",    // 7
		"text",                     // 8
		"### typo key",             // 9
		"<!-- prority=high -->",    // 10
		"text",                     // 11
		"### empty",                // 12
		"",                         // 13
	}, "\n")
	facts, issues := ParseMarkdown(doc)

	if len(facts) != 1 || facts[0].Title != "ok" || facts[0].Line != 1 {
		t.Errorf("only the valid entry survives (with its line), got %+v", facts)
	}
	gotLines := make([]int, len(issues))
	for i, is := range issues {
		gotLines[i] = is.Line
		if is.Message == "" {
			t.Errorf("issue at line %d has no message", is.Line)
		}
	}
	if !slices.Equal(gotLines, []int{3, 6, 9, 12}) {
		t.Errorf("issue lines = %v, want [3 6 9 12]", gotLines)
	}
}

func TestMarkdown_EmptyAndHeaderOnlyDocumentsAreQuiet(t *testing.T) {
	for _, doc := range []string{"", "\n\n", FormatMarkdown("ws", nil)} {
		facts, issues := ParseMarkdown(doc)
		if len(facts) != 0 || len(issues) != 0 {
			t.Errorf("doc %q: facts %v issues %v, want none", doc, facts, issues)
		}
	}
}

func TestMarkdown_TitlesAreSingleLine(t *testing.T) {
	facts, _ := ParseMarkdown(FormatMarkdown("ws", []MemoryEntry{entryOf("two\nlines\t here", "body", nil)}))
	if len(facts) != 1 || facts[0].Title != "two lines here" {
		t.Errorf("title = %+v", facts)
	}
}

func TestMarkdown_WindowsLineEndingsAreAccepted(t *testing.T) {
	facts, issues := ParseMarkdown("### a\r\n<!-- mode=always -->\r\nline1\r\nline2\r\n")
	if len(issues) != 0 || len(facts) != 1 || facts[0].Content != "line1\nline2" || facts[0].Mode != ModeAlways {
		t.Errorf("CRLF not handled: %+v %+v", facts, issues)
	}
}
