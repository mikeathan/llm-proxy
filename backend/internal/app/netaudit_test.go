package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

type memAlertStore struct{ items []models.NetworkAlert }

func (m *memAlertStore) Read() ([]models.NetworkAlert, error) { return m.items, nil }
func (m *memAlertStore) Write(v []models.NetworkAlert) error  { m.items = v; return nil }

// An alert reaches the log even when no connector exists, and says so: the web UI is then the only place it shows.
func TestAlertNotifier_LogsTheAlertWhenNoConnectorIsSetUp(t *testing.T) {
	store := &memAlertStore{}
	s := &AppServices{}
	err := s.alertNotifier(netaudit.NewAlertLog(store, time.Now, nil), models.NetworkAlertPresence)(context.Background(), "Network audit: 1 new device.")
	if !errors.Is(err, netaudit.ErrNoConnector) {
		t.Fatalf("the alert must stay pending, got %v", err)
	}
	if len(store.items) != 1 || store.items[0].Delivery != models.NetworkAlertNoConnector || store.items[0].Kind != models.NetworkAlertPresence {
		t.Fatalf("got %+v", store.items)
	}
}
