package app

import (
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

func servicesWithModels(cfgs ...models.ModelConfig) AppServices {
	return AppServices{Runtime: &mocks.MockManager{ListModelsFunc: func() []models.ModelConfig { return cfgs }}}
}

// A scheduled audit on a cloud model is refused before any model is woken; a local model, or a model this host cannot
// look up (the run itself will say so), passes. Other automations are never checked.
func TestAuditModelIsLocal(t *testing.T) {
	s := servicesWithModels(
		models.ModelConfig{Name: "qwen-local", WorkloadClass: models.WorkloadLocal},
		models.ModelConfig{Name: "deepseek-cloud", WorkloadClass: models.WorkloadCloud},
	)
	audit := func(model string) *automation.AutomationEntry {
		return &automation.AutomationEntry{Profile: netaudit.AuditorProfileID, Model: model}
	}

	if err := s.auditModelIsLocal(audit("qwen-local")); err != nil {
		t.Errorf("a local model passes: %v", err)
	}
	if err := s.auditModelIsLocal(audit("not-in-the-list")); err != nil {
		t.Errorf("an unknown model is left to fail in the run: %v", err)
	}
	if err := s.auditModelIsLocal(&automation.AutomationEntry{Profile: "task-runner", Model: "deepseek-cloud"}); err != nil {
		t.Errorf("only audits are checked: %v", err)
	}

	err := s.auditModelIsLocal(audit("deepseek-cloud"))
	var refusal *automation.PreflightError
	if !errors.As(err, &refusal) || refusal.Code != ModelNotLocalCode || !strings.Contains(refusal.Message, "deepseek-cloud") {
		t.Fatalf("a cloud model must be refused with a coded reason naming it, got %v", err)
	}
}
