package radio

import (
	"context"
	"maps"
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
	"radio_hidden_ap_near", "radio_join_foreign_network", "radio_sensor_blind",
}

// FindingIDs lists every finding id the digest can carry, so a test in the service can hold the core's table of words to it.
func FindingIDs() []string { return slices.Clone(findingIDs) }

// FindingWords is the stack's wording of each finding id, so the same test can hold the core's own table to it word for word.
func FindingWords() map[string]string { return maps.Clone(claimWords) }

// FindingMeanings is the stack's "what this usually means" line of each finding id, for the same parity test.
func FindingMeanings() map[string]string { return maps.Clone(meaningWords) }

// claimWords says what the evidence is, in words that do not depend on how many (the count is shown beside them): "access points", never "one access
// point" beside a 3.
var claimWords = map[string]string{
	"radio_rogue_twin":           "Access points that you have not confirmed are broadcasting the name of one of your own networks. They may be a neighbour's, a repeater or an impostor.",
	"radio_deauth_burst":         "Senders that transmitted a burst of deauthentication frames in the last two days. That is how devices are knocked off a network, and also what some routers do when they change channel.",
	"radio_open_or_weak_ap":      "Confirmed access points of yours that are open or use an outdated protection (open, WEP or WPA).",
	"radio_pmf_off":              "Confirmed WPA2 access points of yours that do not protect their management frames, so a device can be disconnected by a forged frame.",
	"radio_hidden_ap_near":       "Hidden networks that are close to the house.",
	"radio_join_foreign_network": "Devices of yours that tried to join a network that is not yours.",
	"radio_sensor_blind":         "The sensor has not finished a listening cycle in the last day, so nothing new is known about the air.",
}

// meaningWords is what a finding usually means, in fixed hedged words, so a report or the page can say how much it matters without anyone improvising.
// None names a router menu or promises a fix it cannot know about. They are the owner's to approve; the core holds the same table for the assistant.
var meaningWords = map[string]string{
	"radio_rogue_twin":           "A neighbour's network or a repeater can share a name, so these are candidates, not proof. Under Your networks you confirm which access points are yours.",
	"radio_deauth_burst":         "A router that changes channel can do this too, so one burst is not proof of an attack. Repeated bursts from the same sender are worth a look.",
	"radio_open_or_weak_ap":      "These protections are weak or absent. Most routers offer WPA2 or WPA3 in their wireless settings.",
	"radio_pmf_off":              "Common on home routers and low risk on its own: someone nearby could disconnect a device, not read what it sends. Where the router offers protected management frames (802.11w), turning it on closes this.",
	"radio_hidden_ap_near":       "Hidden networks are common (some routers, printers and smart-home devices hide theirs) and are not a sign of attack on their own.",
	"radio_join_foreign_network": "Phones and laptops often try networks they have seen before, such as a café or a neighbour's. It is worth a look only if it keeps happening.",
	"radio_sensor_blind":         "Start a watch on the Listening tab. The adapter may also have been unplugged.",
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
	// Means is what it usually means and how much it matters, in fixed words.
	Means string `json:"means"`
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
	// Now says a watch is running at this moment, and HoursLeft how long it has (whole hours, at least one while it runs). A report made with
	// no watch running rests on what was kept earlier.
	Now       bool `json:"now"`
	HoursLeft int  `json:"hours_left,omitempty"`
}

// DigestClassifier says which rules sorted the devices and that they are provisional (a guess, not a fitted result), so a report words the
// kinds the way the screen does.
type DigestClassifier struct {
	Version     int  `json:"version"`
	Provisional bool `json:"provisional"`
}

// DigestOwn counts what is the owner's.
type DigestOwn struct {
	Devices              int `json:"devices"`
	AccessPoints         int `json:"access_points"`
	ConfirmedAccessPoint int `json:"confirmed_access_points"`
}

