// Package radiodigest gives the assistant the Radio stack's digest as the radio_digest tool. The digest is counts, tiers and ids (Constitution
// I.16: no address and no name from a frame ever leaves the stack), but it arrives from another program, so nothing in it is trusted to be what
// it should: the core reads it into a typed shape of its own, lets through only the finding ids, tiers and kinds it knows, writes every claim
// from its own table (the stack offers ids, the core supplies the words, as for the alerts in core/radiowatch), counts what it left out, and
// puts the result in the untrusted-data fence. The core never imports a stack (IV.6): the answer arrives as bytes from a Source.
package radiodigest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/models"
)

// Source reads the stack's digest answer. It fails when the stack is not running.
type Source func(ctx context.Context) (json.RawMessage, error)

// Listing bounds: the digest is small by construction, but the shape is read from another program, so the lists are cut anyway.
const (
	maxEntries = 16
	maxCount   = 1_000_000
)

// claims are the words of each finding, in the order a report reads them (worst first). The ids and the words are held to the stack's by a test
// in the service (TestRadioDigestTool_TheCoreHasWordsForEveryFindingTheStackOffers).
var claims = []struct{ id, words string }{
	{"radio_rogue_twin", "An access point that you have not confirmed is broadcasting the name of one of your own networks. It may be a neighbour's, a repeater or an impostor."},
	{"radio_deauth_burst", "One sender transmitted a burst of deauthentication frames in the last two days. That is how devices are knocked off a network, and also what some routers do when they change channel."},
	{"radio_open_or_weak_ap", "One of your confirmed access points is open or uses an outdated protection (open, WEP or WPA)."},
	{"radio_pmf_off", "One of your confirmed WPA2 access points does not protect its management frames, so a device can be disconnected by a forged frame."},
	{"radio_join_foreign_network", "One of your devices tried to join a network that is not yours."},
	{"radio_hidden_ap_near", "A hidden network is close to the house."},
	{"radio_unknown_strong_device", "A device that no rule can place has a strong signal: it is close to the house and has been heard often enough to be sorted."},
	{"radio_sensor_blind", "The sensor has not finished a listening cycle in the last day, so nothing new is known about the air."},
}

var (
	tiers   = []string{"seen", "inferred", "estimated"}
	classes = []string{"infrastructure", "personal", "neighbour", "transient", "unknown"}
)

func wordsOf(id string) (string, bool) {
	for _, c := range claims {
		if c.id == id {
			return c.words, true
		}
	}
	return "", false
}

// Words is the core's wording of a finding id, so a test can hold it to the stack's.
func Words(id string) (string, bool) { return wordsOf(id) }

// FindingIDs lists the finding ids the core has words for, so a test can hold them to the stack's.
func FindingIDs() []string {
	ids := make([]string, len(claims))
	for i, c := range claims {
		ids[i] = c.id
	}
	return ids
}

// What is read of the stack's answer, field for field. The stack's own wording (`claim`, `reason`) is not read at all.
type heard struct {
	Listening struct {
		Cycles       int  `json:"cycles"`
		MinCycles    int  `json:"min_cycles"`
		CyclesLast24 int  `json:"cycles_last_24h"`
		Now          bool `json:"now"`
		HoursLeft    int  `json:"hours_left"`
	} `json:"listening"`
	Classifier struct {
		Version     int  `json:"version"`
		Provisional bool `json:"provisional"`
	} `json:"classifier"`
	Own struct {
		Devices   int `json:"devices"`
		APs       int `json:"access_points"`
		Confirmed int `json:"confirmed_access_points"`
	} `json:"own"`
	Others []struct {
		Class string `json:"class"`
		Count int    `json:"count"`
		Tier  string `json:"tier"`
	} `json:"others"`
	Findings []struct {
		ID    string `json:"id"`
		Tier  string `json:"tier"`
		Count int    `json:"count"`
	} `json:"findings"`
	CouldNotSee []struct {
		ID string `json:"id"`
	} `json:"could_not_see"`
}

// What the model is given: the same facts, every label checked and every claim in the core's words.
type kindCount struct {
	Class string `json:"class"`
	Count int    `json:"count"`
	Tier  string `json:"tier"`
}
type found struct {
	ID    string `json:"id"`
	Tier  string `json:"tier"`
	Count int    `json:"count"`
	Claim string `json:"claim"`
}
type told struct {
	Listening struct {
		Cycles       int  `json:"cycles"`
		MinCycles    int  `json:"min_cycles"`
		CyclesLast24 int  `json:"cycles_last_24h"`
		WatchNow     bool `json:"watch_running_now"`
		HoursLeft    int  `json:"watch_hours_left,omitempty"`
	} `json:"listening"`
	Classifier    string      `json:"classifier"`
	Own           string      `json:"own"`
	Others        []kindCount `json:"other_peoples_devices_by_kind"`
	Findings      []found     `json:"findings"`
	CouldNotRun   []string    `json:"checks_that_could_not_run,omitempty"`
	NotRecognised int         `json:"entries_left_out,omitempty"`
}

