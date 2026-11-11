package phonehome

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// MaxLookupNames bounds the names one device's row carries: the newest are the ones a person wants.
const MaxLookupNames = 128

// lookupWindow is what "today" means in the device row.
const lookupWindow = 24 * time.Hour

// Lookups is the screen's view of the watch: one row per device that asked for anything, the busiest first, with the signals' flags. It is pure
// and reads only the names and the signals; nothing is looked up and no name leaves this computer (Constitution I.8).
func Lookups(state models.PhoneHome, signals []Signal, now time.Time) []models.DeviceLookups {
	flags := map[string]map[string]string{} // mac -> name -> the most serious flag for that name
	kinds := map[string][]string{}          // mac -> flags of the device, in the order signals arrive
	for _, s := range signals {
		kinds[s.MAC] = appendUnique(kinds[s.MAC], string(s.Kind))
		for _, n := range s.Names {
			if flags[s.MAC] == nil {
				flags[s.MAC] = map[string]string{}
			}
			if rank(string(s.Kind)) > rank(flags[s.MAC][n]) {
				flags[s.MAC][n] = string(s.Kind)
			}
		}
	}
	seen := map[string]bool{}
	out := make([]models.DeviceLookups, 0, len(state.Devices))
	for _, d := range state.Devices {
		row := models.DeviceLookups{MAC: d.MAC, First: d.First, Last: d.Last, Total: len(d.Names), Flags: kinds[d.MAC], Names: []models.DeviceLookupName{}}
		if row.Flags == nil {
			row.Flags = []string{}
		}
		names := slices.Clone(d.Names)
		slices.SortStableFunc(names, func(a, b models.PhoneHomeName) int { return b.Last.Compare(a.Last) })
		for _, n := range names {
			if now.Sub(n.Last) <= lookupWindow {
				row.Today++
			}
			if len(row.Names) < MaxLookupNames {
				row.Names = append(row.Names, models.DeviceLookupName{Name: n.Name, Domain: Registrable(n.Name), Last: n.Last, Count: n.Count, Flag: flags[d.MAC][n.Name]})
			}
		}
		seen[d.MAC] = true
		out = append(out, row)
	}
	// A device that asks for nothing has no names, but its flag is the point: it is listed with none.
	for mac, k := range kinds {
		if !seen[mac] {
			out = append(out, models.DeviceLookups{MAC: mac, Flags: k, Names: []models.DeviceLookupName{}})
		}
	}
	slices.SortStableFunc(out, func(a, b models.DeviceLookups) int {
		return cmp.Or(cmp.Compare(len(b.Flags), len(a.Flags)), cmp.Compare(b.Today, a.Today), cmp.Compare(a.MAC, b.MAC))
	})
	return out
}

func appendUnique(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// rank orders the flags a name can carry, the most serious first being highest.
func rank(flag string) int {
	switch Kind(flag) {
	case KnownBadName:
		return 4
	case EncryptedDNSName:
		return 3
	case Beaconing:
		return 2
	case NewPlace:
		return 1
	}
	return 0
}

// twoPartSuffixes are the common public suffixes of two labels, so that bbc.co.uk is one domain and not "co.uk". It is a short list judged by
// eye, not the public suffix list: a name under a suffix it lacks is grouped one label too high, which only merges rows.
var twoPartSuffixes = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true, "me.uk": true, "ltd.uk": true, "plc.uk": true, "net.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true, "gov.au": true,
	"co.nz": true, "co.jp": true, "ne.jp": true, "or.jp": true, "co.kr": true, "co.in": true, "co.za": true, "co.il": true,
	"com.br": true, "com.cn": true, "com.hk": true, "com.tw": true, "com.sg": true, "com.mx": true, "com.ar": true, "com.tr": true,
}

// Registrable is the part of a DNS name a person recognises: its last two labels, or three under a common two-part suffix. A name that is
// already that short, or an address-like name, is returned as it is.
func Registrable(name string) string {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	labels := strings.Split(name, ".")
	if len(labels) <= 2 {
		return name
	}
	keep := 2
	if twoPartSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		keep = 3
	}
	if len(labels) <= keep {
		return name
	}
	return strings.Join(labels[len(labels)-keep:], ".")
}
