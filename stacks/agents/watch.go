package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"llm-proxy-stacks/sdk"
)

const (
	// File is the record's file in the data folder.
	File = "agent-watch.json"

	// queueSize bounds the observations waiting for the loop; Observe drops (and counts) beyond it, so the agent loop never waits.
	queueSize = 256
	// maxEvents bounds the kept record; the oldest entries go first.
	maxEvents = 500
	// LearnFor is how long an automation's destinations are learned before a new one is reported.
	LearnFor = 7 * 24 * time.Hour
	// manyHosts marks an automation that reached more hosts than this while learning: it browses, so a new host says nothing.
	manyHosts = 8
	// maxHosts bounds the hosts kept per automation.
	maxHosts = 64
	// alertEvery is the shortest time between two alerts about the same automation.
	alertEvery = 24 * time.Hour
	// maxSeenSecrets bounds the remembered findings, so a secret is reported once.
	maxSeenSecrets = 1000
	// scanEvery is how often the secrets pass runs; firstScanAfter is the wait after start, so it does not run during startup.
	scanEvery      = 24 * time.Hour
	firstScanAfter = 10 * time.Minute
)

// The ids of the alerts the stack publishes. The core words each one from its own table and drops an id it does not know.
const (
	AlertRefused     = "agent_tool_refused"
	AlertDestination = "agent_new_destination"
	AlertSecrets     = "agent_secrets_found"
)

// AlertIDs lists every alert id the stack can publish, so a test in the service can hold the core's table of words to it.
func AlertIDs() []string { return []string{AlertRefused, AlertDestination, AlertSecrets} }

// fileStore keeps the record in one file through the core's storage port. A missing file is an empty record; a file that cannot be read as
// a record is set aside and reported, and the next read starts empty.
type fileStore struct{ dir stacksdk.DataDir }

func (s fileStore) read() (Record, error) {
	data, err := s.dir.Read(File)
	if stacksdk.CodeOf(err) == stacksdk.CodeNotFound {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("failed to read the agent security record: %w", err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		err = fmt.Errorf("failed to decode the agent security record: %w", err)
		if qerr := s.dir.Quarantine(File); qerr != nil {
			err = errors.Join(err, qerr)
		}
		return Record{}, err
	}
	return rec, nil
}

func (s fileStore) write(rec Record) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the agent security record: %w", err)
	}
	if err := s.dir.Write(File, data); err != nil {
		return fmt.Errorf("failed to save the agent security record: %w", err)
	}
	return nil
}

// settings are the two switches the operator keeps for the stack. Both parts have always run, so each is a negative flag: a document with
// neither key keeps them on. The keys are the core's `agents` settings document, unchanged.
type settings struct {
	ActivityOff    bool `json:"activity_off"`
	SecretsPassOff bool `json:"secrets_pass_off"`
}

// watch is the agent security record and its alerts.
type watch struct {
	store   fileStore
	config  stacksdk.Config
	scan    stacksdk.SecretScan
	events  stacksdk.Events
	log     stacksdk.Logger
	now     func() time.Time
	queue   chan stacksdk.Observation
	dropped atomic.Int64
	lastRun atomic.Pointer[time.Time]
}

// newWatch builds the watch with its queue and nothing else, so an observation that arrives before the stack starts waits in the queue (the
// service attaches the bus before it starts the loops) and is handled once the loop runs. bind gives it the host's ports.
func newWatch(now func() time.Time) *watch {
	return &watch{now: now, queue: make(chan stacksdk.Observation, queueSize)}
}

// bind takes the host's ports. It is called once, by Start, before the loop runs.
func (w *watch) bind(h stacksdk.Host) {
	w.store, w.config, w.scan, w.events, w.log = fileStore{dir: h.DataDir}, h.Config, h.SecretScan, h.Events, h.Log
}

// settings reads the switches afresh. A document that cannot be read leaves both parts on: the record is never silenced by a failure.
func (w *watch) settings(ctx context.Context) settings {
	var s settings
	if doc, err := w.config.Load(ctx); err == nil {
		_ = json.Unmarshal(doc, &s)
	}
	return s
}

// observe hands one observation to the loop. It never blocks: one that does not fit is dropped and counted. Only the kinds the record uses
// are queued.
func (w *watch) observe(o stacksdk.Observation) {
	switch o.Kind {
	case stacksdk.ObservedRefused, stacksdk.ObservedApproval, stacksdk.ObservedFetch:
	default:
		return
	}
	select {
	case w.queue <- o:
	default:
		w.dropped.Add(1)
	}
}

// run handles queued observations, and runs the secrets pass once a day, until ctx ends.
func (w *watch) run(ctx context.Context) {
	scan := time.NewTimer(firstScanAfter)
	defer scan.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case o := <-w.queue:
			w.handle(ctx, o)
		case <-scan.C:
			w.scanSecrets(ctx)
			scan.Reset(scanEvery)
		}
	}
}

