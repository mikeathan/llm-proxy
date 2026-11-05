package netaudit

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// CheckMonitorStalled is a part of the Network module that is switched on and has not worked for longer than it should: the
// presence check, the router's log, the DNS comparison, the decoys or the nightly audit. "Silence is never absence" holds for
// the module itself too: a monitor that went quiet must say so, because a quiet page is what an intruder who silences the
// monitoring would cause.
const CheckMonitorStalled = "monitor_stalled"

// The parts the watch knows. Each ID is fixed; the words that name a part in a message are fixed too.
const (
	MonitorPresence       = "presence"
	MonitorRouterLog      = "router_log"
	MonitorRouterLogQuiet = "router_log_quiet"
	MonitorDNS            = "dns_check"
	MonitorDecoys         = "decoys"
	MonitorAudit          = "audit"
)

// monitorEvery is how often the watch looks at the parts. It reads statuses held in memory and one small file; it probes nothing.
const monitorEvery = 5 * time.Minute

// MonitorReading is what one part says about itself at one moment: whether the operator has it switched on, when it last did
// its work, and how long it may go without working (read each time, because a part's interval is a setting). A part with no
// clock of its own (the decoys, the router log's socket) reports the moment of the reading while it is healthy, and a zero
// time while it is not.
type MonitorReading struct {
	On       bool
	LastGood time.Time
	Limit    time.Duration
}

// MonitorPart is one watched part: a fixed ID, the fixed words that name it, and how it is read.
type MonitorPart struct {
	ID    string
	Words string
	Read  func() MonitorReading
}

// MonitorStall is a part that has stopped: its ID, its words, and the last moment it was known to work (or the moment it was
// first seen switched on, when it never worked since).
type MonitorStall struct {
	ID    string    `json:"id"`
	Words string    `json:"words"`
	Since time.Time `json:"since"`
}

// MonitorWatchDeps are what the watch needs; the application wires them in one place.
type MonitorWatchDeps struct {
	Parts  []MonitorPart
	Notify func(ctx context.Context, message string) error
	Now    func() time.Time
	// Every is how often Run ticks (monitorEvery when zero).
	Every time.Duration
}

// MonitorWatch watches the module's own parts and alerts once when a switched-on part stops working, and again only after it
// worked in between. It keeps everything in memory: after a restart each part gets its full limit again, so a part that was
// already stalled is told once more at most.
type MonitorWatch struct {
	deps MonitorWatchDeps

	mu      sync.Mutex
	good    map[string]time.Time // the latest moment each part was known to work, or first seen on
	stalled map[string]MonitorStall
	told    map[string]bool
}

func NewMonitorWatch(deps MonitorWatchDeps) *MonitorWatch {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Every <= 0 {
		deps.Every = monitorEvery
	}
	return &MonitorWatch{deps: deps, good: map[string]time.Time{}, stalled: map[string]MonitorStall{}, told: map[string]bool{}}
}

// Run ticks until ctx ends. It is started with the application, from the one place the Network loops start (Constitution II.2,
// II.14).
func (w *MonitorWatch) Run(ctx context.Context) {
	ticker := time.NewTicker(w.deps.Every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Tick(ctx)
		}
	}
}

// Tick reads every part once and sends one alert naming the parts that stalled since the last alert. A send that fails is
// tried again on the next tick.
func (w *MonitorWatch) Tick(ctx context.Context) {
	fresh := w.update()
	if len(fresh) == 0 || w.deps.Notify == nil || ctx.Err() != nil {
		return
	}
	if err := w.deps.Notify(ctx, MonitorMessage(fresh)); err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range fresh {
		if _, still := w.stalled[s.ID]; still {
			w.told[s.ID] = true
		}
	}
}

