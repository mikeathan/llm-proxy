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

// The audit's pre-flight and the timeline summary ask the same question, so they must give the same answer: the class the
// runtime resolved, or, for a model it has not resolved yet, the classifier's. A local model whose class is not filled in yet
// must not be refused as "in the cloud", and a model the runtime marked cloud must not be sent the summary.
func TestModelLocality_TheAuditAndTheSummaryAgree(t *testing.T) {
	s := servicesWithModels(
		models.ModelConfig{Name: "unresolved-gguf", Provider: "local", Filename: "qwen.gguf"},
		models.ModelConfig{Name: "resolved-cloud", Provider: "openai", WorkloadClass: models.WorkloadCloud},
		models.ModelConfig{Name: "resolved-local", Provider: "openai", WorkloadClass: models.WorkloadLocal},
	)
	audit := func(model string) *automation.AutomationEntry {
		return &automation.AutomationEntry{Profile: netaudit.AuditorProfileID, Model: model}
	}
	for name, local := range map[string]bool{"unresolved-gguf": true, "resolved-cloud": false, "resolved-local": true} {
		if got := s.modelIsLocal(name); got != local {
			t.Errorf("summary: modelIsLocal(%s) = %v, want %v", name, got, local)
		}
		if refused := s.auditModelIsLocal(audit(name)) != nil; refused == local {
			t.Errorf("audit: %s refused = %v, want %v", name, refused, !local)
		}
	}
}

// Every wired background loop of the Network module is started from the one place, including the counter of the router's refused
// attempts, whose timed write is what keeps a quiet log's counts from waiting for the next drop.
func TestNetauditLoops_StartsWhatIsWiredAndNothingElse(t *testing.T) {
	if loops := (AppServices{}).netauditLoops(); len(loops) != 0 {
		t.Fatalf("no services wired, %d loops", len(loops))
	}
	s := AppServices{netaudit: &netauditServices{Blocks: netaudit.NewBlocks(nil, nil)}}
	if loops := s.netauditLoops(); len(loops) != 1 {
		t.Fatalf("only the counter is wired, got %d loops", len(loops))
	}
}