// Digest is the answer of DigestPath.
type Digest struct {
	Listening   DigestListening  `json:"listening"`
	Classifier  DigestClassifier `json:"classifier"`
	Own         DigestOwn        `json:"own"`
	Others      []DigestOthers   `json:"others"`
	Findings    []DigestFinding  `json:"findings"`
	CouldNotSee []DigestUnseen   `json:"could_not_see"`
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
	d, err := s.digest(ctx, h, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the findings could not be read")
	}
	return jsonResponse(http.StatusOK, d)
}

func (s *Stack) digest(ctx context.Context, h stacksdk.Host, store *Store) (Digest, error) {
	now := s.now()
	rules, _, listened, err := s.airRows(ctx, store)
	if err != nil {
		return Digest{}, err
	}
	d := Digest{Others: []DigestOthers{}, Findings: []DigestFinding{}, CouldNotSee: []DigestUnseen{}}
	hours, err := store.airHours(ctx, 0)
	if err != nil {
		return d, err
	}
	d.Listening = s.listeningNow(ctx, h, listeningOf(rules, listened, hours, hourNumber(now)), now)
	d.Classifier = DigestClassifier{Version: rules.Version, Provisional: rules.Provisional}
	if d.Others, err = othersOf(ctx, store); err != nil {
		return d, err
	}
	aps, err := s.ownAPsNow(ctx, h)
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
		"radio_rogue_twin":           {TierSeen, twins},
		"radio_open_or_weak_ap":      {TierSeen, weak},
		"radio_pmf_off":              {TierSeen, noPMF},
		"radio_deauth_burst":         {TierSeen, seen.bursts},
		"radio_sensor_blind":         {TierSeen, blindCount(hours, d.Listening)},
		"radio_join_foreign_network": {TierSeen, seen.foreign},
		"radio_hidden_ap_near":       {TierEstimated, seen.hidden},
	} {
		if c.n > 0 {
			counts[id] = DigestFinding{ID: id, Tier: c.tier, Count: c.n, Claim: claimWords[id], Means: meaningWords[id]}
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

// listeningNow adds to what the listening rests on whether a watch is running at this moment and how many whole hours it has left.
func (s *Stack) listeningNow(ctx context.Context, h stacksdk.Host, l DigestListening, now time.Time) DigestListening {
	if st := s.listenState(ctx, h); st.Listening {
		l.Now, l.HoursLeft = true, hoursLeft(st.Until.Sub(now))
	}
	return l
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

// ownAPsNow is the access points heard under the owner's names, marked from the core's list as it is now.
func (s *Stack) ownAPsNow(ctx context.Context, h stacksdk.Host) ([]OwnAP, error) {
	rec, _, err := s.recorder(h)
	if err != nil {
		return nil, err
	}
	_ = rec.SyncOwnNetworks(ctx) // a failed read keeps the last list
	return rec.OwnAPs(ctx)
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

// eventCounts is how many events of the kind each device had from sinceDay on, by tag.
func (s *Store) eventCounts(ctx context.Context, kind string, sinceDay int64) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag, sum(n) FROM events WHERE kind = ? AND day >= ? GROUP BY tag`, kind, sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var tag []byte
		var n int
		if err := rows.Scan(&tag, &n); err != nil {
			return nil, err
		}
		out[string(tag)] = n
	}
	return out, rows.Err()
}

// hiddenNear counts hidden networks (a zero-length name) of other people heard from sinceDay on at a signal of at least strongDBm.
func (s *Store) hiddenNear(ctx context.Context, sinceDay int64, strongDBm int) (int, error) {
	var n int
	// A max above maxReceivedDBm is a "no reading" chain stored before the parser dropped them, not a strong beacon.
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM beacons WHERE own = 0 AND name_len = 0 AND last_day >= ? AND sig_max >= ? AND sig_max <= ?`, sinceDay, strongDBm, maxReceivedDBm).Scan(&n)
	return n, err
}
