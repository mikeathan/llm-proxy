package netaudit

import (
	"slices"
	"time"

	"llm-proxy/models"
)

// The posture is one word for the top of the Network page.
const (
	PostureUnknown   = "unknown"   // nothing has looked at the network yet
	PostureQuiet     = "quiet"     // nothing needs the operator
	PostureAttention = "attention" // something does
)

// Kinds of recent change.
const (
	RecentNewDevice      = "new_device"
	RecentAddressChanged = "address_changed"
)

// maxRecent bounds the "what changed" list, and recentWindow how far back a new device still counts as recent.
const (
	maxRecent    = 12
	recentWindow = 14 * 24 * time.Hour
)

// OverviewInput is what the overview is built from; the caller reads each part from where it already lives.
type OverviewInput struct {
	Now       time.Time
	Inventory models.DeviceInventory
	Baseline  models.ScanBaseline
	// Acks are the operator's acceptances of the audit's findings; an accepted finding is not counted.
	Acks     []models.ScanAck
	Presence PresenceStatus
	Series   []models.NetworkDay
	// AttackPaths is true while the attack paths module is on; Touches are the devices that connected to a decoy.
	AttackPaths bool
	Touches     []models.DecoyTouch
	// Life is the history of each finding; it gives the standing. Optional.
	Life []models.FindingLife
}

// DeviceCounts are the device numbers of the overview.
type DeviceCounts struct {
	Known   int `json:"known"`
	Unnamed int `json:"unnamed"`
	// New are the devices nobody has approved or named, which is what raises a "new device" alert.
	New int `json:"new"`
	// Active are the devices seen in the last 24 hours.
	Active int `json:"active"`
}

// FindingCounts are the audit numbers of the overview.
type FindingCounts struct {
	Scanned   bool      `json:"scanned"`
	ScannedAt time.Time `json:"scanned_at,omitzero"`
	// Open are the problems the checks found and the operator has not accepted (what the Findings tab lists as problems);
	// Serious the high and critical ones among them; Ports the open ports, which are facts, not problems.
	Open    int `json:"open"`
	Serious int `json:"serious"`
	Ports   int `json:"ports"`
	New     int `json:"new"`
}

// RecentChange is one thing that changed lately. It names a device by its hardware and IP address, and by the name the
// operator gave it; the screen composes the words, so nothing a device printed is in it.
type RecentChange struct {
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
	MAC  string    `json:"mac,omitempty"`
	IP   string    `json:"ip,omitempty"`
	Name string    `json:"name,omitempty"`
}

// Overview is the Network page's first screen: a verdict, a few counts, the trend and what changed lately. Every part is
// read from data the audit and the presence check already keep; building it probes nothing.
type Overview struct {
	Posture        string              `json:"posture"`
	Devices        DeviceCounts        `json:"devices"`
	Findings       FindingCounts       `json:"findings"`
	GatewayChanged bool                `json:"gateway_changed"`
	SharedAddress  bool                `json:"shared_address"`
	Presence       PresenceStatus      `json:"presence"`
	Series         []models.NetworkDay `json:"series"`
	Recent         []RecentChange      `json:"recent"`
	// AttackPaths are the few ways in that matter, best first; an empty list while there are none or the module is off.
	AttackPaths []AttackPath `json:"attack_paths"`
	// AttackPathsOn says whether the module is on, so the card can tell "nothing found" from "switched off".
	AttackPathsOn bool `json:"attack_paths_on"`
	// Standing is how long the open problems have lasted and how the last week compared with the one before, from the finding history.
	Standing Standing `json:"standing"`
}

// OverviewOf builds the overview.
func OverviewOf(in OverviewInput) Overview {
	o := Overview{
		Presence: in.Presence, Series: in.Series, Recent: recentActivity(in.Inventory, in.Now),
		GatewayChanged: gatewayChanged(in.Inventory), SharedAddress: len(in.Inventory.Shared) > 0,
		Devices: countDevices(in.Inventory, in.Now), Findings: countFindings(in.Baseline, in.Acks, in.Now),
	}
	if o.Series == nil {
		o.Series = []models.NetworkDay{}
	}
	o.AttackPaths, o.AttackPathsOn = overviewPaths(in), in.AttackPaths
	o.Standing = StandingOf(StandingInput{Now: in.Now, Baseline: in.Baseline, Acks: in.Acks, Life: in.Life})
	o.Posture = postureOf(o, in.Inventory.Baselined)
	return o
}

func countDevices(inv models.DeviceInventory, now time.Time) DeviceCounts {
	var c DeviceCounts
	for _, d := range inv.Devices {
		c.Known++
		if d.Name == "" {
			c.Unnamed++
		}
		if newDeviceIsNews(d, now) {
			c.New++
		}
		if now.Sub(d.LastSeen) <= 24*time.Hour {
			c.Active++
		}
	}
	return c
}

func countFindings(b models.ScanBaseline, acks []models.ScanAck, now time.Time) FindingCounts {
	if b.ScannedAt.IsZero() {
		return FindingCounts{}
	}
	c := FindingCounts{Scanned: true, ScannedAt: b.ScannedAt, New: len(b.NewKeys)}
	for _, f := range ApplyAcks(b.Findings, acks, now).Visible {
		if f.Check == CheckOpenPort {
			c.Ports++
			continue
		}
		c.Open++
		if f.Severity == SeverityHigh || f.Severity == SeverityCritical {
			c.Serious++
		}
	}
	return c
}

func postureOf(o Overview, baselined bool) string {
	switch {
	case o.Devices.New > 0 || o.Findings.Serious > 0 || o.GatewayChanged || o.SharedAddress:
		return PostureAttention
	case !baselined && !o.Findings.Scanned:
		return PostureUnknown
	}
	return PostureQuiet
}

func recentActivity(inv models.DeviceInventory, now time.Time) []RecentChange {
	out := allRecentChanges(inv, now)
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	return out
}

// allRecentChanges lists every recent change, newest first and not cut: the overview shows the first few, the activity list
// the whole of it.
func allRecentChanges(inv models.DeviceInventory, now time.Time) []RecentChange {
	var out []RecentChange
	if inv.Baselined {
		for _, d := range inv.Devices {
			if now.Sub(d.FirstSeen) <= recentWindow {
				out = append(out, RecentChange{Kind: RecentNewDevice, At: d.FirstSeen, MAC: d.MAC, IP: d.IP, Name: d.Name})
			}
		}
	}
	for _, c := range inv.Changes {
		out = append(out, RecentChange{Kind: RecentAddressChanged, At: c.At, MAC: c.To, IP: c.IP})
	}
	slices.SortStableFunc(out, func(a, b RecentChange) int { return b.At.Compare(a.At) })
	return out
}

// overviewPaths are the attack paths of the last audit, with the operator's acceptances applied; empty while the module is off or
// no audit has run.
func overviewPaths(in OverviewInput) []AttackPath {
	if !in.AttackPaths || in.Baseline.ScannedAt.IsZero() {
		return []AttackPath{}
	}
	applied := ApplyAcks(in.Baseline.Findings, in.Acks, in.Now)
	paths := AttackPaths(PathInput{Visible: applied.Visible, Accepted: applied.Accepted, Inventory: in.Inventory, Touches: in.Touches})
	if paths == nil {
		return []AttackPath{}
	}
	return paths
}
