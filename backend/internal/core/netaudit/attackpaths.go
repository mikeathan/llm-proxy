package netaudit

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"llm-proxy/models"
)

// Attack paths turn the audit's findings into the few ways in that matter: a way in, the weaknesses it passes through, and the
// thing at the end worth protecting. They are fixed rules in code over what the audit already holds; no model builds a path and
// nothing is probed. A path is never drawn from an open port alone: it needs a weakness behind the port.

// MaxAttackPaths is how many paths are shown.
const MaxAttackPaths = 3

// EntryKind is where a path starts.
type EntryKind string

const (
	// EntryInternet is a port the internet can reach (the Internet view's finding).
	EntryInternet EntryKind = "internet"
	// EntryOnNetwork is any device on the network: a guest, a phone, a camera that was taken over.
	EntryOnNetwork EntryKind = "network"
)

// PrizeKind is what a path ends at.
type PrizeKind string

const (
	PrizeComputer  PrizeKind = "computer"
	PrizeRouter    PrizeKind = "router"
	PrizeImportant PrizeKind = "important"
)

// PathEntry is the way in, in fixed words. Probing is set on a network entry while a device has touched a decoy.
type PathEntry struct {
	Kind    EntryKind `json:"kind"`
	Text    string    `json:"text"`
	Probing bool      `json:"probing,omitempty"`
}

// PathStep is one real finding the path passes through.
type PathStep struct {
	Key      string `json:"key"`
	Check    string `json:"check"`
	Title    string `json:"title"`
	Port     int    `json:"port,omitempty"`
	Severity string `json:"severity"`
	// Accepted marks a finding the operator accepted: it stays in the path, quietly.
	Accepted bool `json:"accepted,omitempty"`
}

// PathPrize is what the path ends at. Where is the fixed label of this computer or the address of the device.
type PathPrize struct {
	Kind  PrizeKind `json:"kind"`
	Where string    `json:"where"`
	// Name is what the operator calls an important device. The screen shows it as text; the report text never carries it.
	Name string `json:"name,omitempty"`
}

// PathFix is the one change that breaks the path: the key of the finding to fix and its fix text.
type PathFix struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// AttackPath is one way in to one prize.
type AttackPath struct {
	ID    string     `json:"id"`
	Entry PathEntry  `json:"entry"`
	Steps []PathStep `json:"steps"`
	Prize PathPrize  `json:"prize"`
	Fix   PathFix    `json:"fix"`
	// Breaks is how many of the paths found the fix breaks, itself included.
	Breaks int `json:"breaks"`
	// AllAccepted is true when every step is one the operator accepted.
	AllAccepted bool `json:"all_accepted,omitempty"`
}

// PathInput is what the paths are built from. Visible and Accepted are the audit's findings split by the operator's acceptances.
type PathInput struct {
	Visible, Accepted []Finding
	Inventory         models.DeviceInventory
	Touches           []models.DecoyTouch
}

// Fixed words of the paths.
const (
	entryInternetText      = "The internet can reach a port on your router"
	entryInternetPortText  = "The internet can reach port %d on your router"
	entryNetworkText       = "Any device on your network"
	entryProbingText       = "A device on your network that has already probed a decoy"
	pathAcceptedMark       = " (accepted)"
	fixFallback            = "Fix the weakness this step names."
	prizeComputerWords     = "this computer"
	prizeRouterWords       = "your router"
	prizeImportantWords    = "the important device at "
	pathsReportHeading     = "### Ways in that matter"
	pathsReportBreaksWords = "One fix breaks %d of these paths."
)

// stepChecks are the findings that let someone in or further: a service with no real login, a weak login, a flaw known to be
// attacked. An open port, a certificate or an old version is not on the list.
var stepChecks = map[string]bool{
	CheckSSHPasswordLogin: true, CheckHostSSHPasswordLogin: true, CheckHostSSHRootLogin: true, CheckHostSSHEmptyPasswords: true,
	CheckTelnetOpen: true, CheckSMBv1: true, CheckSMBSigning: true, CheckFTPAnonymous: true, CheckVNCNoAuth: true,
	CheckRedisNoAuth: true, CheckNFSExports: true, CheckRDPNoNLA: true, CheckMemcachedOpen: true, CheckMongoNoAuth: true,
	CheckMQTTAnonymous: true, CheckDockerAPI: true, CheckADBExposed: true, CheckOpenProxy: true, CheckElasticsearchExposed: true,
	CheckRsyncExposed: true, CheckKubeletExposed: true, CheckEtcdExposed: true, CheckRTSPNoAuth: true, CheckAIAPIExposed: true,
	CheckOwnModelServerOpen: true, CheckAdminServiceOpen: true, CheckKnownVulns: true, CheckUPnPForward: true,
}

// isStep says whether a finding is a weakness a path may pass through. A known-flaw finding counts only when one of its flaws is
// known to be attacked.
func isStep(f Finding) bool {
	if !stepChecks[f.Check] {
		return false
	}
	return f.Check != CheckKnownVulns || len(f.Exploited) > 0
}

// AttackPaths builds the paths, best first, at most MaxAttackPaths. It is pure.
func AttackPaths(in PathInput) []AttackPath {
	steps := stepsByDevice(in)
	exposed := internetPorts(append(slices.Clone(in.Visible), in.Accepted...))
	var paths []AttackPath
	for _, device := range slices.Sorted(maps.Keys(steps)) {
		prize, ok := prizeOf(device, in.Inventory)
		if !ok {
			continue
		}
		for _, entry := range entriesFor(prize, exposed, in.Touches) {
			paths = append(paths, AttackPath{ID: string(entry.Kind) + "|" + device, Entry: entry, Steps: steps[device], Prize: prize})
		}
	}
	rankPaths(paths)
	slices.SortStableFunc(paths, comparePaths)
	if len(paths) > MaxAttackPaths {
		paths = paths[:MaxAttackPaths]
	}
	return paths
}

