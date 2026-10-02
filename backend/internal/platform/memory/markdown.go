package memory

import (
	"fmt"
	"regexp"
	"strings"
)

// Markdown import/export of memory. The file is meant to be read and edited by
// a person, so it is plain markdown: one "### Title" section per fact, an
// optional metadata comment on the next line, then the text.
//
//	# Memory export — workspace-1
//
//	<!-- llm-proxy memory export v1 -->
//
//	### build
//	<!-- scope=workspace mode=always keep=permanent priority=high -->
//	run go build ./... to verify
//
// Missing metadata means the safe defaults (workspace, on demand, permanent,
// normal). Text lines that would read as structure (starting with "#", "<!--"
// or a backslash) are backslash-escaped on export and unescaped on import, so
// any fact survives a round trip unchanged.

const (
	markdownHeadingPrefix = "### "
	markdownFormatMarker  = "<!-- llm-proxy memory export v1 -->"
	markdownMetaOpen      = "<!--"
	markdownMetaClose     = "-->"
	markdownEscape        = `\`
)

var markdownMetaLine = regexp.MustCompile(`^<!--\s*(.*?)\s*-->$`)

var (
	priorityNames  = map[int]string{PriorityLow: "low", PriorityNormal: "normal", PriorityHigh: "high"}
	priorityByName = map[string]int{"low": PriorityLow, "normal": PriorityNormal, "high": PriorityHigh}
)

// Fact is one memory entry as it appears in an import/export file: the
// operator-facing choices (scope, mode, keep, priority), not storage details.
type Fact struct {
	Line     int // 1-based line of the entry's heading, for reporting problems
	Title    string
	Content  string
	Scope    Scope
	Mode     Mode
	Keep     Keep
	Priority int
}

// ParseIssue describes an entry that could not be imported.
type ParseIssue struct {
	Line    int    `json:"line"` // 1-based line of the entry's heading
	Message string `json:"message"`
}

// FormatMarkdown renders entries as a human-editable markdown document.
func FormatMarkdown(title string, entries []MemoryEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Memory export — %s\n\n%s\n", title, markdownFormatMarker)
	for _, e := range entries {
		fmt.Fprintf(&b, "\n%s%s\n", markdownHeadingPrefix, singleLine(e.Title))
		fmt.Fprintf(&b, "%s scope=%s mode=%s keep=%s priority=%s %s\n",
			markdownMetaOpen, factScope(e), factMode(e), factKeep(e), priorityNames[e.Priority], markdownMetaClose)
		for _, line := range strings.Split(strings.TrimRight(e.Content, "\n"), "\n") {
			b.WriteString(escapeMarkdownLine(line))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func factScope(e MemoryEntry) Scope {
	if e.WorkspaceID == "global" || e.MemoryType == UserProfile {
		return ScopeUser
	}
	return ScopeWorkspace
}

func factMode(e MemoryEntry) Mode {
	if e.IsHot() {
		return ModeAlways
	}
	return ModeOnDemand
}

func factKeep(e MemoryEntry) Keep {
	if e.MemoryType == Session {
		return KeepSession
	}
	return KeepPermanent
}

func singleLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func escapeMarkdownLine(line string) string {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, markdownMetaOpen) || strings.HasPrefix(line, markdownEscape) {
		return markdownEscape + line
	}
	return line
}

func unescapeMarkdownLine(line string) string {
	rest, ok := strings.CutPrefix(line, markdownEscape)
	if !ok {
		return line
	}
	if strings.HasPrefix(rest, markdownEscape) || strings.HasPrefix(rest, "#") || strings.HasPrefix(rest, markdownMetaOpen) {
		return rest
	}
	return line
}

// pendingFact is an entry being assembled while scanning the document.
type pendingFact struct {
	line    int
	title   string
	meta    string
	hasMeta bool
	content []string
}

// ParseMarkdown reads a document produced by FormatMarkdown, or written by hand
// in the same shape. Entries that cannot be used are skipped and reported with
// the line of their heading; the rest are returned in order.
func ParseMarkdown(doc string) ([]Fact, []ParseIssue) {
	var facts []Fact
	var issues []ParseIssue
	var cur *pendingFact

	finish := func() {
		if cur == nil {
			return
		}
		fact, err := cur.build()
		if err != nil {
			issues = append(issues, ParseIssue{Line: cur.line, Message: err.Error()})
		} else {
			facts = append(facts, fact)
		}
		cur = nil
	}

	for i, line := range strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, markdownHeadingPrefix):
			finish()
			cur = &pendingFact{line: i + 1, title: singleLine(strings.TrimPrefix(line, markdownHeadingPrefix))}
		case cur == nil:
			// Preamble before the first entry (title, format marker, notes): ignored.
		case !cur.hasMeta && len(cur.content) == 0 && markdownMetaLine.MatchString(line):
			cur.meta = markdownMetaLine.FindStringSubmatch(line)[1]
			cur.hasMeta = true
		default:
			cur.content = append(cur.content, unescapeMarkdownLine(line))
		}
	}
	finish()
	return facts, issues
}

func (p *pendingFact) build() (Fact, error) {
	f := Fact{Line: p.line, Title: p.title, Scope: ScopeWorkspace, Mode: ModeOnDemand, Keep: KeepPermanent, Priority: PriorityNormal}
	f.Content = strings.Trim(strings.Join(p.content, "\n"), "\n")
	if strings.TrimSpace(f.Content) == "" {
		return Fact{}, fmt.Errorf("empty content")
	}
	for _, pair := range strings.Fields(p.meta) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return Fact{}, fmt.Errorf("metadata %q is not key=value", pair)
		}
		if err := f.set(key, value); err != nil {
			return Fact{}, err
		}
	}
	return f, nil
}

// factSetters apply one metadata key. Adding a key is one entry here.
var factSetters = map[string]func(f *Fact, value string) error{
	"scope": func(f *Fact, v string) error {
		if f.Scope = Scope(v); f.Scope != ScopeWorkspace && f.Scope != ScopeUser {
			return fmt.Errorf("unknown scope %q (workspace or user)", v)
		}
		return nil
	},
	"mode": func(f *Fact, v string) error {
		if f.Mode = Mode(v); f.Mode != ModeAlways && f.Mode != ModeOnDemand {
			return fmt.Errorf("unknown mode %q (always or on_demand)", v)
		}
		return nil
	},
	"keep": func(f *Fact, v string) error {
		if f.Keep = Keep(v); f.Keep != KeepPermanent && f.Keep != KeepSession {
			return fmt.Errorf("unknown keep %q (permanent or session)", v)
		}
		return nil
	},
	"priority": func(f *Fact, v string) error {
		p, ok := priorityByName[v]
		if !ok {
			return fmt.Errorf("unknown priority %q (low, normal or high)", v)
		}
		f.Priority = p
		return nil
	},
}

// set applies one metadata pair. Unknown keys are an error, not ignored: a typo
// such as "prority=high" would otherwise silently change what the fact does.
func (f *Fact) set(key, value string) error {
	setter, ok := factSetters[key]
	if !ok {
		return fmt.Errorf("unknown metadata key %q", key)
	}
	return setter(f, value)
}
