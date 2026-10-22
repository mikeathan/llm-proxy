package assistant

import (
	"context"
	"strings"
	"testing"

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

// Each summary is cut to its share of the parent's result cap, so the whole result arrives intact.
func TestRunDelegate_SummariesFitTheParentsCap(t *testing.T) {
	long := strings.Repeat("A finding with its source https://example.com/x. ", 400)
	runner := &scriptedSubRunner{enabled: true, answer: func(SubRunRequest) SubRunResult { return SubRunResult{Status: SubRunDone, Summary: long} }}
	a := delegateAgent(runner, leadProfile())
	out, _ := a.runDelegate(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: models.ToolDelegate,
		Arguments: `{"tasks":[{"agent":"web-researcher","task":"a"},{"agent":"web-researcher","task":"b"}]}`}})
	text := out.(string)
	if len(text) > a.currentResultCap()+len("\n\n") {
		t.Errorf("result is %d chars, over the parent's cap %d", len(text), a.currentResultCap())
	}
	if strings.Count(text, "cut to fit") != 2 {
		t.Errorf("both summaries should say they were cut:\n%s", text[:200])
	}
}
