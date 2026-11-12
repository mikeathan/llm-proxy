package netaudit

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// Bounds on the window digest. The digest goes to a local model with a small context (a dense result once overflowed 16k tokens),
// so every list has a cap, worst or newest first, and the cut is stated. DigestMaxBytes is the whole rendered digest: if the
// lists are still too long the longest is halved until it fits.
const (
	DigestMaxScanRows = 12
	DigestMaxOpen     = 25
	DigestMaxListed   = 15
	DigestMaxBytes    = 9000
)

// DigestInput is everything DigestOf needs, already read; the caller reads each part from where it already lives.
type DigestInput struct {
	Now        time.Time
	Window     time.Duration
	Automation string
	Log        []models.ScanEntry
	Life       []models.FindingLife
	// Baseline is the latest saved scan (the one before the scan being made), with every module's findings in it.
	Baseline  models.ScanBaseline
	Acks      map[string]models.ScanAck
	Alerts    []models.NetworkAlert
	Series    []models.NetworkDay
	Inventory models.DeviceInventory
}

// DigestRange is the lowest and highest of a number across the window's scans.
type DigestRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// DigestScanRow is one scan of the window.
type DigestScanRow struct {
	At         time.Time `json:"at"`
	Unattended bool      `json:"scheduled,omitempty"`
	DevicesUp  int       `json:"devices_up"`
	OpenPorts  int       `json:"open_ports"`
	Serious    int       `json:"serious,omitempty"`
	New        int       `json:"new,omitempty"`
	Resolved   int       `json:"resolved,omitempty"`
}

// DigestScans is what the window's scans looked like. Count is every scan; Rows are the latest few, oldest first.
type DigestScans struct {
	Count     int             `json:"count"`
	Scheduled int             `json:"scheduled"`
	Pressed   int             `json:"pressed"`
	DevicesUp DigestRange     `json:"devices_up"`
	OpenPorts DigestRange     `json:"open_ports"`
	Rows      []DigestScanRow `json:"rows,omitempty"`
}

// DigestOpen is a finding of the latest saved scan that is still open. Service and Version are text a device sent, which is why
// the whole digest goes to the model inside the untrusted-data fence.
type DigestOpen struct {
	Device    string `json:"device"`
	Port      int    `json:"port,omitempty"`
	Proto     string `json:"proto,omitempty"`
	Check     string `json:"check"`
	Severity  string `json:"severity"`
	Service   string `json:"service,omitempty"`
	Version   string `json:"version,omitempty"`
	FirstSeen string `json:"first_seen,omitempty"`
	CameBack  int    `json:"came_back,omitempty"`
}

// DigestChange is a finding that appeared, went away or came back inside the window. Check is the check's plain title.
type DigestChange struct {
	Device   string    `json:"device"`
	Port     int       `json:"port,omitempty"`
	Check    string    `json:"check"`
	At       time.Time `json:"at"`
	CameBack int       `json:"came_back,omitempty"`
}

// DigestActivity is the window's other happenings as counts by kind. A device's name, address or hardware address and an
// alert's text are never in it.
type DigestActivity struct {
	NewDevices     int            `json:"new_devices,omitempty"`
	AddressChanges int            `json:"address_changes,omitempty"`
	PortsOpened    int            `json:"ports_opened,omitempty"`
	PortsClosed    int            `json:"ports_closed,omitempty"`
	RouterBlocked  int            `json:"router_refused_attempts,omitempty"`
	Alerts         map[string]int `json:"alerts,omitempty"`
}

// WindowDigest is the bounded, typed account of the last window of the network: the scans, what is open, what changed and what
// else happened. It is built only from records the program already keeps; building it probes nothing and keeps nothing.
type WindowDigest struct {
	Window string      `json:"window"`
	From   time.Time   `json:"from"`
	To     time.Time   `json:"to"`
	Scans  DigestScans `json:"scans"`
	// Open are the problems of the latest saved scan, worst first, without what the operator accepted and without known-vulnerability
	// findings (the report adds those itself). OpenPorts is how many open ports it showed.
	Open      []DigestOpen `json:"open,omitempty"`
	OpenMore  int          `json:"open_more,omitempty"`
	OpenPorts int          `json:"open_ports,omitempty"`
	// OpenAllWindow is how many findings were already open when the window began and still are.
	OpenAllWindow    int            `json:"open_all_window"`
	NewInWindow      []DigestChange `json:"new_in_window,omitempty"`
	NewMore          int            `json:"new_more,omitempty"`
	GoneInWindow     []DigestChange `json:"gone_in_window,omitempty"`
	GoneMore         int            `json:"gone_more,omitempty"`
	ReturnedInWindow []DigestChange `json:"returned_in_window,omitempty"`
	ReturnedMore     int            `json:"returned_more,omitempty"`
	Activity         DigestActivity `json:"activity"`
	// Unread names a record that could not be read, so the digest is never taken for a quiet network.
	Unread []string `json:"unread,omitempty"`
}