// handle turns one observation into at most one entry and one alert.
func (w *watch) handle(ctx context.Context, o stacksdk.Observation) {
	switch o.Kind {
	case stacksdk.ObservedRefused:
		w.refused(ctx, o)
	case stacksdk.ObservedApproval:
		w.update(ctx, func(r *Record, now time.Time) *stacksdk.Alert {
			addEvent(r, Event{At: now, Kind: KindApprovalAsked, Workspace: o.Workspace, Automation: o.Automation, Tool: o.Tool})
			return nil
		})
	case stacksdk.ObservedFetch:
		w.fetched(ctx, o)
	}
}

// refused records a refused tool call. An unattended run's first refusal of the day is alerted; a chat's is not, because the person is there
// and was told.
func (w *watch) refused(ctx context.Context, o stacksdk.Observation) {
	w.update(ctx, func(r *Record, now time.Time) *stacksdk.Alert {
		addEvent(r, Event{At: now, Kind: KindToolRefused, Workspace: o.Workspace, Automation: o.Automation, Tool: o.Tool})
		if !o.Unattended || o.Automation == "" || !dueAlert(r, o.Workspace+"/"+o.Automation+"/refused", now) {
			return nil
		}
		return &stacksdk.Alert{ID: AlertRefused, Workspace: o.Workspace, Automation: o.Automation, Tool: o.Tool}
	})
}

// fetched learns where an automation fetches from and reports a new host after the learning week.
func (w *watch) fetched(ctx context.Context, o stacksdk.Observation) {
	if o.Automation == "" || o.Host == "" {
		return
	}
	key := o.Workspace + "/" + o.Automation
	w.update(ctx, func(r *Record, now time.Time) *stacksdk.Alert {
		d, learning, added := learnHost(r, key, o.Host, now)
		if !added || learning {
			return nil
		}
		addEvent(r, Event{At: now, Kind: KindNewDestination, Workspace: o.Workspace, Automation: o.Automation, Host: o.Host})
		if d.Many || !dueAlert(r, key+"/destination", now) {
			return nil
		}
		return &stacksdk.Alert{ID: AlertDestination, Workspace: o.Workspace, Automation: o.Automation, Host: o.Host}
	})
}

// learnHost adds host to the automation's destinations. It reports the destinations after, whether the automation is still in its learning
// week, and whether the host was new.
func learnHost(r *Record, key, host string, now time.Time) (Destinations, bool, bool) {
	if r.Destinations == nil {
		r.Destinations = map[string]Destinations{}
	}
	d, seen := r.Destinations[key]
	if !seen {
		d.Since = now
	}
	learning := now.Sub(d.Since) < LearnFor
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

// update reads the record, applies change, writes it back, and publishes the alert change returns (if any). A record that cannot be read is
// reported and nothing is recorded or alerted: an alert is only ever sent for an entry that was kept.
func (w *watch) update(ctx context.Context, change func(r *Record, now time.Time) *stacksdk.Alert) {
	rec, err := w.store.read()
	if err != nil {
		w.report(err)
		return
	}
	before, _ := json.Marshal(rec)
	alert := change(&rec, w.now())
	if after, _ := json.Marshal(rec); string(after) != string(before) {
		if err := w.store.write(rec); err != nil {
			w.report(err)
			return
		}
	}
	if alert != nil {
		w.publish(ctx, *alert)
	}
}

// publish sends an alert to the core, which words it and tells the operator.
func (w *watch) publish(ctx context.Context, a stacksdk.Alert) {
	data, err := json.Marshal(a)
	if err != nil {
		w.report(fmt.Errorf("failed to encode an agent watch alert: %w", err))
		return
	}
	if err := w.events.Publish(ctx, stacksdk.Event{Type: stacksdk.EventAlert, Subject: a.ID, Data: data, At: w.now()}); err != nil {
		w.report(fmt.Errorf("failed to send an agent watch alert: %w", err))
	}
}

func (w *watch) report(err error) {
	w.log.Log(stacksdk.LevelWarn, "agent watch", "error", err.Error())
}

// addEvent appends an entry and keeps the newest maxEvents.
func addEvent(r *Record, e Event) {
	r.Events = append(r.Events, e)
	if over := len(r.Events) - maxEvents; over > 0 {
		r.Events = slices.Delete(r.Events, 0, over)
	}
}

// dueAlert reports whether an alert for key may be sent now, and marks it sent.
func dueAlert(r *Record, key string, now time.Time) bool {
	if last, ok := r.Alerted[key]; ok && now.Sub(last) < alertEvery {
		return false
	}
	if r.Alerted == nil {
		r.Alerted = map[string]time.Time{}
	}
	r.Alerted[key] = now
	return true
}
