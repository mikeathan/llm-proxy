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
func (enabledRunner) Run(context.Context, assistant.SubRunRequest) assistant.SubRunResult {
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
