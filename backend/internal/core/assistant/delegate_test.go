package assistant

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// scriptedSubRunner answers every task with the result its function gives.
type scriptedSubRunner struct {
	enabled bool
	answer  func(SubRunRequest) SubRunResult
}

func (s *scriptedSubRunner) Enabled() bool { return s.enabled }
func (s *scriptedSubRunner) Run(_ context.Context, req SubRunRequest) SubRunResult {
	return s.answer(req)
}

func leadProfile() profile.Profile {
	return profile.Profile{ID: "lead", Delegates: []string{"web-researcher"}}
}

func delegateAgent(runner SubRunner, p profile.Profile) *Agent {
	return NewAgent(&MockClient{}, &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolInternetSearch}}}},
		&MockEngine{Result: "ok"}, AgentOptions{Logger: logging.NewNopLogger(), Profile: p, SubRunner: runner})
}

func offersDelegate(t *testing.T, a *Agent) bool {
	t.Helper()
	list, err := a.deps.Provider.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list {
		if tool.Function.Name == models.ToolDelegate {
			return true
		}
	}
	return false
}

// The delegate tool is offered only to a profile that names helpers, and only while delegation is set up.
func TestDelegateTool_Offered(t *testing.T) {
	on := &scriptedSubRunner{enabled: true}
	cases := []struct {
		name   string
		runner SubRunner
		p      profile.Profile
		want   bool
	}{
		{"a profile with helpers, delegation on", on, leadProfile(), true},
		{"a profile without helpers", on, profile.Profile{ID: "general"}, false},
		{"delegation off (no model named)", &scriptedSubRunner{enabled: false}, leadProfile(), false},
		{"no runner (a sub-agent itself)", nil, leadProfile(), false},
	}
	for _, tc := range cases {
		if got := offersDelegate(t, delegateAgent(tc.runner, tc.p)); got != tc.want {
			t.Errorf("%s: offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// helperEnum is the helpers the delegate tool's schema lists for the agent field of its tasks.
func helperEnum(t *testing.T, a *Agent) []string {
	t.Helper()
	list, err := a.deps.Provider.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list {
		if tool.Function.Name != models.ToolDelegate {
			continue
		}
		raw, _ := json.Marshal(tool.Function.Parameters)
		var schema struct {
			Properties struct {
				Tasks struct {
					Items struct {
						Properties struct {
							Agent struct {
								Enum []string `json:"enum"`
							} `json:"agent"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"tasks"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		return schema.Properties.Tasks.Items.Properties.Agent.Enum
	}
	t.Fatal("the delegate tool is not offered")
	return nil
}

// The schema names the helpers the run may use, so the model picks one instead of inventing a name: on 2026-10-22
// a local Qwen asked to delegate called its helper "general-purpose-reasoner". Each run's schema is its own copy.
func TestDelegateTool_SchemaListsTheRunsHelpers(t *testing.T) {
	runner := &scriptedSubRunner{enabled: true}
	two := delegateAgent(runner, profile.Profile{ID: "lead", Delegates: []string{"web-researcher", "reviewer"}})
	one := delegateAgent(runner, leadProfile())
	if got := helperEnum(t, two); !slices.Equal(got, []string{"web-researcher", "reviewer"}) {
		t.Errorf("two helpers: enum = %v", got)
	}
	if got := helperEnum(t, one); !slices.Equal(got, []string{"web-researcher"}) {
		t.Errorf("one helper: enum = %v (the schema was shared with the first run)", got)
	}
}

// A malformed call is an error the model can correct; nothing runs.
func TestRunDelegate_RejectsMalformedCalls(t *testing.T) {
	ran := 0
	runner := &scriptedSubRunner{enabled: true, answer: func(SubRunRequest) SubRunResult { ran++; return SubRunResult{Status: SubRunDone} }}
	task := `{"agent":"web-researcher","task":"q"}`
	cases := map[string]string{
		"no tasks":         `{"tasks":[]}`,
		"four tasks":       `{"tasks":[` + strings.Repeat(task+",", 3) + task + `]}`,
		"an unknown agent": `{"tasks":[{"agent":"operator","task":"q"}]}`,
		"an empty task":    `{"tasks":[{"agent":"web-researcher","task":"  "}]}`,
	}
	for name, args := range cases {
		a := delegateAgent(runner, leadProfile())
		if _, err := a.runDelegate(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolDelegate, Arguments: args}}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if ran != 0 {
		t.Errorf("%d tasks ran from malformed calls", ran)
	}
}

// One run delegates at most maxSubagentsPerRun tasks.
func TestRunDelegate_RunLimit(t *testing.T) {
	runner := &scriptedSubRunner{enabled: true, answer: func(SubRunRequest) SubRunResult { return SubRunResult{Status: SubRunDone, Summary: "ok"} }}
	a := delegateAgent(runner, leadProfile())
	three := proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolDelegate,
		Arguments: `{"tasks":[{"agent":"web-researcher","task":"a"},{"agent":"web-researcher","task":"b"},{"agent":"web-researcher","task":"c"}]}`}}
	for i := range maxSubagentsPerRun / maxDelegateTasks {
		if _, err := a.runDelegate(context.Background(), three); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if _, err := a.runDelegate(context.Background(), three); err == nil || !strings.Contains(err.Error(), "already delegated") {
		t.Errorf("a call past the run's limit: err = %v", err)
	}
}

// A task that could not run is part of the result, never an error: one busy or failed helper must not end the run.
func TestRunDelegate_UnfinishedTasksAreResults(t *testing.T) {
	statuses := map[string]SubRunResult{
		"busy":    {Status: SubRunBusy},
		"failed":  {Status: SubRunFailed, Error: "provider said 500"},
		"refused": {Status: SubRunRefused, Error: "the sub-agent model is not a cloud model"},
	}
	runner := &scriptedSubRunner{enabled: true, answer: func(req SubRunRequest) SubRunResult { return statuses[req.Task] }}
	a := delegateAgent(runner, leadProfile())
	out, err := a.runDelegate(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolDelegate,
		Arguments: `{"tasks":[{"agent":"web-researcher","task":"busy"},{"agent":"web-researcher","task":"failed"},{"agent":"web-researcher","task":"refused"}]}`}})
	if err != nil {
		t.Fatalf("err = %v, want none", err)
	}
	text := out.(string)
	for _, want := range []string{"slot was busy", "provider said 500", "not a cloud model"} {
		if !strings.Contains(text, want) {
			t.Errorf("result misses %q:\n%s", want, text)
		}
	}
}

// Each summary is cut to its share of the parent's result cap, measured as the parent measures it — the JSON-encoded
// result — so newlines, quotes and markup in a summary cannot push the whole past the cap and get it cut mid-sentence.
func TestRunDelegate_SummariesFitTheParentsCap(t *testing.T) {
	long := strings.Repeat("- A \"finding\" <b>with</b> its source https://example.com/x?a=1&b=2\n", 400)
	runner := &scriptedSubRunner{enabled: true, answer: func(SubRunRequest) SubRunResult { return SubRunResult{Status: SubRunDone, Summary: long} }}
	a := delegateAgent(runner, leadProfile())
	out, _ := a.runDelegate(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolDelegate,
		Arguments: `{"tasks":[{"agent":"web-researcher","task":"a"},{"agent":"web-researcher","task":"b"}]}`}})
	encoded, _ := json.Marshal(out)
	if len(encoded) > a.currentResultCap() {
		t.Errorf("encoded result is %d chars, over the parent's cap %d: the parent would cut it", len(encoded), a.currentResultCap())
	}
	if text := out.(string); strings.Count(text, "cut to fit") != 2 || !utf8.ValidString(text) {
		t.Errorf("both summaries should say they were cut, as valid text:\n%.300s", text)
	}
}

// A cut never splits a character.
func TestCutAtBoundary_KeepsWholeCharacters(t *testing.T) {
	text := strings.Repeat("é", 500)
	for limit := 50; limit < 120; limit++ {
		if got := cutAtBoundary(text, limit); !utf8.ValidString(got) || len(got) > limit {
			t.Fatalf("limit %d: %d bytes, valid %v", limit, len(got), utf8.ValidString(got))
		}
	}
}