// DigestOf builds the digest for [Now-Window, Now]. It is a pure function of its input.
func DigestOf(in DigestInput) WindowDigest {
	from := in.Now.Add(-in.Window)
	d := WindowDigest{Window: windowLabel(in.Window), From: from, To: in.Now}
	d.Scans = digestScans(in, from)
	hidden := hiddenKeys(in)
	d.Open, d.OpenMore, d.OpenPorts = digestOpen(in, hidden)
	d.digestLife(in, from, hidden)
	d.Activity = digestActivity(in, from)
	return d
}

func windowLabel(w time.Duration) string {
	if w >= 48*time.Hour && w%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(w/(24*time.Hour)))
	}
	return fmt.Sprintf("%dh", int(w/time.Hour))
}

func digestScans(in DigestInput, from time.Time) DigestScans {
	var rows []DigestScanRow
	var s DigestScans
	for _, e := range in.Log {
		if e.Automation != in.Automation || e.At.Before(from) || e.At.After(in.Now) {
			continue
		}
		rows = append(rows, DigestScanRow{At: e.At, Unattended: e.Unattended, DevicesUp: e.DevicesUp, OpenPorts: e.OpenPorts,
			Serious: e.BySeverity[SeverityHigh] + e.BySeverity[SeverityCritical], New: e.New, Resolved: e.Resolved})
	}
	slices.SortStableFunc(rows, func(a, b DigestScanRow) int { return a.At.Compare(b.At) })
	for i, r := range rows {
		s.Count++
		if r.Unattended {
			s.Scheduled++
		} else {
			s.Pressed++
		}
		if i == 0 {
			s.DevicesUp, s.OpenPorts = DigestRange{r.DevicesUp, r.DevicesUp}, DigestRange{r.OpenPorts, r.OpenPorts}
			continue
		}
		s.DevicesUp = DigestRange{min(s.DevicesUp.Min, r.DevicesUp), max(s.DevicesUp.Max, r.DevicesUp)}
		s.OpenPorts = DigestRange{min(s.OpenPorts.Min, r.OpenPorts), max(s.OpenPorts.Max, r.OpenPorts)}
	}
	if len(rows) > DigestMaxScanRows {
		rows = rows[len(rows)-DigestMaxScanRows:]
	}
	s.Rows = rows
	return s
}

// hiddenKeys are the findings the digest never shows: what the operator accepted, whatever the check. Known-vulnerability findings
// are left out by their check, not here.
func hiddenKeys(in DigestInput) map[string]bool {
	hidden := map[string]bool{}
	for _, f := range in.Baseline.Findings {
		if a, ok := in.Acks[f.Key()]; ok && StateFor(a, f, in.Now) == AckActive {
			hidden[f.Key()] = true
		}
	}
	return hidden
}

func digestOpen(in DigestInput, hidden map[string]bool) (open []DigestOpen, more, ports int) {
	life := make(map[string]models.FindingLife, len(in.Life))
	for _, l := range in.Life {
		life[l.Key] = l
	}
	var pick []models.ScanFinding
	for _, f := range in.Baseline.Findings {
		switch {
		case f.Check == CheckOpenPort:
			ports++
		case f.Check == CheckKnownVulns, hidden[f.Key()]:
		default:
			pick = append(pick, f)
		}
	}
	slices.SortStableFunc(pick, func(a, b models.ScanFinding) int {
		if c := severityWeight(b.Severity) - severityWeight(a.Severity); c != 0 {
			return c
		}
		return strings.Compare(a.Key(), b.Key())
	})
	if len(pick) > DigestMaxOpen {
		more, pick = len(pick)-DigestMaxOpen, pick[:DigestMaxOpen]
	}
	for _, f := range pick {
		o := DigestOpen{Device: f.Device, Port: f.Port, Proto: f.Proto, Check: titleOrID(f.Check), Severity: f.Severity, Service: f.Service, Version: f.Version}
		if l, ok := life[f.Key()]; ok {
			o.FirstSeen, o.CameBack = l.FirstSeen.UTC().Format(time.RFC3339), l.Returned
		}
		open = append(open, o)
	}
	return open, more, ports
}

func severityWeight(s string) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// lifeKind says what a finding's history did inside the window.
type lifeKind int

const (
	lifeQuiet    lifeKind = iota // nothing inside the window, and not open from before it
	lifeNew                      // first seen inside the window
	lifeGone                     // resolved inside the window
	lifeReturned                 // came back inside the window
	lifeWhole                    // open from before the window began and still open
)

// classifyLife reads one finding's history against the window and gives the time that put it in its class.
func classifyLife(l models.FindingLife, from time.Time) (lifeKind, time.Time) {
	switch {
	case !l.Open():
		if l.ResolvedAt.Before(from) {
			return lifeQuiet, time.Time{}
		}
		return lifeGone, l.ResolvedAt
	case l.Returned > 0 && !l.OpenSince.Before(from):
		return lifeReturned, l.OpenSince
	case !l.FirstSeen.Before(from):
		return lifeNew, l.FirstSeen
	case !l.OpenSince.After(from):
		return lifeWhole, time.Time{}
	}
	return lifeQuiet, time.Time{}
}

