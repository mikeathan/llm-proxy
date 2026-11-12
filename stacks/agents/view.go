package agents

import (
	"slices"
	"strings"
	"time"
)

// maxViewEvents is how many entries the screen is sent, newest first.
const maxViewEvents = 100

type eventView struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Workspace  string    `json:"workspace"`
	Automation string    `json:"automation,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	Host       string    `json:"host,omitempty"`
	Where      string    `json:"where,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

// automationView is what the record learned of one automation's destinations.
type automationView struct {
	Workspace  string `json:"workspace"`
	Automation string `json:"automation"`
	Hosts      int    `json:"hosts"`
	// Learning is true during the first week; LearningUntil says when it ends.
	Learning      bool      `json:"learning"`
	LearningUntil time.Time `json:"learning_until"`
	// Browses marks an automation that reached many hosts while learning: its new hosts are recorded, never alerted.
	Browses bool `json:"browses"`
}

type view struct {
	Events      []eventView      `json:"events"`
	Automations []automationView `json:"automations"`
}

func buildView(rec Record, learnFor time.Duration, now time.Time) view {
	v := view{Events: []eventView{}, Automations: []automationView{}}
	for i := len(rec.Events) - 1; i >= 0 && len(v.Events) < maxViewEvents; i-- {
		e := rec.Events[i]
		v.Events = append(v.Events, eventView{At: e.At, Kind: string(e.Kind), Workspace: e.Workspace, Automation: e.Automation,
			Tool: e.Tool, Host: e.Host, Where: e.Where, Detail: e.Detail})
	}
	for key, d := range rec.Destinations {
		workspace, automation, _ := strings.Cut(key, "/")
		until := d.Since.Add(learnFor)
		v.Automations = append(v.Automations, automationView{Workspace: workspace, Automation: automation, Hosts: len(d.Hosts),
			Learning: now.Before(until), LearningUntil: until, Browses: d.Many})
	}
	slices.SortFunc(v.Automations, func(a, b automationView) int {
		return strings.Compare(a.Workspace+"/"+a.Automation, b.Workspace+"/"+b.Automation)
	})
	return v
}
