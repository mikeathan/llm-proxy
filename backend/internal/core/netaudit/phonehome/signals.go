package phonehome

import (
	"cmp"
	"slices"
	"time"

	"llm-proxy/models"
)

// Kind is what a signal says.
type Kind string

const (
	// NewPlace is a name a device asked for for the first time after its learning week.
	NewPlace Kind = "new_place"
	// Beaconing is such a name asked for at a steady beat.
	Beaconing Kind = "beaconing"
	// AroundDNS is a device that is online and asks the router for nothing: it uses a resolver of its own.
	AroundDNS Kind = "around_dns"
)

// Signal is one thing worth a look. Names are plain DNS names (vetted by the parser) and are empty for AroundDNS.
type Signal struct {
	Kind  Kind
	MAC   string
	Names []string
}

// InventoryDevice is what the signals need of the device list: who it is and when it was seen.
type InventoryDevice struct {
	MAC                 string
	FirstSeen, LastSeen time.Time
}

const (
	// silentFor is how long a device that is online must have asked nothing, and how long the watch must have watched, before it is
	// reported as going around the router's DNS.
	silentFor = 24 * time.Hour
	// onlineWithin is how recently a device was seen to count as online now.
	onlineWithin = time.Hour
	// beatWindow, beatMin, beatSteady: a name is beating when at least beatMin of its lookups fall in the last beatWindow and at least
	// beatSteady of the gaps between them are within a quarter of the median gap, which itself lies between beatShortest and beatLongest.
	beatWindow   = 24 * time.Hour
	beatMin      = 12
	beatSteady   = 0.75
	beatShortest = 10 * time.Second
	beatLongest  = 3 * time.Hour
)

// Evaluate returns what is new now. It is pure: it reads the names and the device list and changes neither. A device is judged only
// after its learning week, and only on names it first asked for after that week.
func Evaluate(state models.PhoneHome, devices []InventoryDevice, now time.Time) []Signal {
	var out []Signal
	for _, d := range state.Devices {
		if now.Sub(d.First) < LearnFor {
			continue
		}
		var fresh, beating []string
		for _, n := range d.Names {
			if !isNew(d, n) || now.Sub(n.First) > NewFor {
				continue
			}
			fresh = append(fresh, n.Name)
			if beats(n.Times, now) {
				beating = append(beating, n.Name)
			}
		}
		slices.Sort(fresh)
		slices.Sort(beating)
		if len(fresh) > 0 {
			out = append(out, Signal{Kind: NewPlace, MAC: d.MAC, Names: fresh})
		}
		if len(beating) > 0 {
			out = append(out, Signal{Kind: Beaconing, MAC: d.MAC, Names: beating})
		}
	}
	out = append(out, silent(state, devices, now)...)
	slices.SortStableFunc(out, func(a, b Signal) int { return cmp.Or(cmp.Compare(a.MAC, b.MAC), cmp.Compare(a.Kind, b.Kind)) })
	return out
}

// beats says whether lookups at these times keep a steady beat.
func beats(times []time.Time, now time.Time) bool {
	var recent []time.Time
	for _, t := range times {
		if now.Sub(t) <= beatWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) < beatMin {
		return false
	}
	gaps := make([]time.Duration, 0, len(recent)-1)
	for i := 1; i < len(recent); i++ {
		gaps = append(gaps, recent[i].Sub(recent[i-1]))
	}
	sorted := slices.Clone(gaps)
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	if median < beatShortest || median > beatLongest {
		return false
	}
	steady := 0
	for _, g := range gaps {
		if diff := g - median; diff >= -median/4 && diff <= median/4 {
			steady++
		}
	}
	return float64(steady) >= beatSteady*float64(len(gaps))
}

// silent finds the devices that are online and asked for nothing for a day, once the watch itself has watched a day.
func silent(state models.PhoneHome, devices []InventoryDevice, now time.Time) []Signal {
	if state.Since.IsZero() || now.Sub(state.Since) < silentFor {
		return nil
	}
	last := map[string]time.Time{}
	for _, d := range state.Devices {
		last[d.MAC] = d.Last
	}
	var out []Signal
	for _, d := range devices {
		if now.Sub(d.LastSeen) > onlineWithin || now.Sub(d.FirstSeen) < silentFor {
			continue
		}
		if at, ok := last[d.MAC]; ok && now.Sub(at) < silentFor {
			continue
		}
		out = append(out, Signal{Kind: AroundDNS, MAC: d.MAC})
	}
	return out
}
