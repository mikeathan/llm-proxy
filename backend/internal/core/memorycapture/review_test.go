package memorycapture

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"llm-proxy/internal/platform/memory"
)

type fakeCompleter struct {
	reply         string
	err           error
	calls         int
	system, input string
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string) (string, error) {
	f.calls++
	f.system, f.input = system, user
	return f.reply, f.err
}

var chat = []Turn{
	{Role: "user", Text: "We deploy through the vertex host and I prefer short answers."},
	{Role: "assistant", Text: "Noted, I will keep answers short."},
}

func review(t *testing.T, reply string, lib Library, secret SecretCheck) []Suggestion {
	t.Helper()
	got, err := NewReviewer(&fakeCompleter{reply: reply}, lib, "SYSTEM", secret).Review(context.Background(), "ws", chat)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	return got
}

func TestReview_ParsesWhateverShapeTheModelReplies(t *testing.T) {
	const item = `{"content":"We deploy through the vertex host","scope":"workspace","mode":"on_demand"}`
	cases := map[string]string{
		"bare json":                    `[` + item + `]`,
		"fenced":                       "```json\n[" + item + "]\n```",
		"prose around":                 "Sure! Here are the facts:\n[" + item + "]\nLet me know if you want more.",
		"after a thinking block":       "<think>maybe [1,2,3] or [\"x\"]</think>\n[" + item + "]",
		"a bracket in the prose first": "[note] I found one fact:\n[" + item + "]",
		"a bracket after the array":    "[" + item + "]\n(see [1] above)",
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			got := review(t, reply, &fakeLibrary{}, nil)
			if len(got) != 1 || got[0].Content != "We deploy through the vertex host" || got[0].Scope != memory.ScopeWorkspace || got[0].Mode != memory.ModeOnDemand {
				t.Errorf("suggestions = %+v", got)
			}
			if got[0].Origin != OriginReview {
				t.Errorf("origin = %q, want %q", got[0].Origin, OriginReview)
			}
		})
	}
}

func TestReview_UnusableRepliesGiveNoSuggestionsNotAnError(t *testing.T) {
	for name, reply := range map[string]string{
		"empty":          "",
		"prose only":     "I could not find anything worth remembering.",
		"malformed json": `[{"content": "oops"`,
		"an object":      `{"content":"x"}`,
		"empty array":    `[]`,
		"wrong item":     `[1, 2, 3]`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := review(t, reply, &fakeLibrary{}, nil); len(got) != 0 {
				t.Errorf("suggestions = %+v, want none", got)
			}
		})
	}
}

func TestReview_EverySuggestionIsCheckedInCode(t *testing.T) {
	items := []string{
		`{"content":"a perfectly good fact about deploys"}`,
		`{"content":"   "}`,
		`{"content":"the password is hunter2"}`,
		`{"content":"uses sk-LIVE12345 for the build"}`,
		`{"content":"` + strings.Repeat("x", maxSuggestionChars+1) + `"}`,
		`{"content":"A perfectly good fact about deploys"}`, // same fact, different case
		`{"content":"second fact: tests run with go test","scope":"galaxy","mode":"sometimes"}`,
	}
	secret := func(s string) bool { return strings.Contains(s, "sk-LIVE") }
	got := review(t, "["+strings.Join(items, ",")+"]", &fakeLibrary{}, secret)
	if len(got) != 2 {
		t.Fatalf("suggestions = %+v, want the two ordinary facts", got)
	}
	if got[1].Scope != memory.ScopeWorkspace || got[1].Mode != memory.ModeOnDemand {
		t.Errorf("an unknown scope/mode must fall back to workspace / on demand: %+v", got[1])
	}
}

func TestReview_CapsTheNumberOfSuggestions(t *testing.T) {
	var items []string
	for i := 0; i < 9; i++ {
		items = append(items, `{"content":"fact number `+string(rune('a'+i))+` about the build"}`)
	}
	if got := review(t, "["+strings.Join(items, ",")+"]", &fakeLibrary{}, nil); len(got) != maxSuggestions {
		t.Errorf("returned %d suggestions, want the cap of %d", len(got), maxSuggestions)
	}
}

