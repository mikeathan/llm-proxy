package handlers

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// agentWatchMaxEvents is how many entries the screen is sent, newest first.
const agentWatchMaxEvents = 100

// AgentWatchRecord is the agent security record: *agentwatch.Watch.
type AgentWatchRecord interface {
	Record() (models.AgentWatch, error)
}

// AgentWatchHandlers serve the agent security record to Settings, Safety.
type AgentWatchHandlers struct {
	rec      AgentWatchRecord
	learnFor time.Duration
	now      func() time.Time
}

func NewAgentWatchHandlers(rec AgentWatchRecord, learnFor time.Duration, now func() time.Time) *AgentWatchHandlers {
	return &AgentWatchHandlers{rec: rec, learnFor: learnFor, now: now}
}

type agentWatchEventView struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Workspace  string    `json:"workspace"`
	Automation string    `json:"automation,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	Host       string    `json:"host,omitempty"`
	Where      string    `json:"where,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

// agentWatchAutomationView is what the record learned of one automation's destinations.
type agentWatchAutomationView struct {
	Workspace  string `json:"workspace"`
	Automation string `json:"automation"`
	Hosts      int    `json:"hosts"`
	// Learning is true during the first week; LearningUntil says when it ends.
	Learning      bool      `json:"learning"`
	LearningUntil time.Time `json:"learning_until"`
	// Browses marks an automation that reached many hosts while learning: its new hosts are recorded, never alerted.
	Browses bool `json:"browses"`
}

type agentWatchView struct {
	Events      []agentWatchEventView      `json:"events"`
	Automations []agentWatchAutomationView `json:"automations"`
}

// Get handles GET /admin/api/agent-watch: the newest entries first, and what each automation's destinations look like. A record
// that cannot be read is an error, never an empty record.
func (h *AgentWatchHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	rec, err := h.rec.Record()
	if err != nil {
		logging.Warn("agent security record failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to read the agent security record")
		return
	}
	respondJSON(w, buildAgentWatchView(rec, h.learnFor, h.now()))
}

func buildAgentWatchView(rec models.AgentWatch, learnFor time.Duration, now time.Time) agentWatchView {
	v := agentWatchView{Events: []agentWatchEventView{}, Automations: []agentWatchAutomationView{}}
	for i := len(rec.Events) - 1; i >= 0 && len(v.Events) < agentWatchMaxEvents; i-- {
		e := rec.Events[i]
		v.Events = append(v.Events, agentWatchEventView{At: e.At, Kind: string(e.Kind), Workspace: e.Workspace, Automation: e.Automation,
			Tool: e.Tool, Host: e.Host, Where: e.Where, Detail: e.Detail})
	}
	for key, d := range rec.Destinations {
		workspace, automation, _ := strings.Cut(key, "/")
		until := d.Since.Add(learnFor)
		v.Automations = append(v.Automations, agentWatchAutomationView{Workspace: workspace, Automation: automation, Hosts: len(d.Hosts),
			Learning: now.Before(until), LearningUntil: until, Browses: d.Many})
	}
	slices.SortFunc(v.Automations, func(a, b agentWatchAutomationView) int {
		return strings.Compare(a.Workspace+"/"+a.Automation, b.Workspace+"/"+b.Automation)
	})
	return v
}
