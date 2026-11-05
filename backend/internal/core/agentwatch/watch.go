// Package agentwatch keeps the agent security record: what the agents' runs were refused, what they asked approval for, where
// a stable automation fetched from for the first time, and credentials found in what agents wrote. It reads the agent events the
// event bus already carries, keeps vetted plain tokens only (a tool name, a host name), and alerts the operator in fixed words
// when an unattended run did something the safety rules stopped, or a stable automation reached a new host.
//
// It never stores arguments, a URL path or query, an error text, a model's words or a secret.
package agentwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

const (
	// queueSize bounds the events waiting for the loop; Observe drops (and counts) beyond it, so the agent loop never waits.
	queueSize = 256
	// maxEvents bounds the kept record; the oldest entries go first.
	maxEvents = 500
	// learnFor is how long an automation's destinations are learned before a new one is reported.
	learnFor = 7 * 24 * time.Hour
	// manyHosts marks an automation that reached more hosts than this while learning: it browses, so a new host says nothing.
	manyHosts = 8
	// maxHosts bounds the hosts kept per automation.
	maxHosts = 64
	// alertEvery is the shortest time between two alerts about the same automation.
	alertEvery = 24 * time.Hour
	// unknownTool stands for a tool name that is not a plain token (an MCP tool may be named by its server).
	unknownTool = "another tool"
)

