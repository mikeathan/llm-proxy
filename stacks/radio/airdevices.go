package radio

import (
	"context"
	"encoding/hex"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// What the air holds, one step finer than counts, and what the owner's own devices did hour by hour (plan §8). Everything here is read from rows
// the recorder already wrote. About other people it gives a short id (four bytes of the keyed tag, which is not an address and cannot be turned
// into one), a class, a signal band and counts; never an address, never a full tag, never a name.
const (
	// AirDevicesPath lists other people's devices (GET ?class=&sort=).
	AirDevicesPath = "/admin/api/radio/air/devices"
	// AirwavesPath answers how the channels and the classes use the air (GET).
	AirwavesPath = "/admin/api/radio/air/airwaves"
	// PresencePath answers the owner's devices by hour (GET).
	PresencePath = "/admin/api/radio/devices/mine/presence"

	maxAirDevices = 200
	shortIDBytes  = 4
	// presenceHours is the length of the strip of hours the owner's devices are shown over.
	presenceHours = 48
)

// The signal bands, from a device's mean signal against the rules' thresholds.
const (
	bandStrong = "strong"
	bandMedium = "medium"
	bandWeak   = "weak"
	bandNone   = "unknown"
)

// The sorts a list may ask for.
const (
	sortLastHeard = "last_heard"
	sortCycles    = "cycles"
	sortSignal    = "signal"
	sortRetries   = "retries"
)

var airSorts = []string{sortLastHeard, sortCycles, sortSignal, sortRetries}

// AirDevice is one device of someone else, as far as the evidence goes.
type AirDevice struct {
	ID    string `json:"id"` // eight hex digits of the keyed tag
	Class string `json:"class"`
	Why   string `json:"why"`
	// Signal is strong, medium, weak or unknown, against the rules' thresholds.
	Signal       string `json:"signal"`
	FirstSeen    string `json:"first_seen"`
	LastSeen     string `json:"last_seen"`
	CyclesHeard  int    `json:"cycles_heard"`
	NeedHeard    int    `json:"need_heard"`
	RetryPercent int    `json:"retry_percent"`
	Channels     []int  `json:"channels"`
	Random       bool   `json:"random_address"`
	JoinedOwn    bool   `json:"joined_own"`
}

// AirDevices is the answer of AirDevicesPath.
type AirDevices struct {
	// Listened is how many cycles the sensor listened through in the rules' window and MinListened how many it needs before it sorts anyone.
	Listened    int         `json:"listened_cycles"`
	MinListened int         `json:"min_listened_cycles"`
	Total       int         `json:"total"`
	Devices     []AirDevice `json:"devices"`
}

func signalBand(r Rules, f Features) string {
	switch {
	case !f.HasSignal:
		return bandNone
	case f.SignalMean >= float64(r.StrongDBm):
		return bandStrong
	case f.SignalMean <= float64(r.WeakDBm):
		return bandWeak
	}
	return bandMedium
}

func percent(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	return int(math.Round(100 * float64(part) / float64(whole)))
}

func shortID(tag []byte) string { return hex.EncodeToString(tag[:min(len(tag), shortIDBytes)]) }

// airRows reads every neighbour's features over the rules' window.
func (s *Stack) airRows(ctx context.Context, store *Store) (Rules, []FeatureRow, map[int64]int, error) {
	now := s.now()
	rules, err := store.CurrentRules(ctx, now)
	if err != nil {
		return rules, nil, nil, err
	}
	rows, listened, err := store.FeatureRows(ctx, dayNumber(now)-int64(rules.WindowDays)+1)
	return rules, rows, listened, err
}

func (s *Stack) airDevices(ctx context.Context, store *Store, query string) (stacksdk.Response, error) {
	q, err := url.ParseQuery(query)
	class, sortBy := q.Get("class"), q.Get("sort")
	if sortBy == "" {
		sortBy = sortLastHeard
	}
	if err != nil || (class != "" && !slices.Contains(Classes, Class(class))) || !slices.Contains(airSorts, sortBy) {
		return plainError(http.StatusBadRequest, "ask for a known class and sort")
	}
	rules, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	v := AirDevices{MinListened: rules.MinCyclesListened, Devices: []AirDevice{}}
	for _, n := range listened {
		v.Listened += n
	}
	var out []AirDevice
	for _, r := range rows {
		if r.Owner {
			continue
		}
		d := airDevice(rules, r, listened)
		if class == "" || d.Class == class {
			out = append(out, d)
		}
	}
	v.Total = len(out)
	sortAir(out, sortBy)
	v.Devices = append(v.Devices, out[:min(len(out), maxAirDevices)]...)
	return jsonResponse(http.StatusOK, v)
}

func airDevice(rules Rules, r FeatureRow, listened map[int64]int) AirDevice {
	f := FeaturesOf(r, listened)
	_, why := rules.Classify(f)
	class := r.Class
	if class == "" {
		class = string(ClassUnknown)
	}
	if r.Cycles == 0 && r.Class != "" && r.Class != string(ClassUnknown) {
		why = WhyNotHeardLately // sorted before, quiet in the window: it keeps its class, and is not waiting for evidence
	}
	last := lastDayOf(r)
	return AirDevice{
		ID: shortID(r.Tag), Class: class, Why: string(why), Signal: signalBand(rules, f),
		FirstSeen: dayString(r.FirstDay), LastSeen: dayString(last), CyclesHeard: r.Cycles, NeedHeard: rules.MinCyclesHeard,
		RetryPercent: percent(r.Retries, r.Frames), Channels: channelsOf(r.Channels), Random: r.Random, JoinedOwn: r.JoinedOwn,
	}
}

func lastDayOf(r FeatureRow) int64 { return max(r.LastDay, r.FirstDay) }

func sortAir(out []AirDevice, by string) {
	rank := map[string]int{bandStrong: 3, bandMedium: 2, bandWeak: 1, bandNone: 0}
	slices.SortStableFunc(out, func(a, b AirDevice) int {
		switch by {
		case sortCycles:
			return b.CyclesHeard - a.CyclesHeard
		case sortSignal:
			return rank[b.Signal] - rank[a.Signal]
		case sortRetries:
			return b.RetryPercent - a.RetryPercent
		}
		return strings.Compare(b.LastSeen, a.LastSeen)
	})
}

// AirwaveChannel is how many devices were heard on a channel in the window.
type AirwaveChannel struct {
	Channel int `json:"channel"`
	Devices int `json:"devices"`
}

// AirwaveClass is what a class of sender did to the air: how many, how many frames, how many of them were retries.
type AirwaveClass struct {
	Class        string `json:"class"`
	Devices      int    `json:"devices"`
	Frames       int    `json:"frames"`
	RetryPercent int    `json:"retry_percent"`
}

// AirwaveRouter is a confirmed access point of the owner's and the channel it was last heard on.
type AirwaveRouter struct {
	Name    string `json:"name"`
	Channel int    `json:"channel"`
}

// AirwavesView is the answer of AirwavesPath: counts only.
type AirwavesView struct {
	Channels []AirwaveChannel `json:"channels"`
	Classes  []AirwaveClass   `json:"classes"`
	Routers  []AirwaveRouter  `json:"routers"`
	// NearUnknown is how many devices that are not yet sorted have a strong signal: unknown and close to the house.
	NearUnknown int `json:"near_unknown"`
	Listened    int `json:"listened_cycles"`
}

func (s *Stack) airwaves(ctx context.Context, store *Store) (stacksdk.Response, error) {
	rules, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	aps, err := store.OwnAPs(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	v := tallyAirwaves(rules, rows, listened)
	for _, a := range aps {
		if a.Confirmed {
			v.Routers = append(v.Routers, AirwaveRouter{Name: a.Name, Channel: a.Channel})
		}
	}
	return jsonResponse(http.StatusOK, v)
}

// tallyAirwaves counts the neighbours' devices by channel and by class.
func tallyAirwaves(rules Rules, rows []FeatureRow, listened map[int64]int) AirwavesView {
	v := AirwavesView{Channels: []AirwaveChannel{}, Classes: []AirwaveClass{}, Routers: []AirwaveRouter{}}
	for _, n := range listened {
		v.Listened += n
	}
	perChannel := map[int]int{}
	type tally struct{ devices, frames, retries int }
	perClass := map[string]*tally{}
	for _, r := range rows {
		if r.Owner {
			continue
		}
		d := airDevice(rules, r, listened)
		for _, ch := range d.Channels {
			perChannel[ch]++
		}
		if perClass[d.Class] == nil {
			perClass[d.Class] = &tally{}
		}
		t := perClass[d.Class]
		t.devices++
		t.frames += r.Frames
		t.retries += r.Retries
		if d.Class == string(ClassUnknown) && d.Signal == bandStrong {
			v.NearUnknown++
		}
	}
	for _, ch := range validChannels {
		if n := perChannel[ch]; n > 0 {
			v.Channels = append(v.Channels, AirwaveChannel{Channel: ch, Devices: n})
		}
	}
	for _, c := range Classes {
		if t := perClass[string(c)]; t != nil {
			v.Classes = append(v.Classes, AirwaveClass{Class: string(c), Devices: t.devices, Frames: t.frames, RetryPercent: percent(t.retries, t.frames)})
		}
	}
	return v
}

// The states of one hour of the strip.
const (
	hourHere         = "here"          // the device was heard
	hourAway         = "away"          // the sensor listened and did not hear it
	hourNotListening = "not_listening" // the sensor did not listen, so nothing is known
)

// PresenceHour is one hour of the strip.
type PresenceHour struct {
	Hour   string `json:"hour"` // RFC 3339, the start of the hour (UTC)
	State  string `json:"state"`
	Signal string `json:"signal,omitempty"`
}

// PresenceDevice is one of the owner's devices over the last presenceHours.
type PresenceDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// State is the current hour's state, or the last one the sensor listened in: here, away, or not_listening when it has not listened lately.
	State     string         `json:"state"`
	Signal    string         `json:"signal,omitempty"`
	LastHeard string         `json:"last_heard,omitempty"` // the start of the last hour it was heard
	JoinedOwn bool           `json:"joined_own"`
	Hours     []PresenceHour `json:"hours"`
}

// PresenceView is the answer of PresencePath.
type PresenceView struct {
	Devices []PresenceDevice `json:"devices"`
}

func (s *Stack) presence(ctx context.Context, store *Store) (stacksdk.Response, error) {
	rules, err := store.CurrentRules(ctx, s.now())
	if err != nil {
		return plainError(http.StatusInternalServerError, "the presence could not be read")
	}
	now := hourNumber(s.now())
	from := now - presenceHours + 1
	listening, err := store.airHours(ctx, from)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the presence could not be read")
	}
	owners, err := store.OwnerDevices(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the presence could not be read")
	}
	v := PresenceView{Devices: []PresenceDevice{}}
	for _, o := range owners {
		d, err := presenceOf(ctx, store, rules, o, listening, from, now)
		if err != nil {
			return plainError(http.StatusInternalServerError, "the presence could not be read")
		}
		v.Devices = append(v.Devices, d)
	}
	return jsonResponse(http.StatusOK, v)
}

