package memorycapture

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"llm-proxy/internal/platform/memory"
)

type want struct {
	content string
	mode    memory.Mode
}

func extract(t *testing.T, message string) []want {
	t.Helper()
	var got []want
	for _, c := range NewExtractor(nil).Extract(message) {
		got = append(got, want{c.Content, c.Mode})
		if c.Scope != memory.ScopeWorkspace {
			t.Errorf("%q: scope = %q, want workspace", message, c.Scope)
		}
	}
	return got
}

func onDemand(s string) want { return want{s, memory.ModeOnDemand} }
func always(s string) want   { return want{s, memory.ModeAlways} }

// Adding a keyword is one phrase in phrases.go plus, when its behaviour is new, one row here.
func TestExtract_Golden(t *testing.T) {
	cases := []struct {
		name, in string
		want     []want
	}{
		// Explicit store verbs: saved on demand, the trigger stripped.
		{"remember that", "Remember that the staging DB runs on port 5433.", []want{onDemand("the staging DB runs on port 5433")}},
		{"remember to", "remember to run go test before committing", []want{onDemand("run go test before committing")}},
		{"remember with colon", "remember: the staging box is called vertex", []want{onDemand("the staging box is called vertex")}},
		{"dont forget, all spellings", "Please don't forget that deploys go through vertex.", []want{onDemand("deploys go through vertex")}},
		{"dont forget no apostrophe", "dont forget we use pnpm here", []want{onDemand("we use pnpm here")}},
		{"do not forget", "Do not forget the API runs on port 4001", []want{onDemand("the API runs on port 4001")}},
		{"curly apostrophe and caps", "DON’T FORGET to bump the version", []want{onDemand("bump the version")}},
		{"keep in mind", "Keep in mind the build takes ten minutes.", []want{onDemand("the build takes ten minutes")}},
		{"note that", "Note that the linter is strict.", []want{onDemand("the linter is strict")}},
		{"make a note of", "make a note of the new port 4001", []want{onDemand("the new port 4001")}},
		{"for future reference", "For future reference, we use Postgres 16.", []want{onDemand("we use Postgres 16")}},
		{"filler words", "Hey, ok so remember that the port is 5433", []want{onDemand("the port is 5433")}},

		// Standing instructions: only when the user says it is for the future; always injected.
		{"from now on strips the trigger", "From now on answer in short sentences.", []want{always("answer in short sentences")}},
		{"going forward", "Going forward, use tabs.", []want{always("use tabs")}},
		{"in the future", "In the future run gofmt before every commit", []want{always("run gofmt before every commit")}},
		// Several in one message, in order, across sentences and lines.
		{"two sentences", "Remember that the port is 5433. Also from now on answer briefly.", []want{onDemand("the port is 5433"), always("answer briefly")}},
		{"two lines", "remember that the port is 5433\nfrom now on never push without review", []want{onDemand("the port is 5433"), always("never push without review")}},
		{"the same fact twice is saved once", "Remember that the port is 5433. remember that the port is 5433.", []want{onDemand("the port is 5433")}},

		// Not requests to remember.
		{"empty", "", nil},
		{"whitespace", "  \n\t ", nil},
		{"question", "Do you remember the port?", nil},
		{"question with the trigger first", "Remember what I told you?", nil},
		{"asking for it", "Can you remember that?", nil},
		{"the user remembering", "I remember the port was 5433.", nil},
		{"mid-sentence", "I will tell you to remember that thing later", nil},
		{"negative instruction", "Don't remember this conversation.", nil},
		{"remember when", "Remember when we deployed on Friday", nil},
		{"remember how", "remember how the build failed last week", nil},
		{"nothing to save", "Remember that.", nil},
		{"unresolved this", "Remember this.", nil},
		{"unresolved it", "remember it", nil},
		{"unresolved short object", "always do that", nil},
		// "always"/"never"/"whenever" are ordinary imperatives in any instruction, often for this task only: the user's
		// own "from now on" or the review decides, never a guess.
		{"always is not a request to remember", "Always use tabs for indentation.", nil},
		{"never is not a request to remember", "Never reuse items from an earlier brief or from memory.", nil},
		{"whenever is not a request to remember", "Whenever you edit Go files, run gofmt.", nil},
		{"every time is not a request to remember", "Every time you finish, summarise the diff.", nil},
		{"never mind", "Never mind, I'll do it myself.", nil},
		{"always been", "Always been a fan of Go", nil},
		{"inline code", "Use `remember that the port is 5433` as an example", nil},
		{"fenced code", "Here:\n```\nremember that the port is 5433\n```\nthanks", nil},
		{"quoted reply", "> remember that the port is 5433\nokay", nil},
		{"too short", "remember tabs", nil},
		{"too long", "remember that " + strings.Repeat("word ", 130), nil},

		// Sensitive material is never captured.
		{"password", "Remember the password is hunter2", nil},
		{"api key words", "Remember my API key for the staging box", nil},
		{"token", "From now on use this token for deploys", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extract(t, c.in)
			if len(got) != len(c.want) {
				t.Fatalf("Extract(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("Extract(%q)[%d] = %v, want %v", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestExtract_CapsCapturesPerMessage(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 9; i++ {
		sb.WriteString("Remember that fact number " + string(rune('a'+i)) + " holds true. ")
	}
	if got := extract(t, sb.String()); len(got) != maxCapturesPerMessage {
		t.Errorf("captured %d facts, want the cap of %d", len(got), maxCapturesPerMessage)
	}
}

func TestExtract_SecretShapedTextIsRejected(t *testing.T) {
	secret := func(s string) bool { return strings.Contains(s, "sk-LIVE") }
	ex := NewExtractor(secret)
	if got := ex.Extract("Remember that the build step uses sk-LIVE12345 somewhere"); len(got) != 0 {
		t.Errorf("a secret-shaped fact was captured: %v", got)
	}
	if got := ex.Extract("Remember that the build step is slow"); len(got) != 1 {
		t.Errorf("an ordinary fact was dropped by the secret check: %v", got)
	}
}

// Every phrase in the table must work on its own; this is what makes extending it one line.
func TestPhraseTable_EveryTriggerWorksAndTheTableIsClean(t *testing.T) {
	seen := map[string]string{}
	for _, f := range families {
		for _, phrase := range f.triggers {
			if phrase != strings.ToLower(phrase) || strings.TrimSpace(phrase) != phrase || phrase == "" {
				t.Errorf("%s: phrase %q must be lowercase, trimmed and non-empty", f.name, phrase)
			}
			for _, r := range phrase {
				if unicode.IsUpper(r) || r == '’' {
					t.Errorf("%s: phrase %q must use plain lowercase and a straight apostrophe", f.name, phrase)
				}
			}
			if other, dup := seen[phrase]; dup {
				t.Errorf("phrase %q is in both %s and %s", phrase, other, f.name)
			}
			seen[phrase] = f.name
		}
	}
	for phrase, name := range seen {
		in := phrase + " the staging build runs on port 5433"
		got := NewExtractor(nil).Extract(in)
		if len(got) != 1 {
			t.Errorf("%s: %q captured %v, want exactly one fact", name, in, got)
			continue
		}
		if !strings.Contains(strings.ToLower(got[0].Content), "staging build runs on port 5433") {
			t.Errorf("%s: %q lost its body: %q", name, in, got[0].Content)
		}
	}
}

func FuzzExtract(f *testing.F) {
	for _, s := range []string{"", "remember that x y", "```", "`", "> remember", "Don’t forget", "always\n\nnever", "\x00\xff", strings.Repeat("remember ", 50)} {
		f.Add(s)
	}
	ex := NewExtractor(nil)
	f.Fuzz(func(t *testing.T, message string) {
		for _, c := range ex.Extract(message) {
			if strings.TrimSpace(c.Content) == "" || len(c.Content) > maxBodyChars {
				t.Fatalf("invalid candidate %q from %q", c.Content, message)
			}
		}
	})
}

// Text the user pastes (a playbook, a document, a log) is not the user speaking: a message shaped like a document is
// never scanned, however many "remember" or "from now on" sentences it holds.
func TestExtract_PastedDocumentsAreNeverScanned(t *testing.T) {
	cases := map[string]string{
		"a heading":       "# Notes\nRemember that the staging DB runs on port 5433.",
		"a table":         "| a | b |\n|---|---|\nRemember that the staging DB runs on port 5433.",
		"a bullet list":   "Remember these:\n- the staging DB runs on port 5433\n- the build takes ten minutes\n- deploys go through vertex",
		"a numbered list": "Remember that:\n1. the port is 5433\n2. the build is slow\n3. deploys go through vertex",
		"very long":       "Remember that the staging DB runs on port 5433. " + strings.Repeat("Some more text goes here. ", 60),
		"many lines":      strings.Repeat("a line of text\n", 9) + "Remember that the staging DB runs on port 5433.",
	}
	for name, message := range cases {
		t.Run(name, func(t *testing.T) {
			if got := extract(t, message); len(got) != 0 {
				t.Errorf("a document-shaped message was scanned: %v", got)
			}
		})
	}
}

// The real regression: the playbook the operator pasted into the assistant must save nothing.
func TestExtract_ThePastedPlaybookSavesNothing(t *testing.T) {
	data, err := os.ReadFile("../../../data/templates/llm_ai_release_brief.md")
	if err != nil {
		t.Fatal(err)
	}
	if got := extract(t, string(data)); len(got) != 0 {
		t.Errorf("the pasted playbook produced memories: %v", got)
	}
	// The sentence on its own, typed by the user in a short message, is still not an explicit request to remember.
	if got := extract(t, "Everything must come from this run's searches. Never reuse items from an earlier brief or from memory."); len(got) != 0 {
		t.Errorf("an ordinary instruction was captured: %v", got)
	}
}
