package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

func TestActiveRunsHandler_AggregatesSubsystems(t *testing.T) {
	tests := []struct {
		name                    string
		assistantRunning        bool
		automationRunning       bool
		assistantConversationID string
		assistantQueued         bool
	}{
		{"neither running", false, false, "", false},
		{"assistant only", true, false, "conv_123", false},
		{"automation only", false, true, "", false},
		{"both running", true, true, "conv_456", false},
		{"assistant waiting for the lane", true, false, "conv_789", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewActiveRunsHandler(ActiveRunsSources{
				AssistantRunning:        func(string) bool { return tt.assistantRunning },
				AutomationRunning:       func(string) bool { return tt.automationRunning },
				AssistantConversationID: func(string) string { return tt.assistantConversationID },
				AssistantQueued:         func(string) bool { return tt.assistantQueued },
			})

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
			if got.AssistantQueued != tt.assistantQueued {
				t.Errorf("AssistantQueued = %v, want %v", got.AssistantQueued, tt.assistantQueued)
			}
		})
	}
}

// laneSnapshotFixture returns a two-lane snapshot with one holder and two queued
// entries for the global lane-state tests.
func laneSnapshotFixture() runlane.Snapshot {
	return runlane.Snapshot{Lanes: []runlane.LaneState{
		{
			Lane:    runlane.LaneLocal,
			Limit:   1,
			Running: 1,
			Holders: []runlane.Holder{{Key: "ws/a", Kind: runlane.KindAutomation, WorkspaceID: "ws", Automation: "a", Label: "ws/a"}},
			Queued:  []runlane.Entry{{Key: "ws/b", Lane: runlane.LaneLocal, WorkspaceID: "ws", Automation: "b", Position: 1}},
		},
		{
			Lane:   runlane.LaneCloud,
			Limit:  3,
			Queued: []runlane.Entry{{Key: "ws/c", Lane: runlane.LaneCloud, WorkspaceID: "ws", Automation: "c", Position: 2}},
		},
	}}
}

func TestActiveRunsHandler_GlobalLaneState(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{
		LaneSnapshot: func() runlane.Snapshot { return laneSnapshotFixture() },
	})

	// The global route takes no workspace path parameter, so nothing is set
	// here — that is the point of the endpoint.
	req := httptest.NewRequest(http.MethodGet, "/admin/api/active-runs", nil)
	rec := httptest.NewRecorder()

	h.ServeGlobalHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got GlobalActiveRunsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.LaneHolders) != 1 || got.LaneHolders[0].Key != "ws/a" {
		t.Fatalf("holders = %+v, want the single automation holder", got.LaneHolders)
	}
	if len(got.Queued) != 2 {
		t.Fatalf("queued = %+v, want both lanes flattened", got.Queued)
	}
}

func TestActiveRunsHandler_GlobalWithoutSnapshotSource(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/active-runs", nil)
	rec := httptest.NewRecorder()

	h.ServeGlobalHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got GlobalActiveRunsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.LaneHolders != nil || got.Queued != nil {
		t.Errorf("lane fields = %+v/%+v, want empty without a snapshot source", got.LaneHolders, got.Queued)
	}
}

func TestActiveRunsHandler_MissingWorkspace(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{
		AssistantRunning:        func(string) bool { return true },
		AutomationRunning:       func(string) bool { return true },
		AssistantConversationID: func(string) string { return "conv_123" },
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/workspaces//active-runs", nil)
	req.SetPathValue(models.WorkspaceIDParam, "")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace, got %d", rec.Code)
	}
}

func queueAction(t *testing.T, h *ActiveRunsHandler, serve func(http.ResponseWriter, *http.Request), key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/queue/"+key+"/x", nil)
	req.SetPathValue(QueueKeyParam, key)
	rec := httptest.NewRecorder()
	serve(rec, req)
	return rec
}

func TestActiveRunsHandler_PromoteServesAQueuedCaller(t *testing.T) {
	var promoted string
	h := NewActiveRunsHandler(ActiveRunsSources{
		PromoteModelWait: func(key string) error { promoted = key; return nil },
	})

	rec := queueAction(t, h, h.ServePromoteHTTP, "inbound:7")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if promoted != "inbound:7" {
		t.Fatalf("promoted %q, want inbound:7", promoted)
	}
}

func TestActiveRunsHandler_PromoteReportsAnUnknownEntry(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{
		PromoteModelWait: func(string) error { return runlane.ErrUnknownWaiter },
	})

	rec := queueAction(t, h, h.ServePromoteHTTP, "inbound:gone")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (the entry may have been served or expired)", rec.Code)
	}
}