// presenceOf lays one owner device's hours from `from` to `now` over what the sensor listened to.
func presenceOf(ctx context.Context, store *Store, rules Rules, o Device, listening map[int64]int, from, now int64) (PresenceDevice, error) {
	heard, last, err := store.ownerHours(ctx, o.Tag, from)
	if err != nil {
		return PresenceDevice{}, err
	}
	joined, err := store.JoinedOwn(ctx, o.Tag)
	if err != nil {
		return PresenceDevice{}, err
	}
	d := PresenceDevice{ID: hex.EncodeToString(o.Tag), Label: o.Label, JoinedOwn: joined, Hours: make([]PresenceHour, 0, presenceHours)}
	for h := from; h <= now; h++ {
		c := PresenceHour{Hour: time.Unix(h*3600, 0).UTC().Format(time.RFC3339), State: hourNotListening}
		switch {
		case heard[h].Cycles > 0:
			c.State, c.Signal = hourHere, signalBand(rules, Features{HasSignal: heard[h].SigN > 0, SignalMean: heard[h].Mean()})
		case listening[h] > 0:
			c.State = hourAway
		}
		d.Hours = append(d.Hours, c)
	}
	cur := d.Hours[len(d.Hours)-1]
	if cur.State == hourNotListening && len(d.Hours) > 1 { // the hour just begun may have no cycle yet: judge by the one before
		cur = d.Hours[len(d.Hours)-2]
	}
	d.State, d.Signal = cur.State, cur.Signal
	if last > 0 {
		d.LastHeard = time.Unix(last*3600, 0).UTC().Format(time.RFC3339)
	}
	return d, nil
}

