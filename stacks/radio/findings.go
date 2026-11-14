package radio

import (
	"context"
	"net/http"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// The digest (plan §7): what the stack has found, in a form a person or an agent can read without trusting anything a device sent. Every field is a
// number, a fixed word from a table in this file, or a class name; there is no field for a network name, a probe name, an address or a tag, so a
// device in range has no way to put text into it (Constitution I.8, I.16). Other people's devices are counts by class only. Each claim carries
// how it is known: seen (counted directly), inferred (a rule over counts) or estimated (a signal band). The list of what it could not see says
// which checks are missing, so silence is never read as safety.
const (
	// DigestPath answers the digest (GET).
	DigestPath = "/admin/api/radio/digest"

	// deauthBurstPerDay is how many deauthentication frames one sender may send in a day before it is a burst. A provisional number, like the
	// classifier's: nothing has been fitted to a measured attack yet.
	deauthBurstPerDay = 50
	// blindAfterHours is how long the sensor may go without finishing a cycle, once it has listened, before it is said to be blind.
	blindAfterHours = 24
)

// How a claim is known.
const (
	TierSeen      = "seen"
	TierInferred  = "inferred"
	TierEstimated = "estimated"
)

// The protection states of management frames.
const (
	PMFOff      = "off"
	PMFCapable  = "capable"
	PMFRequired = "required"
)

// The finding ids of plan §7. The stack offers ids; the wording is this table's, byte for byte (ADR-0036, ADR-0038 rule 1).
var findingIDs = []string{
	"radio_rogue_twin", "radio_deauth_burst", "radio_open_or_weak_ap", "radio_pmf_off",
	"radio_hidden_ap_near", "radio_unknown_strong_device", "radio_join_foreign_network", "radio_sensor_blind",
}

var claimWords = map[string]string{
	"radio_rogue_twin":            "An access point that you have not confirmed is broadcasting the name of one of your own networks. It may be a neighbour's, a repeater or an impostor.",
	"radio_deauth_burst":          "One sender transmitted a burst of deauthentication frames in the last two days. That is how devices are knocked off a network, and also what some routers do when they change channel.",
	"radio_open_or_weak_ap":       "One of your confirmed access points is open or uses an outdated protection (open, WEP or WPA).",
	"radio_pmf_off":               "One of your confirmed WPA2 access points does not protect its management frames, so a device can be disconnected by a forged frame.",
	"radio_hidden_ap_near":        "A hidden network is close to the house.",
	"radio_unknown_strong_device": "A device that no rule can place has a strong signal: it is close to the house and has been heard often enough to be sorted.",
	"radio_join_foreign_network":  "One of your devices tried to join a network that is not yours.",
	"radio_sensor_blind":          "The sensor has not finished a listening cycle in the last day, so nothing new is known about the air.",
}

// couldNotSee is why a check cannot run, as a fixed phrase, keyed by finding id. A check that cannot run is not a clean result. Every check of plan
// section 7 has its evidence now, so the table is empty; a check added later without evidence is listed here instead of being left out.
var couldNotSee = map[string]string{}

// DigestFinding is one finding: an id, how it is known, how many, and its fixed wording.
type DigestFinding struct {
	ID    string `json:"id"`
	Tier  string `json:"tier"`
	Count int    `json:"count"`
	Claim string `json:"claim"`
}

// DigestUnseen is a check that could not run, and why.
type DigestUnseen struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// DigestOthers is how many devices of other people are of a class: a count, never a device.
type DigestOthers struct {
	Class string `json:"class"`
	Count int    `json:"count"`
	Tier  string `json:"tier"`
}

// DigestListening says how much listening the claims rest on.
type DigestListening struct {
	Cycles       int `json:"cycles"`
	MinCycles    int `json:"min_cycles"`
	WindowDays   int `json:"window_days"`
	CyclesLast24 int `json:"cycles_last_24h"`
}

// DigestOwn counts what is the owner's.
type DigestOwn struct {
	Devices              int `json:"devices"`
	AccessPoints         int `json:"access_points"`
	ConfirmedAccessPoint int `json:"confirmed_access_points"`
}

// Digest is the answer of DigestPath.
type Digest struct {
	Listening   DigestListening `json:"listening"`
	Own         DigestOwn       `json:"own"`
	Others      []DigestOthers  `json:"others"`
	Findings    []DigestFinding `json:"findings"`
	CouldNotSee []DigestUnseen  `json:"could_not_see"`
}

func (s *Stack) handleDigest(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if req.Method != http.MethodGet || req.Path != DigestPath {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	_, store, err := s.recorder(h)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the radio database could not be opened")
	}
	d, err := s.digest(ctx, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the findings could not be read")
	}
	return jsonResponse(http.StatusOK, d)
}

func (s *Stack) digest(ctx context.Context, store *Store) (Digest, error) {
	now := s.now()
	rules, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return Digest{}, err
	}
	d := Digest{Others: []DigestOthers{}, Findings: []DigestFinding{}, CouldNotSee: []DigestUnseen{}}
	hours, err := store.airHours(ctx, 0)
	if err != nil {
		return d, err
	}
	d.Listening = listeningOf(rules, listened, hours, hourNumber(now))
	if d.Others, err = othersOf(ctx, store); err != nil {
		return d, err
	}
	aps, err := store.OwnAPs(ctx)
	if err != nil {
		return d, err
	}
	owners, err := store.OwnerDevices(ctx)
	if err != nil {
		return d, err
	}
	seen, err := recentEvidence(ctx, store, now, rules)
	if err != nil {
		return d, err
	}
	twins, weak, noPMF, confirmed := ownAPFindings(aps)
	d.Own = DigestOwn{Devices: len(owners), AccessPoints: len(aps), ConfirmedAccessPoint: confirmed}
	counts := map[string]DigestFinding{}
	for id, c := range map[string]struct {
		tier string
		n    int
	}{
		"radio_rogue_twin":            {TierSeen, twins},
		"radio_open_or_weak_ap":       {TierSeen, weak},
		"radio_pmf_off":               {TierSeen, noPMF},
		"radio_deauth_burst":          {TierSeen, seen.bursts},
		"radio_unknown_strong_device": {TierInferred, unknownStrong(rules, rows, listened, d.Listening)},
		"radio_sensor_blind":          {TierSeen, blindCount(hours, d.Listening)},
		"radio_join_foreign_network":  {TierSeen, seen.foreign},
		"radio_hidden_ap_near":        {TierEstimated, seen.hidden},
	} {
		if c.n > 0 {
			counts[id] = DigestFinding{ID: id, Tier: c.tier, Count: c.n, Claim: claimWords[id]}
		}
	}
	for _, id := range findingIDs { // the table's order, so the same evidence reads the same
		if f, ok := counts[id]; ok {
			d.Findings = append(d.Findings, f)
		}
		if why, ok := couldNotSee[id]; ok {
			d.CouldNotSee = append(d.CouldNotSee, DigestUnseen{ID: id, Reason: why})
		}
	}
	return d, nil
}

