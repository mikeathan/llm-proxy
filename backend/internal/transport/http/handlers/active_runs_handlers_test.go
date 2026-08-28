package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/models"
)

func TestActiveRunsHandler_AggregatesSubsystems(t *testing.T) {
	tests := []struct {
		name                     string
		assistantRunning         bool
		automationRunning        bool
		assistantConversationID  string
	}{
		{"neither running", false, false, ""},
		{"assistant only", true, false, "conv_123"},
		{"automation only", false, true, ""},
		{"both running", true, true, "conv_456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewActiveRunsHandler(
				func(string) bool { return tt.assistantRunning },
				func(string) bool { return tt.automationRunning },
				func(string) string { return tt.assistantConversationID },
			)

			req := httptest.NewRequest(http.MethodGet, "/admin/api/workspaces/ws1/active-runs", nil)
			req.SetPathValue(models.WorkspaceIDParam, "ws1")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}

			var got ActiveRunsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.AssistantRunning != tt.assistantRunning {
				t.Errorf("AssistantRunning = %v, want %v", got.AssistantRunning, tt.assistantRunning)
			}
			if got.AutomationRunning != tt.automationRunning {
				t.Errorf("AutomationRunning = %v, want %v", got.AutomationRunning, tt.automationRunning)
			}
			if got.AssistantConversationID != tt.assistantConversationID {
				t.Errorf("AssistantConversationID = %q, want %q", got.AssistantConversationID, tt.assistantConversationID)
			}
		})
	}
}

func TestActiveRunsHandler_MissingWorkspace(t *testing.T) {
	h := NewActiveRunsHandler(
		func(string) bool { return true },
		func(string) bool { return true },
		func(string) string { return "conv_123" },
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/workspaces//active-runs", nil)
	req.SetPathValue(models.WorkspaceIDParam, "")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace, got %d", rec.Code)
	}
}