// HourRow is what one hour of an owner's device adds up to.
type HourRow struct {
	Cycles, SigN int
	SigSum       int64
}

// Mean is the mean signal of the hour, or 0 when none was measured.
func (h HourRow) Mean() float64 {
	if h.SigN == 0 {
		return 0
	}
	return float64(h.SigSum) / float64(h.SigN)
}

func (s *Store) airHours(ctx context.Context, from int64) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hour, cycles FROM air_hours WHERE hour >= ?`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var h int64
		var n int
		if err := rows.Scan(&h, &n); err != nil {
			return nil, err
		}
		out[h] = n
	}
	return out, rows.Err()
}

// ownerHours reads one owner device's hours from `from` on, and the last hour it was ever heard (0 when never).
func (s *Store) ownerHours(ctx context.Context, tag []byte, from int64) (map[int64]HourRow, int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hour, cycles, sig_n, sig_sum FROM owner_hours WHERE tag = ? AND hour >= ?`, tag, from)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[int64]HourRow{}
	for rows.Next() {
		var h int64
		var r HourRow
		if err := rows.Scan(&h, &r.Cycles, &r.SigN, &r.SigSum); err != nil {
			return nil, 0, err
		}
		out[h] = r
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var last int64
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(max(hour), 0) FROM owner_hours WHERE tag = ?`, tag).Scan(&last)
	return out, last, err
}

// JoinedOwn says whether a device has asked to join one of the owner's confirmed networks.
func (s *Store) JoinedOwn(ctx context.Context, tag []byte) (bool, error) {
	var j bool
	err := s.db.QueryRowContext(ctx, `SELECT joined_own FROM devices WHERE tag = ?`, tag).Scan(&j)
	return j, err
}