var (
	plainTool = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	plainHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	plainName = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,64}$`)
)

// Store keeps the record. A missing file is an empty record.
type Store interface {
	Read() (models.AgentWatch, error)
	Write(models.AgentWatch) error
}

// Deps are what the watch needs. Notify sends an alert to the operator's connectors (through the alert log); OnError hears a
// record that could not be read or written. Now is the clock.
type Deps struct {
	Store   Store
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
	Now     func() time.Time
	// Workspaces lists the workspace folders the nightly secrets pass reads; nil turns the pass off.
	Workspaces func() ([]WorkspaceDir, error)
	// ActivityOn and SecretsPassOn are the operator's switches for the record and the secrets pass (the Agents stack's
	// modules). They are read on every event and every pass, never cached, and nil means on.
	ActivityOn    func() bool
	SecretsPassOn func() bool
}

// Watch is the agent security record and its alerts.
type Watch struct {
	deps    Deps
	queue   chan observed
	dropped atomic.Int64
}

type observed struct {
	workspace string
	event     assistant.AgentEvent
}

// New builds the watch. It does no I/O; Run reads and writes the record.
func New(deps Deps) *Watch {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Watch{deps: deps, queue: make(chan observed, queueSize)}
}

// Observe hands one published event to the watch. It never blocks: an event that does not fit is dropped and counted. Only the
// event types the record uses are queued.
func (w *Watch) Observe(workspaceID string, ev assistant.AgentEvent) {
	if !isOn(w.deps.ActivityOn) {
		return
	}
	switch ev.Type {
	case assistant.EventGuardrailViolation, assistant.EventGuardrailBlocked, assistant.EventToolCall:
	default:
		return
	}
	select {
	case w.queue <- observed{workspaceID, ev}:
	default:
		w.dropped.Add(1)
	}
}

// isOn reads a switch that is on when it is not wired.
func isOn(f func() bool) bool { return f == nil || f() }

// Dropped is how many events did not fit the queue since the program started.
func (w *Watch) Dropped() int64 { return w.dropped.Load() }

// Run handles queued events, and runs the secrets pass once a day, until ctx ends.
func (w *Watch) Run(ctx context.Context) {
	scan := time.NewTimer(firstScanAfter)
	defer scan.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case o := <-w.queue:
			w.handle(ctx, o.workspace, o.event)
		case <-scan.C:
			w.scanSecrets(ctx)
			scan.Reset(scanEvery)
		}
	}
}

// handle turns one event into at most one entry and one alert.
func (w *Watch) handle(ctx context.Context, workspace string, ev assistant.AgentEvent) {
	switch ev.Type {
	case assistant.EventGuardrailViolation:
		w.refused(ctx, workspace, ev)
	case assistant.EventGuardrailBlocked:
		if p, ok := ev.Payload.(assistant.GuardrailBlockedPayload); ok {
			w.update(ctx, func(r *models.AgentWatch, now time.Time) string {
				addEvent(r, models.AgentWatchEvent{At: now, Kind: models.AgentApprovalAsked, Workspace: workspace, Automation: automationOf(ev), Tool: toolName(p.Tool)})
				return ""
			})
		}
	case assistant.EventToolCall:
		w.fetched(ctx, workspace, ev)
	}
}

// refused records a refused tool call. An unattended run's first refusal of the day is alerted; a chat's is not, because the
// person is there and was told.
func (w *Watch) refused(ctx context.Context, workspace string, ev assistant.AgentEvent) {
	payload, _ := ev.Payload.(map[string]string)
	tool, automation := toolName(payload["tool"]), automationOf(ev)
	w.update(ctx, func(r *models.AgentWatch, now time.Time) string {
		addEvent(r, models.AgentWatchEvent{At: now, Kind: models.AgentToolRefused, Workspace: workspace, Automation: automation, Tool: tool})
		if ev.Channel != assistant.ChannelAutomation || automation == "" || !dueAlert(r, workspace+"/"+automation+"/refused", now) {
			return ""
		}
		return fmt.Sprintf("Agent watch: the automation %q in workspace %q tried a tool call the safety rules refused (%s). Open its last run to see what it was doing.",
			automation, plainOr(workspace), tool)
	})
}

// fetched learns where an automation fetches from and reports a new host after the learning week.
func (w *Watch) fetched(ctx context.Context, workspace string, ev assistant.AgentEvent) {
	automation, host := fetchOf(ev)
	if automation == "" || host == "" {
		return
	}
	key := workspace + "/" + automation
	w.update(ctx, func(r *models.AgentWatch, now time.Time) string {
		d, learning, added := learnHost(r, key, host, now)
		if !added || learning {
			return ""
		}
		addEvent(r, models.AgentWatchEvent{At: now, Kind: models.AgentNewDestination, Workspace: workspace, Automation: automation, Host: host})
		if d.Many || !dueAlert(r, key+"/destination", now) {
			return ""
		}
		return fmt.Sprintf("Agent watch: the automation %q in workspace %q fetched from %s, a host it has not used before. If you did not change its task, open its last run to see why.",
			automation, plainOr(workspace), host)
	})
}

// fetchOf is the automation and the host of a fetch_url call an automation made, or empty strings for anything else.
func fetchOf(ev assistant.AgentEvent) (automation, host string) {
	automation = automationOf(ev)
	if ev.Channel != assistant.ChannelAutomation || automation == "" {
		return "", ""
	}
	call, ok := ev.Payload.(models.ToolCall)
	if !ok || call.Function.Name != models.ToolNetworkFetch {
		return "", ""
	}
	return automation, hostOf(call.Function.Arguments)
}

// learnHost adds host to the automation's destinations. It reports the destinations after, whether the automation is still in
// its learning week, and whether the host was new.
func learnHost(r *models.AgentWatch, key, host string, now time.Time) (models.AgentDestinations, bool, bool) {
	if r.Destinations == nil {
		r.Destinations = map[string]models.AgentDestinations{}
	}
	d, seen := r.Destinations[key]
	if !seen {
		d.Since = now
	}
	learning := now.Sub(d.Since) < learnFor
	if slices.Contains(d.Hosts, host) {
		return d, learning, false
	}
	if len(d.Hosts) < maxHosts {
		d.Hosts = append(d.Hosts, host)
	}
	if learning && len(d.Hosts) > manyHosts {
		d.Many = true
	}
	r.Destinations[key] = d
	return d, learning, true
}

// update reads the record, applies change, writes it back, and sends the alert change returns (if any). A record that cannot be
// read is reported and nothing is recorded or alerted: an alert is only ever sent for an entry that was kept.
func (w *Watch) update(ctx context.Context, change func(r *models.AgentWatch, now time.Time) string) {
	rec, err := w.deps.Store.Read()
	if err != nil {
		w.report(fmt.Errorf("failed to read the agent security record: %w", err))
		return
	}
	before, _ := json.Marshal(rec)
	alert := change(&rec, w.deps.Now())
	if after, _ := json.Marshal(rec); string(after) != string(before) {
		if err := w.deps.Store.Write(rec); err != nil {
			w.report(fmt.Errorf("failed to save the agent security record: %w", err))
			return
		}
	}
	if alert != "" && w.deps.Notify != nil {
		if err := w.deps.Notify(ctx, alert); err != nil {
			w.report(fmt.Errorf("failed to send an agent watch alert: %w", err))
		}
	}
}

func (w *Watch) report(err error) {
	if w.deps.OnError != nil {
		w.deps.OnError(err)
	}
}

// addEvent appends an entry and keeps the newest maxEvents.
func addEvent(r *models.AgentWatch, e models.AgentWatchEvent) {
	r.Events = append(r.Events, e)
	if over := len(r.Events) - maxEvents; over > 0 {
		r.Events = slices.Delete(r.Events, 0, over)
	}
}

// dueAlert reports whether an alert for key may be sent now, and marks it sent.
func dueAlert(r *models.AgentWatch, key string, now time.Time) bool {
	if last, ok := r.Alerted[key]; ok && now.Sub(last) < alertEvery {
		return false
	}
	if r.Alerted == nil {
		r.Alerted = map[string]time.Time{}
	}
	r.Alerted[key] = now
	return true
}

// toolName keeps a tool's name when it is a plain token.
func toolName(name string) string {
	if plainTool.MatchString(name) {
		return name
	}
	return unknownTool
}

// automationOf is the event's automation name when it is a plain one.
func automationOf(ev assistant.AgentEvent) string {
	if plainName.MatchString(ev.Automation) {
		return ev.Automation
	}
	return ""
}

// plainOr returns a workspace id that is a plain name, or a fixed word.
func plainOr(workspace string) string {
	if plainName.MatchString(workspace) {
		return workspace
	}
	return "unnamed"
}

// hostOf is the lower-cased host name (or address) of a fetch_url call's url, or "" when the arguments hold no http(s) URL
// with a plain host.
func hostOf(arguments string) string {
	var args struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(arguments), &args) != nil {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !plainHost.MatchString(host) {
		return ""
	}
	return host
}

// Record reads the kept record, for the screen.
func (w *Watch) Record() (models.AgentWatch, error) { return w.deps.Store.Read() }

// LearnFor is how long an automation's destinations are learned before a new one is reported.
func LearnFor() time.Duration { return learnFor }
