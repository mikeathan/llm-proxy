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

// claims are the words of each finding, in the order a report reads them (worst first): what the evidence is, and what it usually means and how much
// it matters, both fixed and hedged. The ids and the words are held to the stack's by a test in the service
// (TestRadioDigestTool_TheCoreHasWordsForEveryFindingTheStackOffers). The owner approves them.
var claims = []struct{ id, words, means string }{
	{"radio_rogue_twin", "Access points that you have not confirmed are broadcasting the name of one of your own networks. They may be a neighbour's, a repeater or an impostor.",
		"A neighbour's network or a repeater can share a name, so these are candidates, not proof. Under Your networks you confirm which access points are yours."},
	{"radio_deauth_burst", "Senders that transmitted a burst of deauthentication frames in the last two days. That is how devices are knocked off a network, and also what some routers do when they change channel.",
		"A router that changes channel can do this too, so one burst is not proof of an attack. Repeated bursts from the same sender are worth a look."},
	{"radio_open_or_weak_ap", "Confirmed access points of yours that are open or use an outdated protection (open, WEP or WPA).",
		"These protections are weak or absent. Most routers offer WPA2 or WPA3 in their wireless settings."},
	{"radio_pmf_off", "Confirmed WPA2 access points of yours that do not protect their management frames, so a device can be disconnected by a forged frame.",
		"Common on home routers and low risk on its own: someone nearby could disconnect a device, not read what it sends. Where the router offers protected management frames (802.11w), turning it on closes this."},
	{"radio_join_foreign_network", "Devices of yours that tried to join a network that is not yours.",
		"Phones and laptops often try networks they have seen before, such as a café or a neighbour's. It is worth a look only if it keeps happening."},
	{"radio_hidden_ap_near", "Hidden networks that are close to the house.",
		"Hidden networks are common (some routers, printers and smart-home devices hide theirs) and are not a sign of attack on their own."},
	{"radio_sensor_blind", "The sensor has not finished a listening cycle in the last day, so nothing new is known about the air.",
		"Start a watch on the Listening tab. The adapter may also have been unplugged."},
}

var (
	tiers   = []string{"seen", "inferred", "estimated"}
	classes = []string{"infrastructure", "personal", "neighbour", "transient", "unknown"}
)

func wordsOf(id string) (words, means string, ok bool) {
	for _, c := range claims {
		if c.id == id {
			return c.words, c.means, true
		}
	}
	return "", "", false
}

// Words is the core's wording of a finding id and its "what this usually means" line, so a test can hold them to the stack's.
func Words(id string) (words, means string, ok bool) { return wordsOf(id) }

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
	Means string `json:"what_it_usually_means"`
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
	OthersTotal   int         `json:"other_peoples_devices_total"`
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
	t.Classifier = fmt.Sprintf("kinds of device are sorted by rule set number %d", max(h.Classifier.Version, 0))
	if h.Classifier.Provisional {
		t.Classifier = "kinds of device are sorted by a first-guess (provisional) rule set, not yet fitted to this house's air"
	}
	t.Own = fmt.Sprintf("%d of the owner's devices, %d access points (%d confirmed)", clamp(h.Own.Devices), clamp(h.Own.APs), clamp(h.Own.Confirmed))
	t.Others, t.OthersTotal, t.NotRecognised = tellOthers(h)
	var dropped int
	t.Findings, dropped = tellFindings(h)
	t.NotRecognised += dropped
	t.CouldNotRun, dropped = tellUnseen(h)
	t.NotRecognised += dropped
	return t
}

// tellOthers keeps the counts of other people's devices whose kind and tier the core knows, with their total and how many it left out.
func tellOthers(h heard) (kept []kindCount, total, dropped int) {
	kept = []kindCount{}
	for _, o := range h.Others {
		if !slices.Contains(classes, o.Class) || !slices.Contains(tiers, o.Tier) || len(kept) >= maxEntries {
			dropped++
			continue
		}
		kept = append(kept, kindCount{o.Class, clamp(o.Count), o.Tier})
		total += clamp(o.Count)
	}
	return kept, total, dropped
}

// tellFindings words each finding the core knows from its own table, and counts the ones it does not.
func tellFindings(h heard) (kept []found, dropped int) {
	kept = []found{}
	for _, f := range h.Findings {
		words, means, ok := wordsOf(f.ID)
		if !ok || !slices.Contains(tiers, f.Tier) || len(kept) >= maxEntries {
			dropped++
			continue
		}
		kept = append(kept, found{f.ID, f.Tier, clamp(f.Count), words, means})
	}
	return kept, dropped
}

// Finding is one finding of the digest the core can word: its id, the tier of evidence, a clamped count and the core's own sentence for the id.
type Finding struct {
	ID    string
	Tier  string
	Count int
	Words string
}

// Findings reads the stack's digest answer and returns only the findings the core has words for, in the core's words and with clamped counts, at most
// maxEntries. Anything else in the answer, including the stack's own wording, is ignored.
func Findings(raw json.RawMessage) ([]Finding, error) {
	var h heard
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, fmt.Errorf("the radio sensor's answer could not be read: %w", err)
	}
	kept, _ := tellFindings(h)
	out := make([]Finding, len(kept))
	for i, f := range kept {
		out[i] = Finding{ID: f.ID, Tier: f.Tier, Count: f.Count, Words: f.Claim}
	}
	return out, nil
}

// tellUnseen keeps the ids of checks that could not run, when the core knows the check.
func tellUnseen(h heard) (kept []string, dropped int) {
	for _, c := range h.CouldNotSee {
		if _, _, ok := wordsOf(c.ID); !ok || len(kept) >= maxEntries {
			dropped++
			continue
		}
		kept = append(kept, c.ID)
	}
	return kept, dropped
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
	if t.OthersTotal > 0 {
		parts := make([]string, len(t.Others))
		for i, o := range t.Others {
			parts[i] = fmt.Sprintf("%d %s", o.Count, o.Class)
		}
		fmt.Fprintf(&b, "%d devices of other people are counted, by kind: %s. ", t.OthersTotal, strings.Join(parts, ", "))
	}
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
