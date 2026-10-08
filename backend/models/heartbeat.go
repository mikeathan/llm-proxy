// heartbeat.go — the per-workspace heartbeat: a frequent, cheap check that only speaks up when
// something needs attention. It compiles to an ordinary automation, so scheduling, lanes, history
// and delivery are unchanged (docs/PLANS/automation/memory-and-heartbeat-simplification.md).
package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	HeartbeatAutomationName = "heartbeat"
	HeartbeatDefaultEvery   = "30m"
	heartbeatStrategy       = "isolated"
	minHeartbeatEvery       = time.Minute
	maxHeartbeatEvery       = 24 * time.Hour
	htmlCommentOpen         = "<!--"
	htmlCommentClose        = "-->"
	codeFence               = "```"
)

// HeartbeatConfig is the workspace's heartbeat. Off by default; the checklist lives in heartbeat.md.
type HeartbeatConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	Every   string        `yaml:"every,omitempty" json:"every,omitempty"`
	Model   string        `yaml:"model,omitempty" json:"model,omitempty"`
	Notify  *NotifyConfig `yaml:"notify,omitempty" json:"notify,omitempty"`
	// ActiveHours ("HH:MM-HH:MM", server local time — the zone cron triggers use) limits scheduled
	// checks to a daily window; empty means always active.
	ActiveHours string `yaml:"active_hours,omitempty" json:"active_hours,omitempty"`
}

// HeartbeatResult is how the most recent heartbeat check ended.
type HeartbeatResult string

const (
	HeartbeatQuiet           HeartbeatResult = "quiet"
	HeartbeatAlert           HeartbeatResult = "alert"
	HeartbeatSkippedNoChecks HeartbeatResult = "skipped_no_checks"
	HeartbeatSkippedBusy     HeartbeatResult = "skipped_busy"
	// HeartbeatSkippedOutsideHours: a scheduled tick fell outside ActiveHours.
	HeartbeatSkippedOutsideHours HeartbeatResult = "skipped_outside_hours"
	HeartbeatError               HeartbeatResult = "error"
)

// HeartbeatStatus is the last check: when it happened and how it ended.
type HeartbeatStatus struct {
	At     time.Time       `json:"at"`
	Result HeartbeatResult `json:"result"`
}

// HeartbeatState is what the Heartbeat panel shows: the settings, the last check, which lane its model runs
// on (a local model is woken by every tick) and whether heartbeat.md holds any checks.
type HeartbeatState struct {
	Config          HeartbeatConfig  `json:"config"`
	Status          *HeartbeatStatus `json:"status,omitempty"`
	Lane            string           `json:"lane"`
	WakesLocalModel bool             `json:"wakes_local_model"`
	HasChecks       bool             `json:"has_checks"`
}

// Validate checks the interval (empty takes the default) and the active-hours window (empty is always active).
func (h HeartbeatConfig) Validate() error {
	if h.Every != "" {
		every, err := time.ParseDuration(h.Every)
		if err != nil || every < minHeartbeatEvery || every > maxHeartbeatEvery {
			return fmt.Errorf("invalid heartbeat.every %q: use a duration between 1m and 24h, such as 30m", h.Every)
		}
	}
	if h.ActiveHours != "" {
		if _, err := ParseDailyWindow(h.ActiveHours); err != nil {
			return fmt.Errorf("invalid heartbeat.active_hours: %w", err)
		}
	}
	return nil
}

// ActiveAt reports whether a scheduled check may run at t. A window that fails to parse (Validate rejects it on
// save) never silences the heartbeat: a monitor that quietly stops is worse than one that runs off-hours.
func (h HeartbeatConfig) ActiveAt(t time.Time) bool {
	if h.ActiveHours == "" {
		return true
	}
	window, err := ParseDailyWindow(h.ActiveHours)
	return err != nil || window.Contains(t)
}

// Automation compiles the heartbeat to the automation the dispatcher schedules. It never injects
// hot memory: a check that runs all day must not pay that cost on every tick.
func (h HeartbeatConfig) Automation() *Automation {
	every := h.Every
	if every == "" {
		every = HeartbeatDefaultEvery
	}
	return &Automation{
		Name:       HeartbeatAutomationName,
		Trigger:    TriggerConfig{Type: TriggerInterval, Value: every},
		TaskFile:   HeartbeatFilename,
		Strategy:   heartbeatStrategy,
		Model:      h.Model,
		Notify:     h.Notify,
		MemoryMode: MemoryModeOff,
	}
}

// HeartbeatBody is the checklist heartbeat.md actually holds: HTML comments are stripped (they explain the
// file to the operator), outside code fences, and the rest is trimmed. Empty means there is nothing to check.
func HeartbeatBody(content string) string {
	var out strings.Builder
	inFence, inComment := false, false
	for _, line := range strings.Split(content, "\n") {
		if !inComment && strings.HasPrefix(strings.TrimSpace(line), codeFence) {
			inFence = !inFence
		} else if !inFence {
			line = stripComments(line, &inComment)
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String())
}

// stripComments removes comment text from one line, tracking a comment that spans lines.
func stripComments(line string, inComment *bool) string {
	var kept strings.Builder
	for line != "" {
		if *inComment {
			end := strings.Index(line, htmlCommentClose)
			if end < 0 {
				return kept.String()
			}
			line = line[end+len(htmlCommentClose):]
			*inComment = false
			continue
		}
		start := strings.Index(line, htmlCommentOpen)
		if start < 0 {
			kept.WriteString(line)
			break
		}
		kept.WriteString(line[:start])
		line = line[start+len(htmlCommentOpen):]
		*inComment = true
	}
	return kept.String()
}