func TestActiveRunsHandler_CancelDropsAQueuedCaller(t *testing.T) {
	var cancelled string
	h := NewActiveRunsHandler(ActiveRunsSources{
		CancelModelWait: func(key string) bool { cancelled = key; return true },
	})

	rec := queueAction(t, h, h.ServeCancelHTTP, "inbound:3")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if cancelled != "inbound:3" {
		t.Fatalf("cancelled %q, want inbound:3", cancelled)
	}

	// A second press finds nothing: the caller is already gone.
	h.sources.CancelModelWait = func(string) bool { return false }
	rec = queueAction(t, h, h.ServeCancelHTTP, "inbound:3")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 on a repeat cancel", rec.Code)
	}
}

func TestActiveRunsHandler_QueueActionsAreUnavailableWithoutALane(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{})

	for name, serve := range map[string]func(http.ResponseWriter, *http.Request){
		"promote": h.ServePromoteHTTP,
		"cancel":  h.ServeCancelHTTP,
	} {
		rec := queueAction(t, h, serve, "inbound:1")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503 when no scheduler is wired", name, rec.Code)
		}
	}
}

func TestActiveRunsHandler_QueueActionNeedsAKey(t *testing.T) {
	h := NewActiveRunsHandler(ActiveRunsSources{
		PromoteModelWait: func(string) error { return errors.New("must not be called") },
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/queue//promote", nil)
	req.SetPathValue(QueueKeyParam, "")
	rec := httptest.NewRecorder()
	h.ServePromoteHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing key", rec.Code)
	}
}

// The lane snapshot is consumed by the frontend's LaneHolder / QueuedRun types.
// Assert the raw JSON keys: a Go struct without tags emits its Go field names
// (WorkspaceID, Since), which the UI reads as undefined — a silent break the
// Go-side decode tests cannot catch.
func TestActiveRunsHandler_GlobalLaneStateWireKeys(t *testing.T) {
	since := time.Now()
	h := NewActiveRunsHandler(ActiveRunsSources{
		LaneSnapshot: func() runlane.Snapshot {
			return runlane.Snapshot{
				Lanes: []runlane.LaneState{{
					Lane:    runlane.LaneLocal,
					Limit:   1,
					Running: 1,
					Holders: []runlane.Holder{{
						Key: "ws/a", Kind: runlane.KindAutomation, WorkspaceID: "ws",
						Automation: "a", Label: "ws/a", Model: "m", Since: since,
					}},
					Queued: []runlane.Entry{{
						Key: "ws/b", Kind: runlane.KindAutomation, Lane: runlane.LaneLocal,
						WorkspaceID: "ws", Automation: "b", Label: "ws/b", Model: "m",
						Position: 1, QueuedAt: since,
					}},
				}},
				ModelHolders: []runlane.Holder{{
					Key: "inbound:2", Kind: runlane.KindInbound, Label: "c", Model: "c", Since: since,
				}},
				ModelWaiters: []runlane.Entry{{
					Key: "inbound:1", Kind: runlane.KindInbound, Lane: runlane.LaneLocal,
					Label: "b", Model: "b", Position: 1, QueuedAt: since,
				}},
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/active-runs", nil)
	rec := httptest.NewRecorder()
	h.ServeGlobalHTTP(rec, req)

	var raw struct {
		LaneHolders []map[string]any `json:"lane_holders"`
		Queued      []map[string]any `json:"queued"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.LaneHolders) != 2 || len(raw.Queued) != 2 {
		t.Fatalf("lane_holders = %d, queued = %d, want the lane holder + the served inbound caller, and the lane queue + the waiting inbound caller",
			len(raw.LaneHolders), len(raw.Queued))
	}

	for _, holder := range raw.LaneHolders {
		for _, key := range []string{"key", "kind", "label", "model", "since"} {
			if _, ok := holder[key]; !ok {
				t.Errorf("lane holder JSON is missing %q: %v", key, holder)
			}
		}
	}
	// Every queued row (lane queue and inbound waiter alike) carries the same
	// keys — including the inbound caller's identity, which the operator acts on.
	for _, row := range raw.Queued {
		for _, key := range []string{"key", "kind", "lane", "label", "position", "queued_at"} {
			if _, ok := row[key]; !ok {
				t.Errorf("queued entry JSON is missing %q: %v", key, row)
			}
		}
	}
}
