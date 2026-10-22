package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

type fakeHelpers struct {
	runner   assistant.SubRunner
	settings *models.AgentConfig
}

func (f fakeHelpers) SubRunner() assistant.SubRunner     { return f.runner }
func (f fakeHelpers) AgentSettings() *models.AgentConfig { return f.settings }

type enabledRunner bool

func (e enabledRunner) Enabled() bool { return bool(e) }
func (e enabledRunner) Problem() assistant.HelperProblem {
	if e {
		return assistant.HelperNone
	}
	return assistant.HelperNotCloud
}
func (enabledRunner) Run(context.Context, assistant.SubRunRequest) assistant.SubRunResult {
	return assistant.SubRunResult{}
}

// problemRunner is a runner that cannot run for the given reason (HelperNone: it can).
type problemRunner assistant.HelperProblem

func (p problemRunner) Enabled() bool                    { return assistant.HelperProblem(p) == assistant.HelperNone }
func (p problemRunner) Problem() assistant.HelperProblem { return assistant.HelperProblem(p) }
func (problemRunner) Run(context.Context, assistant.SubRunRequest) assistant.SubRunResult {
	return assistant.SubRunResult{}
}

// The page asks the server whether helpers can run and on which model, instead of working it out from settings.
func TestHelpersHandlers_Status(t *testing.T) {
	cases := []struct {
		name      string
		svc       fakeHelpers
		available bool
		model     string
	}{
		{"set up", fakeHelpers{enabledRunner(true), &models.AgentConfig{SubagentModel: "deepseek-v4.1-flash"}}, true, "deepseek-v4.1-flash"},
		{"a model named that is not a cloud model", fakeHelpers{enabledRunner(false), &models.AgentConfig{SubagentModel: "qwen"}}, false, "qwen"},
		{"no model named", fakeHelpers{enabledRunner(false), nil}, false, ""},
		{"no runner wired", fakeHelpers{nil, &models.AgentConfig{SubagentModel: "deepseek-v4.1-flash"}}, false, "deepseek-v4.1-flash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHelpersHandlers(tc.svc).Status(rec, httptest.NewRequest(http.MethodGet, "/admin/api/helpers", nil))
			var got struct {
				Available bool   `json:"available"`
				Model     string `json:"model"`
			}
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if got.Available != tc.available || got.Model != tc.model {
				t.Errorf("got %+v, want available=%v model=%q", got, tc.available, tc.model)
			}
		})
	}
}

// The status says why helpers cannot run, so the page can name the fix: the Helper model was removed, is not a cloud
// model, or was never set. A removed model still answers with its name, so the page can say which one is gone.
func TestHelpersHandlers_StatusSaysWhy(t *testing.T) {
	cases := []struct {
		name    string
		runner  assistant.SubRunner
		model   string
		problem string
	}{
		{"set up", problemRunner(assistant.HelperNone), "deepseek-v4.1-flash", ""},
		{"removed", problemRunner(assistant.HelperMissing), "gone-m", "missing"},
		{"not a cloud model", problemRunner(assistant.HelperNotCloud), "qwen", "not_cloud"},
		{"unset", problemRunner(assistant.HelperUnset), "", "unset"},
		{"no runner wired", nil, "", "unset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHelpersHandlers(fakeHelpers{tc.runner, &models.AgentConfig{SubagentModel: tc.model}}).Status(rec, httptest.NewRequest(http.MethodGet, "/admin/api/helpers", nil))
			var got struct {
				Available bool   `json:"available"`
				Model     string `json:"model"`
				Problem   string `json:"problem"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Problem != tc.problem || got.Available != (tc.problem == "") || got.Model != tc.model {
				t.Errorf("got %+v, want problem %q model %q", got, tc.problem, tc.model)
			}
		})
	}
}
