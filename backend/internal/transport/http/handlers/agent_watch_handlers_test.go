package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/models"
)

type fixedAgentWatch struct {
	rec models.AgentWatch
	err error
}

func (f fixedAgentWatch) Record() (models.AgentWatch, error) { return f.rec, f.err }

func TestAgentWatchGet_NewestFirstWithDestinationsAndNoInternals(t *testing.T) {
	t0 := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	rec := models.AgentWatch{
		Events: []models.AgentWatchEvent{
			{At: t0, Kind: models.AgentToolRefused, Workspace: "ws", Automation: "nightly", Tool: "read_file"},
			{At: t0.Add(time.Hour), Kind: models.AgentNewDestination, Workspace: "ws", Automation: "weather", Host: "203.0.113.9"},
		},
		Destinations: map[string]models.AgentDestinations{"ws/weather": {Since: t0.Add(-10 * 24 * time.Hour), Hosts: []string{"a", "b"}}},
		Alerted:      map[string]time.Time{"ws/nightly/refused": t0},
		SecretsSeen:  map[string]time.Time{"ws/x|y": t0},
	}
	h := NewAgentWatchHandlers(fixedAgentWatch{rec: rec}, 7*24*time.Hour, func() time.Time { return t0 })
	w := httptest.NewRecorder()
	h.Get(w, httptest.NewRequest(http.MethodGet, "/admin/api/agent-watch", nil))
	var got agentWatchView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 2 || got.Events[0].Kind != "new_destination" {
		t.Fatalf("events = %+v; want newest first", got.Events)
	}
	if len(got.Automations) != 1 || got.Automations[0].Hosts != 2 || got.Automations[0].Learning || got.Automations[0].Automation != "weather" {
		t.Fatalf("automations = %+v", got.Automations)
	}
	var raw map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	if _, ok := raw["alerted"]; ok {
		t.Error("the alert bookkeeping is not shown")
	}
}

func TestAgentWatchGet_UnreadableRecordIsAnError(t *testing.T) {
	h := NewAgentWatchHandlers(fixedAgentWatch{err: errors.New("corrupt")}, time.Hour, time.Now)
	w := httptest.NewRecorder()
	h.Get(w, httptest.NewRequest(http.MethodGet, "/admin/api/agent-watch", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; a record that cannot be read is never shown as empty", w.Code)
	}
}