// evidence is what the last two days' counts say, read from the store.
type evidence struct{ bursts, foreign, hidden int }

func recentEvidence(ctx context.Context, store *Store, now time.Time, rules Rules) (evidence, error) {
	since := dayNumber(now) - 1
	var e evidence
	var err error
	if e.bursts, err = store.deauthSenders(ctx, since, deauthBurstPerDay); err != nil {
		return e, err
	}
	if e.foreign, err = store.ownerEventDevices(ctx, eventJoinForeign, since); err != nil {
		return e, err
	}
	e.hidden, err = store.hiddenNear(ctx, since, rules.StrongDBm)
	return e, err
}

// listeningOf says how much listening the claims rest on: the cycles in the rules' window and in the last day.
func listeningOf(rules Rules, listened map[int64]int, hours map[int64]int, hour int64) DigestListening {
	l := DigestListening{MinCycles: rules.MinCyclesListened, WindowDays: rules.WindowDays}
	for _, n := range listened {
		l.Cycles += n
	}
	for h, n := range hours {
		if h > hour-blindAfterHours {
			l.CyclesLast24 += n
		}
	}
	return l
}

// blindCount is 1 when the sensor has listened before and finished no cycle in the last day.
func blindCount(hours map[int64]int, l DigestListening) int {
	if len(hours) > 0 && l.CyclesLast24 == 0 {
		return 1
	}
	return 0
}