// Module builds the tool module. available says whether the Radio stack is switched on: the tool is not offered while it is off.
func Module(src Source, available func(ctx context.Context) bool) toolregistry.Module {
	return toolregistry.Module{
		Handlers: map[string]toolregistry.ToolHandler{
			models.ToolRadioDigest: toolregistry.Handle(func(ctx context.Context, _ struct{}) (any, error) {
				return summarize(ctx, src)
			}),
		},
		Available: available,
	}
}

func summarize(ctx context.Context, src Source) (string, error) {
	raw, err := src(ctx)
	if err != nil {
		return "", fmt.Errorf("the radio sensor could not be read: %w", err)
	}
	var h heard
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", fmt.Errorf("the radio sensor's answer could not be read: %w", err)
	}
	t := tell(h)
	body, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode the radio digest: %w", err)
	}
	return header(t) + "\n" + prompts.FenceScanData(string(body)), nil
}

func clamp(n int) int { return max(0, min(n, maxCount)) }

// tell keeps what the core knows how to word and counts the rest. A finding with an id or a tier it does not know is dropped, not shown with the
// stack's own words.
func tell(h heard) told {
	var t told
	t.Listening.Cycles, t.Listening.MinCycles, t.Listening.CyclesLast24 = clamp(h.Listening.Cycles), clamp(h.Listening.MinCycles), clamp(h.Listening.CyclesLast24)
	t.Listening.WatchNow, t.Listening.HoursLeft = h.Listening.Now, clamp(h.Listening.HoursLeft)
	t.Classifier = fmt.Sprintf("kinds of device are sorted by rules version %d%s", max(h.Classifier.Version, 0), map[bool]string{true: ", which are provisional: a first guess, not a fitted result", false: ""}[h.Classifier.Provisional])
	t.Own = fmt.Sprintf("%d of the owner's devices, %d access points (%d confirmed)", clamp(h.Own.Devices), clamp(h.Own.APs), clamp(h.Own.Confirmed))
	t.Others, t.Findings = []kindCount{}, []found{}
	for _, o := range h.Others {
		if !slices.Contains(classes, o.Class) || !slices.Contains(tiers, o.Tier) || len(t.Others) >= maxEntries {
			t.NotRecognised++
			continue
		}
		t.Others = append(t.Others, kindCount{o.Class, clamp(o.Count), o.Tier})
	}
	for _, f := range h.Findings {
		words, ok := wordsOf(f.ID)
		if !ok || !slices.Contains(tiers, f.Tier) || len(t.Findings) >= maxEntries {
			t.NotRecognised++
			continue
		}
		t.Findings = append(t.Findings, found{f.ID, f.Tier, clamp(f.Count), words})
	}
	for _, c := range h.CouldNotSee {
		if _, ok := wordsOf(c.ID); !ok || len(t.CouldNotRun) >= maxEntries {
			t.NotRecognised++
			continue
		}
		t.CouldNotRun = append(t.CouldNotRun, c.ID)
	}
	return t
}

// header is fixed wording and numbers only. It says how much listening the claims rest on, and whether a watch is running, before it says what
// was found, because a short run finds nothing and that is not a quiet air.
func header(t told) string {
	var b strings.Builder
	l := t.Listening
	fmt.Fprintf(&b, "radio_digest: what the Wi-Fi listening sensor knows, from %d listening cycles (%d in the last 24 hours; a device is not sorted before %d). ", l.Cycles, l.CyclesLast24, l.MinCycles)
	if l.WatchNow {
		fmt.Fprintf(&b, "A watch is running now (%d hours left). ", l.HoursLeft)
	} else {
		b.WriteString("No watch is running now, so this rests on what was heard earlier. ")
	}
	if l.Cycles < l.MinCycles {
		b.WriteString("There has been too little listening to sort devices or call the air quiet. ")
	}
	b.WriteString(t.Classifier + ". ")
	switch n := len(t.Findings); n {
	case 0:
		b.WriteString("No findings. ")
	case 1:
		b.WriteString("1 finding. ")
	default:
		fmt.Fprintf(&b, "%d findings. ", n)
	}
	if len(t.CouldNotRun) > 0 {
		b.WriteString("Some checks could not run, and that is not a clean result. ")
	}
	if t.NotRecognised > 0 {
		fmt.Fprintf(&b, "%d entries from the sensor were not recognised and are left out. ", t.NotRecognised)
	}
	b.WriteString("Tiers: seen is heard directly, inferred comes from patterns, estimated is rough. Position is never known, only a range. This is counts and fixed wording: there are no addresses or names to report.")
	return b.String()
}