func stepsByDevice(in PathInput) map[string][]PathStep {
	out := map[string][]PathStep{}
	add := func(list []Finding, accepted bool) {
		for _, f := range list {
			if !isStep(f) {
				continue
			}
			out[f.Device] = append(out[f.Device], PathStep{Key: f.Key(), Check: f.Check, Title: checkTitles[f.Check], Port: f.Port, Severity: f.Severity, Accepted: accepted})
		}
	}
	add(in.Visible, false)
	add(in.Accepted, true)
	for _, steps := range out {
		slices.SortStableFunc(steps, func(a, b PathStep) int {
			return cmp.Or(cmp.Compare(severityRank(b.Severity), severityRank(a.Severity)), cmp.Compare(a.Key, b.Key))
		})
	}
	return out
}

// internetPorts are the ports the Internet view says the internet can reach, lowest first.
func internetPorts(findings []Finding) []int {
	var ports []int
	for _, f := range findings {
		if f.Check == CheckInternetExposedPort {
			ports = append(ports, f.Port)
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}

// prizeOf says whether a device is worth a path: this computer, the router, or a device the operator marked important.
func prizeOf(device string, inv models.DeviceInventory) (PathPrize, bool) {
	switch {
	case device == HostDevice:
		return PathPrize{Kind: PrizeComputer, Where: HostDevice}, true
	case inv.Gateway != "" && device == inv.Gateway:
		return PathPrize{Kind: PrizeRouter, Where: device}, true
	}
	for _, d := range inv.Devices {
		if d.IP == device && d.Important {
			return PathPrize{Kind: PrizeImportant, Where: device, Name: d.Name}, true
		}
	}
	return PathPrize{}, false
}

// entriesFor are the ways in to a prize: the internet reaches only the router, and any device on the network reaches anything.
func entriesFor(prize PathPrize, exposed []int, touches []models.DecoyTouch) []PathEntry {
	var out []PathEntry
	if prize.Kind == PrizeRouter && len(exposed) > 0 {
		text := entryInternetText
		if exposed[0] > 0 {
			text = fmt.Sprintf(entryInternetPortText, exposed[0])
		}
		out = append(out, PathEntry{Kind: EntryInternet, Text: text})
	}
	network := PathEntry{Kind: EntryOnNetwork, Text: entryNetworkText}
	if len(touches) > 0 {
		network.Text, network.Probing = entryProbingText, true
	}
	return append(out, network)
}

// rankPaths picks each path's fix: the step that appears in the most paths, a live step before an accepted one, then the more
// serious. Breaks is how many paths that step is in.
func rankPaths(paths []AttackPath) {
	counts := map[string]int{}
	for _, p := range paths {
		for _, s := range p.Steps {
			counts[s.Key]++
		}
	}
	for i := range paths {
		p := &paths[i]
		best := slices.MaxFunc(p.Steps, func(a, b PathStep) int {
			return cmp.Or(cmp.Compare(boolRank(!a.Accepted), boolRank(!b.Accepted)), cmp.Compare(counts[a.Key], counts[b.Key]), cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)))
		})
		text := remediations[best.Check]
		if text == "" {
			text = fixFallback
		}
		p.Fix, p.Breaks = PathFix{Key: best.Key, Text: text}, counts[best.Key]
		p.AllAccepted = !slices.ContainsFunc(p.Steps, func(s PathStep) bool { return !s.Accepted })
	}
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// comparePaths orders live paths before ones every step of which is accepted, then by how many paths one fix breaks, then by the
// most serious step, then by id so the order is stable.
func comparePaths(a, b AttackPath) int {
	return cmp.Or(
		cmp.Compare(boolRank(a.AllAccepted), boolRank(b.AllAccepted)),
		cmp.Compare(b.Breaks, a.Breaks),
		cmp.Compare(severityRank(b.Steps[0].Severity), severityRank(a.Steps[0].Severity)),
		cmp.Compare(a.ID, b.ID),
	)
}

// PathsText is the paths in fixed words for the nightly report. It never holds a device name.
func PathsText(paths []AttackPath) string {
	if len(paths) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(pathsReportHeading + "\n")
	for i, p := range paths {
		titles := make([]string, 0, len(p.Steps))
		for _, s := range p.Steps {
			t := s.Title
			if s.Accepted {
				t += pathAcceptedMark
			}
			titles = append(titles, t)
		}
		fmt.Fprintf(&b, "%d. %s → %s → %s. %s %s\n", i+1, p.Entry.Text, strings.Join(titles, ", "), prizeWords(p.Prize), p.Fix.Text, fmt.Sprintf(pathsReportBreaksWords, p.Breaks))
	}
	return b.String()
}

func prizeWords(p PathPrize) string {
	switch p.Kind {
	case PrizeComputer:
		return prizeComputerWords
	case PrizeRouter:
		return prizeRouterWords
	}
	return prizeImportantWords + p.Where
}

// AttackPathsConfig reads and changes whether the attack paths are shown. They are on unless switched off.
type AttackPathsConfig struct{ store PolicyStore }

func NewAttackPathsConfig(store PolicyStore) *AttackPathsConfig {
	return &AttackPathsConfig{store: store}
}

// Enabled reports whether the attack paths are on.
func (c *AttackPathsConfig) Enabled() bool { return !c.store.Get().AttackPathsOff }

// Set switches the attack paths on or off.
func (c *AttackPathsConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.AttackPathsOff = !enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the attack paths setting: %w", err)
	}
	return nil
}