// othersOf counts other people's devices by class; one the rules have not looked at yet is unsorted.
func othersOf(ctx context.Context, store *Store) ([]DigestOthers, error) {
	counts, err := store.ClassCounts(ctx)
	if err != nil {
		return nil, err
	}
	counts[string(ClassUnknown)] += counts[""]
	out := []DigestOthers{}
	for _, c := range Classes {
		if n := counts[string(c)]; n > 0 {
			out = append(out, DigestOthers{Class: string(c), Count: n, Tier: TierInferred})
		}
	}
	return out, nil
}

// ownAPFindings reads the owner's access points: twins (an unconfirmed access point under a name that has a confirmed one), weak protection and
// missing management-frame protection, the last two on confirmed ones only (an unconfirmed one is not the owner's). It also counts the confirmed.
func ownAPFindings(aps []OwnAP) (twins, weak, noPMF, confirmed int) {
	named := map[string]bool{}
	for _, a := range aps {
		if a.Confirmed {
			named[a.Name] = true
			confirmed++
		}
	}
	weakModes := []string{string(SecOpen), string(SecWEP), string(SecWPA)}
	for _, a := range aps {
		switch {
		case !a.Confirmed && named[a.Name]:
			twins++
		case a.Confirmed && slices.Contains(weakModes, a.Security):
			weak++
		case a.Confirmed && a.Security == string(SecWPA2) && a.PMF == PMFOff:
			noPMF++
		}
	}
	return twins, weak, noPMF, confirmed
}

// unknownStrong counts devices that no rule can place although the sensor has listened enough to sort them, and whose signal is strong.
func unknownStrong(rules Rules, rows []FeatureRow, listened map[int64]int, l DigestListening) int {
	if l.Cycles < l.MinCycles {
		return 0
	}
	n := 0
	for _, r := range rows {
		if r.Owner {
			continue
		}
		f := FeaturesOf(r, listened)
		if cls, why := rules.Classify(f); cls == ClassUnknown && why == WhyNoRuleFits && signalBand(rules, f) == bandStrong {
			n++
		}
	}
	return n
}

// deauthSenders counts the senders with at least min deauthentication frames from sinceDay on.
func (s *Store) deauthSenders(ctx context.Context, sinceDay int64, min int) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT tag FROM events WHERE kind = 'deauthentication' AND day >= ? GROUP BY tag HAVING sum(n) >= ?)`, sinceDay, min).Scan(&n)
	return n, err
}

// ownerEventDevices counts the owner's devices with at least one event of the kind from sinceDay on.
func (s *Store) ownerEventDevices(ctx context.Context, kind string, sinceDay int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(DISTINCT tag) FROM events WHERE kind = ? AND owner = 1 AND day >= ?`, kind, sinceDay).Scan(&n)
	return n, err
}

// hiddenNear counts hidden networks (a zero-length name) of other people heard from sinceDay on at a signal of at least strongDBm.
func (s *Store) hiddenNear(ctx context.Context, sinceDay int64, strongDBm int) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM beacons WHERE own = 0 AND name_len = 0 AND last_day >= ? AND sig_max >= ?`, sinceDay, strongDBm).Scan(&n)
	return n, err
}