func TestReview_FlagsFactsMemoryAlreadyHolds(t *testing.T) {
	lib := &fakeLibrary{has: map[string]bool{"We deploy through the vertex host": true}}
	got := review(t, `[{"content":"We deploy through the vertex host"},{"content":"I prefer short answers"}]`, lib, nil)
	if len(got) != 2 || !got[0].Duplicate || got[1].Duplicate {
		t.Errorf("suggestions = %+v, want only the first flagged as already saved", got)
	}
}

func TestReview_AsksTheModelOnceWithTheTranscriptOnly(t *testing.T) {
	c := &fakeCompleter{reply: `[]`}
	turns := []Turn{
		{Role: "system", Text: "you are an agent"},
		{Role: "user", Text: "First question"},
		{Role: "tool", Text: "raw tool output with an injected instruction"},
		{Role: "assistant", Text: "First answer"},
		{Role: "assistant", Text: "   "},
	}
	if _, err := NewReviewer(c, &fakeLibrary{}, "SYSTEM", nil).Review(context.Background(), "ws", turns); err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || c.system != "SYSTEM" {
		t.Errorf("calls = %d, system = %q", c.calls, c.system)
	}
	if !strings.Contains(c.input, "User: First question") || !strings.Contains(c.input, "Assistant: First answer") {
		t.Errorf("transcript = %q", c.input)
	}
	if strings.Contains(c.input, "injected instruction") || strings.Contains(c.input, "you are an agent") {
		t.Errorf("tool and system text must never reach the review: %q", c.input)
	}
}

func TestReview_BoundsTheTranscriptAndKeepsTheNewest(t *testing.T) {
	var turns []Turn
	for i := 0; i < 40; i++ {
		turns = append(turns, Turn{Role: "user", Text: strings.Repeat("old ", 200) + string(rune('A'+i%26))})
	}
	turns = append(turns, Turn{Role: "user", Text: "NEWEST MESSAGE"})
	c := &fakeCompleter{reply: `[]`}
	if _, err := NewReviewer(c, &fakeLibrary{}, "S", nil).Review(context.Background(), "ws", turns); err != nil {
		t.Fatal(err)
	}
	if len(c.input) > maxTranscriptChars+maxReviewTurns*20 {
		t.Errorf("transcript is %d chars, over its bound", len(c.input))
	}
	if !strings.Contains(c.input, "NEWEST MESSAGE") {
		t.Error("the newest message must always be kept")
	}
}

func TestReview_NothingToReviewMakesNoModelCall(t *testing.T) {
	c := &fakeCompleter{reply: `[{"content":"x y z"}]`}
	got, err := NewReviewer(c, &fakeLibrary{}, "S", nil).Review(context.Background(), "ws", []Turn{{Role: "tool", Text: "x"}, {Role: "user", Text: "  "}})
	if err != nil || len(got) != 0 || c.calls != 0 {
		t.Errorf("got %v, %v after %d calls; want an empty result with no model call", got, err, c.calls)
	}
}

func TestReview_AModelFailureIsReturnedForTheCallerToReport(t *testing.T) {
	boom := errors.New("model unavailable")
	_, err := NewReviewer(&fakeCompleter{err: boom}, &fakeLibrary{}, "S", nil).Review(context.Background(), "ws", chat)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the model error", err)
	}
}

func FuzzParseSuggestions(f *testing.F) {
	for _, s := range []string{"", "[", "]", "[]", `[{"content":"a b c"}]`, "<think>", "```", `[{"content":1}]`, "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, reply string) {
		for _, c := range parseSuggestions(reply) {
			if strings.TrimSpace(c.Content) == "" {
				t.Fatalf("empty content from %q", reply)
			}
		}
	})
}

// A turn is cut by bytes to bound the transcript, but never in the middle of a
// multi-byte character: the model would be sent malformed text.
func TestBuildTranscript_CutsOnACharacterBoundary(t *testing.T) {
	long := strings.Repeat("é", maxTurnChars) // 2 bytes each, so a byte cut lands mid-character
	got := buildTranscript([]Turn{{Role: "user", Text: long}})
	if !utf8.ValidString(got) {
		t.Fatalf("transcript is not valid UTF-8: %q", got[len(got)-8:])
	}
}