// update reads the parts and returns the stalls not yet told.
func (w *MonitorWatch) update() []MonitorStall {
	now := w.deps.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	var fresh []MonitorStall
	for _, p := range w.deps.Parts {
		r := p.Read()
		if !r.On {
			w.forget(p.ID)
			continue
		}
		good, seen := w.good[p.ID]
		if !seen {
			good = now // the limit starts when the part is first seen on: after a start, or the operator switching it on
		}
		if r.LastGood.After(good) {
			good = r.LastGood
		}
		w.good[p.ID] = good
		if now.Sub(good) <= r.Limit {
			delete(w.stalled, p.ID)
			delete(w.told, p.ID)
			continue
		}
		stall := MonitorStall{ID: p.ID, Words: p.Words, Since: good}
		w.stalled[p.ID] = stall
		if !w.told[p.ID] {
			fresh = append(fresh, stall)
		}
	}
	return fresh
}

func (w *MonitorWatch) forget(id string) {
	delete(w.good, id)
	delete(w.stalled, id)
	delete(w.told, id)
}

// Stalled lists the parts stalled at the last tick, in the order the parts are defined.
func (w *MonitorWatch) Stalled() []MonitorStall {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []MonitorStall
	for _, p := range w.deps.Parts {
		if s, ok := w.stalled[p.ID]; ok {
			out = append(out, s)
		}
	}
	return out
}

// MonitorMessage is the alert for parts that stopped. The words are fixed and the dates are this program's own.
func MonitorMessage(stalls []MonitorStall) string {
	parts := make([]string, 0, len(stalls))
	for _, s := range stalls {
		parts = append(parts, fmt.Sprintf("%s (not since %s)", s.Words, s.Since.Format("2 Jan 15:04")))
	}
	return "Network audit: a monitor that is switched on has stopped working: " + strings.Join(parts, "; ") +
		". Until it works again the Network page shows older data. Open the network audit to see why."
}

// MonitorFindings is one finding about this computer while any part is stalled; it names every stalled part. One finding, not
// one per part, because a finding's key has no room for the part and two would replace each other.
func MonitorFindings(stalls []MonitorStall) []Finding {
	if len(stalls) == 0 {
		return nil
	}
	words := make([]string, 0, len(stalls))
	for _, s := range stalls {
		words = append(words, fmt.Sprintf("%s (not since %s)", s.Words, s.Since.Format(dateLayout)))
	}
	slices.Sort(words)
	return []Finding{{
		Device: HostDevice, Check: CheckMonitorStalled, Severity: SeverityMedium,
		Evidence: "A part of the Network module that is switched on has stopped working: " + strings.Join(words, "; ") +
			". What it watches is not being watched until it works again",
	}}
}

// MonitorStalls is what the nightly report reads: *MonitorWatch.
type MonitorStalls interface {
	Stalled() []MonitorStall
}

func init() {
	checkTitles[CheckMonitorStalled] = "A Network monitor that is switched on has stopped working"
	remediations[CheckMonitorStalled] = "Open the part the finding names on the Network page and read its status line: it says why it cannot work (a port in use, a setting, the router no longer sending). Fix that, or switch the part off if you no longer want it."
	guides[CheckMonitorStalled] = FixGuide{
		Steps: []string{
			"Open the Network page and find the part the finding names: its card or settings line says what it last did and, when it can, why it stopped.",
			"A listener that cannot open its port (the decoys, the router's log) usually means another program took the port, or the service restarted without the right to use it; the service's log names the error.",
			"A router that stopped sending its log may have been restarted or updated and lost its remote-log setting; set it again in the router's own pages.",
			"If you no longer use the part, switch it off: a part that is off is never watched.",
			"If you did not stop it and cannot find why it stopped, treat it as a warning: something that silences monitoring is what an intruder would do first.",
		},
		Commands: []GuideCommand{{platformLinux, "journalctl -u llm-proxy --since -1d | grep -i network\nsudo ss -lnup | grep 5514\nsudo ss -lntp"}},
		Risk:     "None from looking. Switching a part off stops what it watches.",
		Undo:     "Switch the part on again.",
	}
}