// digestLife reads each finding's history for what began, ended or came back inside the window.
func (d *WindowDigest) digestLife(in DigestInput, from time.Time, hidden map[string]bool) {
	var fresh, gone, back []DigestChange
	for _, l := range in.Life {
		dev, port, check, ok := splitFindingKey(l.Key)
		if !ok || check == CheckKnownVulns || hidden[l.Key] {
			continue
		}
		kind, at := classifyLife(l, from)
		change := DigestChange{Device: dev, Port: port, Check: titleOrID(check), At: at, CameBack: l.Returned}
		switch kind {
		case lifeNew:
			fresh = append(fresh, change)
		case lifeGone:
			gone = append(gone, change)
		case lifeReturned:
			back = append(back, change)
		case lifeWhole:
			d.OpenAllWindow++
		}
	}
	d.NewInWindow, d.NewMore = newestFirst(fresh)
	d.GoneInWindow, d.GoneMore = newestFirst(gone)
	d.ReturnedInWindow, d.ReturnedMore = newestFirst(back)
}

func newestFirst(rows []DigestChange) ([]DigestChange, int) {
	slices.SortStableFunc(rows, func(a, b DigestChange) int {
		if c := b.At.Compare(a.At); c != 0 {
			return c
		}
		return strings.Compare(a.Device, b.Device)
	})
	if len(rows) > DigestMaxListed {
		return rows[:DigestMaxListed], len(rows) - DigestMaxListed
	}
	return rows, 0
}

func digestActivity(in DigestInput, from time.Time) DigestActivity {
	act := ActivityOf(ActivityInput{Now: in.Now, Inventory: in.Inventory, Alerts: in.Alerts, Series: in.Series, Life: in.Life}, MaxActivityRows)
	var out DigestActivity
	for _, r := range act.Rows {
		if r.At.Before(from) {
			continue
		}
		switch r.Kind {
		case ActivityNewDevice:
			out.NewDevices++
		case ActivityAddressChanged:
			out.AddressChanges++
		case ActivityPortsChanged:
			out.PortsOpened += r.Opened
			out.PortsClosed += r.Closed
		case ActivityRouterBlocks:
			out.RouterBlocked += r.Count
		case ActivityAlert:
			if models.NetworkAlertKind(r.AlertKind).Valid() {
				if out.Alerts == nil {
					out.Alerts = map[string]int{}
				}
				out.Alerts[r.AlertKind] += max(r.Count, 1)
			}
		}
	}
	return out
}

// RenderDigest is the digest as the model reads it: one fixed sentence saying what it covers, then compact JSON. A window with no
// recorded scan says so in words, so an empty digest is never read as a quiet network. Lists are halved, longest first, until it
// fits DigestMaxBytes, and the cut is added to each list's "more".
func RenderDigest(d WindowDigest) string {
	for {
		body, err := json.Marshal(d)
		if err != nil {
			return "window digest: could not be built."
		}
		if len(body) <= DigestMaxBytes || !d.shrink() {
			return digestHeader(d) + "\n" + string(body)
		}
	}
}

func digestHeader(d WindowDigest) string {
	if d.Scans.Count == 0 {
		return fmt.Sprintf("window digest (last %s): no scan was recorded in this window; the rest is what the program kept from earlier scans and from its watches.", d.Window)
	}
	return fmt.Sprintf("window digest (last %s): %d scan(s) recorded, %d scheduled and %d pressed. Counts are exact; lists are cut worst or newest first and the cut is stated.", d.Window, d.Scans.Count, d.Scans.Scheduled, d.Scans.Pressed)
}

// shrink halves the longest list and reports whether it cut anything.
func (d *WindowDigest) shrink() bool {
	type list struct {
		n   int
		cut func(keep int)
	}
	lists := []list{
		{len(d.Open), func(k int) { d.OpenMore += len(d.Open) - k; d.Open = d.Open[:k] }},
		{len(d.NewInWindow), func(k int) { d.NewMore += len(d.NewInWindow) - k; d.NewInWindow = d.NewInWindow[:k] }},
		{len(d.GoneInWindow), func(k int) { d.GoneMore += len(d.GoneInWindow) - k; d.GoneInWindow = d.GoneInWindow[:k] }},
		{len(d.ReturnedInWindow), func(k int) {
			d.ReturnedMore += len(d.ReturnedInWindow) - k
			d.ReturnedInWindow = d.ReturnedInWindow[:k]
		}},
		{len(d.Scans.Rows), func(k int) { d.Scans.Rows = d.Scans.Rows[len(d.Scans.Rows)-k:] }},
	}
	best := 0
	for i, l := range lists {
		if l.n > lists[best].n {
			best = i
		}
	}
	if lists[best].n == 0 {
		return false
	}
	lists[best].cut(lists[best].n / 2)
	return true
}
