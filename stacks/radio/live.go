package radio

import (
	"context"
	"math"
	"net/http"
	"sort"
	"time"

	"llm-proxy-stacks/sdk"
)

// The live picture: what the sensor is hearing now, drawn as a radar. It is kept IN MEMORY ONLY, bounded, and forgotten when the run ends
// (ADR-0053: nothing finer than a day is written for other people's devices, and no frame is ever stored). A device's recent signal is its last
// few cycles' mean, so a dot moves cycle by cycle and, while a cycle is open, window by window.
const (
	// LivePath answers the radar (GET); CalibratePath stands one of the owner's devices one metre from the sensor (POST).
	LivePath      = "/admin/api/radio/air/live"
	CalibratePath = "/admin/api/radio/air/calibrate/{id}"
	// TrackPath answers one device's signal over the run (GET), from memory.
	TrackPath = "/admin/api/radio/air/track/{id}"

	liveTrailCycles = 8   // cycles of signal a device keeps
	liveKeepCycles  = 6   // cycles a device may be silent before it leaves the picture
	liveMaxDevices  = 500 // the most devices the picture holds
	// liveTrackCycles is how many cycles a device's track keeps (memory only; with the shortest cycle, 60 s, six hours). A device silent for that
	// long loses its track, so memory is bounded by devices x cycles and nothing outlives a run.
	liveTrackCycles = 360
	distanceDoc     = "radio_distance"
)

// liveDev is one device in the picture.
type liveDev struct {
	trail   []float64
	channel int
	missed  int
}

// trackPt is one cycle a device was heard in: which cycle (counted from the start of the run) and its mean signal.
type trackPt struct {
	cycle   int
	at      int64 // unix seconds of the cycle
	signal  float64
	channel int
}

// liveSample is a device as the picture holds it at one moment.
type liveSample struct {
	tag     []byte
	signal  float64
	trail   []float64 // the finished cycles, then the open one
	channel int
	heard   bool
	// since and last are the unix times of the first and the last finished cycle of this run that heard the device, 0 when none did yet.
	since, last int64
	// moving is the focus windows' word that its signal just moved (movingOf); picture only.
	moving bool
}

func firstChannel(mask uint64) int {
	if ch := channelsOf(mask); len(ch) > 0 {
		return ch[0]
	}
	return 0
}

// absorbCycle folds a finished cycle into the picture. cmu is held.
func (r *Recorder) absorbCycle(heard map[string]*agg, at time.Time) {
	if r.live == nil {
		r.live = map[string]*liveDev{}
	}
	for tag, d := range r.live {
		if a := heard[tag]; a != nil && a.n > 0 {
			continue
		}
		if d.missed++; d.missed > liveKeepCycles {
			delete(r.live, tag)
		}
	}
	r.liveCycle++
	r.lastCycle = at.Unix()
	r.absorbTracks(heard, at)
	for tag, a := range heard {
		if a.n == 0 {
			continue
		}
		d := r.live[tag]
		if d == nil {
			if len(r.live) >= liveMaxDevices {
				continue
			}
			d = &liveDev{}
			r.live[tag] = d
		}
		d.missed, d.channel = 0, firstChannel(a.channels)
		mean := float64(a.sum) / float64(a.n)
		a.crossed = jumped(d.trail, mean) // read by FlushCycle, which runs after this
		d.trail = append(d.trail, mean)
		if len(d.trail) > liveTrailCycles {
			d.trail = d.trail[len(d.trail)-liveTrailCycles:]
		}
	}
}

// jumped says whether a cycle's mean signal moved by crossJumpDB or more from the cycle before: movement, not an access point's wobble.
func jumped(trail []float64, mean float64) bool {
	return len(trail) > 0 && math.Abs(trail[len(trail)-1]-mean) >= crossJumpDB
}

// absorbTracks adds the finished cycle to each heard device's track and forgets the tracks that have gone a whole track length without a point.
// cmu is held; r.liveCycle already counts this cycle.
func (r *Recorder) absorbTracks(heard map[string]*agg, at time.Time) {
	if r.tracks == nil {
		r.tracks = map[string][]trackPt{}
	}
	for tag, pts := range r.tracks {
		if r.liveCycle-pts[len(pts)-1].cycle >= liveTrackCycles {
			delete(r.tracks, tag)
		}
	}
	for tag, a := range heard {
		if a.n == 0 {
			continue
		}
		pts, ok := r.tracks[tag]
		if !ok && len(r.tracks) >= liveMaxDevices {
			continue
		}
		pts = append(pts, trackPt{cycle: r.liveCycle, at: at.Unix(), signal: float64(a.sum) / float64(a.n), channel: firstChannel(a.channels)})
		if len(pts) > liveTrackCycles {
			pts = pts[len(pts)-liveTrackCycles:]
		}
		r.tracks[tag] = pts
	}
}

// LastCycleAt is the unix time the last cycle of this run finished, 0 before the first.
func (r *Recorder) LastCycleAt() int64 {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	return r.lastCycle
}

// ClearLive forgets the picture and every track: a run that ends leaves nothing behind.
func (r *Recorder) ClearLive() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.live, r.tracks, r.netNames, r.knownAPs, r.focus, r.liveCycle, r.lastCycle = nil, nil, nil, nil, nil, 0, 0
	r.meshIdx, r.meshTrail, r.meshAsked, r.meshListAt = nil, nil, time.Time{}, 0
}

// TrackSnapshot is the device whose short id this is: its points, the cycle count now and whether any is a point at all. It copies; the caller
// holds nothing.
func (r *Recorder) TrackSnapshot(id string) (tag []byte, pts []trackPt, now int, ok bool) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	for t, p := range r.tracks {
		if shortID([]byte(t)) == id {
			return []byte(t), append([]trackPt(nil), p...), r.liveCycle, true
		}
	}
	return nil, nil, r.liveCycle, false
}

// LiveSnapshot is every device in the picture, newest reading last: the finished cycles, then the focus windows since the last one, then the
// open cycle's reading.
func (r *Recorder) LiveSnapshot() []liveSample {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	seen := map[string]bool{}
	var out []liveSample
	for tag, d := range r.live {
		seen[tag] = true
		s := liveSample{tag: []byte(tag), trail: append([]float64(nil), d.trail...), channel: d.channel, heard: d.missed == 0}
		if pts := r.tracks[tag]; len(pts) > 0 {
			s.since, s.last = pts[0].at, pts[len(pts)-1].at
		}
		r.withFocus(&s, tag, d.trail)
		r.withOpenCycle(&s, tag)
		s.signal = s.trail[len(s.trail)-1]
		out = append(out, s)
	}
	for tag := range r.cycle {
		seen = r.addUnseen(&out, seen, tag)
	}
	for tag := range r.focus {
		seen = r.addUnseen(&out, seen, tag)
	}
	return out
}

// withFocus adds the focus readings since the last finished cycle to a sample. cmu is held.
func (r *Recorder) withFocus(s *liveSample, tag string, cycles []float64) {
	recent := r.focusSince(tag, r.lastCycle)
	if len(recent) == 0 {
		return
	}
	for _, p := range recent {
		s.trail = append(s.trail, p.signal)
	}
	s.channel, s.heard = recent[len(recent)-1].channel, true
	last := 0.0
	if len(cycles) > 0 {
		last = cycles[len(cycles)-1]
	}
	s.moving = movingOf(last, len(cycles) > 0, recent)
}

// withOpenCycle adds the open cycle's reading to a sample. cmu is held.
func (r *Recorder) withOpenCycle(s *liveSample, tag string) {
	if a := r.cycle[tag]; a != nil && a.n > 0 {
		s.trail = append(s.trail, float64(a.sum)/float64(a.n))
		s.channel, s.heard = firstChannel(a.channels), true
	}
}

// addUnseen adds a device heard only in the open cycle or a focus window since the last cycle, not yet in the picture. cmu is held.
func (r *Recorder) addUnseen(out *[]liveSample, seen map[string]bool, tag string) map[string]bool {
	if seen[tag] || len(*out) >= liveMaxDevices {
		return seen
	}
	s := liveSample{tag: []byte(tag)}
	r.withFocus(&s, tag, nil)
	r.withOpenCycle(&s, tag)
	if len(s.trail) == 0 {
		return seen
	}
	seen[tag] = true
	s.signal = s.trail[len(s.trail)-1]
	*out = append(*out, s)
	return seen
}

// LiveDevice is one dot of the radar. A neighbour is a short id and a class; only the owner's devices carry a label.
type LiveDevice struct {
	ID    string `json:"id"`
	Mine  bool   `json:"mine"`
	Label string `json:"label,omitempty"`
	Class string `json:"class"`
	Dbm   int    `json:"dbm"`
	// NearM and FarM are the range the distance can be in, in metres; Ring is where its middle falls. Angle is only a stable place on the ring.
	NearM   float64 `json:"near_m"`
	FarM    float64 `json:"far_m"`
	Ring    string  `json:"ring"`
	Angle   float64 `json:"angle"`
	Trend   string  `json:"trend"`
	Channel int     `json:"channel"`
	Heard   bool    `json:"heard"`
	// Moving says the signal moved by crossJumpDB or more within a few minutes, from the focus windows (focus.go) or the router's readings
	// (mesh.go): the picture's word only.
	Moving bool `json:"moving,omitempty"`
	// Router is what the mesh router's latest list says of the device (mesh.go), nil when it is not in it: never stored, never evidence that it is yours.
	Router *RouterSeen `json:"router,omitempty"`
	// Usual is a device drawn from its stored days only: it is not being heard now, and Dbm is its mean over the days kept.
	Usual bool `json:"usual,omitempty"`
	// UsualDbm is the device's own mean over the days kept, when there are enough samples to call it a baseline; ShiftDB is how far the signal now is
	// from it, and Unusual says the shift is large against the device's own spread.
	UsualDbm int  `json:"usual_dbm,omitempty"`
	ShiftDB  int  `json:"shift_db,omitempty"`
	Unusual  bool `json:"unusual,omitempty"`
	// TailM is where the device's middle distance was in the last few cycles, in metres and oldest first, so a dot can show where it came from. Detail is the evidence behind the kind it was
	// sorted into (never for the owner's devices, which the owner named).
	TailM  []float64  `json:"tail_m,omitempty"`
	Detail *AirDevice `json:"detail,omitempty"`
	// Network is the name an access point broadcasts, held in memory for this run only and never written to the disk (ADR-0053).
	Network string `json:"network,omitempty"`
	// HeardSince and LastAt are the unix times of the first and the latest finished cycle of this run that heard the device (memory only, like the picture).
	HeardSince int64 `json:"heard_since,omitempty"`
	LastAt     int64 `json:"last_at,omitempty"`
	// Ident is what names the device and why (ident.go): its kind, maker and model, the network it uses, its box, its name on your network.
	Ident *DeviceIdent `json:"ident,omitempty"`
}

// LiveView is the answer of LivePath.
type LiveView struct {
	Listening bool           `json:"listening"`
	Cycles    int            `json:"cycles"`
	Model     DistanceModel  `json:"model"`
	Devices   []LiveDevice   `json:"devices"`
	Rings     map[string]int `json:"rings"`
	// CycleSeconds and LastCycleAt (unix, 0 before the first cycle of this run) let the page count down to the next reading. Listened and
	// MinListened are the cycles the sensor has listened through and the number the sorting needs.
	CycleSeconds int   `json:"cycle_seconds"`
	LastCycleAt  int64 `json:"last_cycle_at"`
	Listened     int   `json:"listened_cycles"`
	MinListened  int   `json:"min_listened_cycles"`
}

const (
	// usualMinSamples is how many stored signal readings make a baseline; unusualMinDB and unusualSpreads are how far from it a signal must be.
	usualMinSamples = 10
	unusualMinDB    = 8.0
	unusualSpreads  = 2.0
	maxUsualDots    = 300
)

// tailOf is a trail of signals as the middle of the distance range each stood for.
func tailOf(model DistanceModel, trail []float64, channel int) []float64 {
	out := make([]float64, len(trail))
	for i, v := range trail {
		near, far := model.Range(v, channel)
		out[i] = round1(math.Sqrt(near * far))
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// distanceModel is the model in force: the owner's calibration when there is one.
func (s *Stack) distanceModel(ctx context.Context, store *Store) DistanceModel {
	m := DefaultDistance()
	var kept DistanceModel
	if ok, err := store.GetDoc(ctx, distanceDoc, &kept); err == nil && ok {
		m = kept
	}
	return m
}

// baseline is what a device's stored days say its signal usually is.
type baseline struct {
	mean, std float64
	row       FeatureRow
	has       bool // the stored days hold a real signal (FeaturesOf)
	ok        bool // enough samples to call it a baseline
}

func baselineOf(r FeatureRow, listened map[int64]int) baseline {
	f := FeaturesOf(r, listened)
	return baseline{mean: f.SignalMean, std: f.SignalStd, row: r, has: f.HasSignal, ok: f.HasSignal && r.SigN >= usualMinSamples}
}

// dotOf places a signal on the model: the range, the ring and a stable angle.
func dotOf(model DistanceModel, tag []byte, signal float64, channel int) LiveDevice {
	near, far := model.Range(signal, channel)
	return LiveDevice{ID: shortID(tag), Dbm: int(math.Round(signal)), NearM: round1(near), FarM: round1(far), Ring: RingOf(near, far), Angle: angleOf(tag), Channel: channel}
}

func classWord(c string) string {
	if c == "" {
		return string(ClassUnknown)
	}
	return c
}

func (s *Stack) live(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store) (stacksdk.Response, error) {
	v, _, err := s.liveView(ctx, h, rec, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	return jsonResponse(http.StatusOK, v)
}

// liveSet is the live picture with what was read to build it, so the device page and the networks view reuse one reading.
type liveSet struct {
	rules    Rules
	rows     []FeatureRow
	byTag    map[string]FeatureRow
	byShort  map[string]FeatureRow // short id to row, for the short ids only one device has
	listened map[int64]int
	ident    identCtx
	trails   map[string][]float64
}

// liveView builds the radar's answer.
func (s *Stack) liveView(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store) (LiveView, liveSet, error) {
	model := s.distanceModel(ctx, store)
	st := s.listenState(ctx, s.h)
	v := LiveView{Listening: st.Listening, Cycles: st.Cycles, Model: model, Devices: []LiveDevice{}, Rings: map[string]int{}, CycleSeconds: s.settings(ctx, h).Plan().CycleSeconds, LastCycleAt: rec.LastCycleAt()}
	rules, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return v, liveSet{}, err
	}
	set := newLiveSet(rules, rows, listened, rec.LANIndex(ctx))
	v.MinListened = rules.MinCyclesListened
	for _, n := range listened {
		v.Listened += n
	}
	detail := map[string]AirDevice{}
	base := map[string]baseline{}
	for _, r := range rows {
		base[string(r.Tag)] = baselineOf(r, listened)
		if !r.Owner {
			detail[string(r.Tag)] = airDevice(rules, r, listened)
		}
	}
	now := map[string]bool{}
	for _, sm := range rec.LiveSnapshot() {
		dev, ok, err := store.Device(ctx, sm.tag)
		if err != nil {
			return v, set, err
		}
		if !ok {
			continue
		}
		now[string(sm.tag)] = true
		d := heardDot(model, rec, sm, dev, base[string(sm.tag)])
		if ad, ok := detail[string(sm.tag)]; ok && !dev.Owner {
			d.Detail = &ad
		}
		set.trails[string(sm.tag)] = sm.trail
		d.Ident = set.identOf(sm.tag, sm.trail)
		fixedToSensor(&d)
		v.Rings[d.Ring]++
		v.Devices = append(v.Devices, d)
	}
	sort.Slice(v.Devices, func(i, j int) bool { return v.Devices[i].Dbm > v.Devices[j].Dbm })
	for _, u := range usualDots(model, base, detail, now) {
		u.Ident = set.identOf(u.tag, nil)
		if u.Router = rec.RouterOf(u.tag); u.Router != nil {
			u.Moving = u.Router.Moving
		}
		fixedToSensor(&u.LiveDevice)
		v.Devices = append(v.Devices, u.LiveDevice)
	}
	return v, set, nil
}

// heardDot is a device heard in this run, placed on the model.
func heardDot(model DistanceModel, rec *Recorder, sm liveSample, dev Device, b baseline) LiveDevice {
	d := dotOf(model, sm.tag, sm.signal, sm.channel)
	d.Mine, d.Class, d.Trend, d.Heard = dev.Owner, classWord(dev.Class), TrendOf(sm.trail), sm.heard
	if dev.Owner {
		d.Label = dev.Label
	}
	if b.ok {
		compareToUsual(&d, sm.signal, b)
	}
	d.TailM = tailOf(model, sm.trail, sm.channel)
	d.Network = rec.NetworkName(sm.tag)
	d.HeardSince, d.LastAt = sm.since, sm.last
	d.Router = rec.RouterOf(sm.tag)
	d.Moving = sm.moving || (d.Router != nil && d.Router.Moving)
	return d
}

// fixedToSensor clears the movement words of this computer's own card: it is fixed to the sensor, so a change in its signal is the radio's
// wobble, never a move.
func fixedToSensor(d *LiveDevice) {
	if d.Ident != nil && d.Ident.ThisComputer != "" {
		d.Moving, d.Trend, d.Unusual, d.ShiftDB = false, TrendUnknown, false, 0
	}
}

// newLiveSet indexes the rows by tag and by short id (a short id two devices share is left out of the second).
func newLiveSet(rules Rules, rows []FeatureRow, listened map[int64]int, lan map[string]LANInfo) liveSet {
	markOnLAN(rows, lan) // exactly as ClassifyAllWith does, so the page explains with the evidence the classifier used
	set := liveSet{rules: rules, rows: rows, byTag: make(map[string]FeatureRow, len(rows)), byShort: map[string]FeatureRow{}, listened: listened, ident: newIdentCtx(rows, lan), trails: map[string][]float64{}}
	twice := map[string]bool{}
	for _, r := range rows {
		set.byTag[string(r.Tag)] = r
		if _, has := set.byShort[shortID(r.Tag)]; has {
			twice[shortID(r.Tag)] = true
		}
		set.byShort[shortID(r.Tag)] = r
	}
	for id := range twice {
		delete(set.byShort, id)
	}
	return set
}

// identOf is a stored device's identity, nil when it has no row in the window.
func (set liveSet) identOf(tag []byte, trail []float64) *DeviceIdent {
	r, ok := set.byTag[string(tag)]
	if !ok {
		return nil
	}
	id := set.ident.of(r, trail)
	return &id
}

// compareToUsual sets how far a device's signal now is from its own baseline, and whether that is unusual for it.
func compareToUsual(d *LiveDevice, signal float64, b baseline) {
	d.UsualDbm = int(math.Round(b.mean))
	shift := signal - b.mean
	d.ShiftDB = int(math.Round(shift))
	d.Unusual = math.Abs(shift) >= math.Max(unusualMinDB, unusualSpreads*b.std)
}

// usualDots draws every device the stored days know that is not being heard now, at its mean signal. Stored data is day-level (ADR-0053), so
// these dots say where a device usually is, never where it was at some moment.
// usualDot is a dot drawn from the stored days, with the tag it was made from (never sent).
type usualDot struct {
	LiveDevice
	tag []byte
}

func usualDots(model DistanceModel, base map[string]baseline, detail map[string]AirDevice, now map[string]bool) []usualDot {
	var out []usualDot
	for tag, b := range base {
		if now[tag] || !b.has || b.row.Cycles == 0 {
			continue
		}
		d := dotOf(model, b.row.Tag, b.mean, firstChannel(b.row.Channels))
		d.Mine, d.Class, d.Trend, d.Usual = b.row.Owner, classWord(b.row.Class), TrendUnknown, true
		if b.row.Owner {
			d.Label = b.row.Label
		}
		if ad, ok := detail[tag]; ok {
			d.Detail = &ad
		}
		out = append(out, usualDot{LiveDevice: d, tag: b.row.Tag})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Dbm > out[j].Dbm || (out[i].Dbm == out[j].Dbm && out[i].ID < out[j].ID)
	})
	return out[:min(len(out), maxUsualDots)]
}

// calibrate takes the owner's device as standing a known distance from the sensor (one metre unless the body says {"metres": n}): its signal now
// gives the signal at one metre for its band. One of the owner's own access points can be used the same way, at the distance the owner measured,
// so a fixed mesh node calibrates the picture without anyone holding a phone next to the computer.
func (s *Stack) calibrate(ctx context.Context, rec *Recorder, store *Store, id string, body []byte) (stacksdk.Response, error) {
	at, bad := calibrationMetres(body)
	if bad != "" {
		return plainError(http.StatusBadRequest, bad)
	}
	for _, sm := range rec.LiveSnapshot() {
		if shortID(sm.tag) != id {
			continue
		}
		if mine, err := store.CanCalibrate(ctx, sm.tag); err != nil || !mine {
			break
		}
		cur := s.distanceModel(ctx, store)
		model := cur.WithRef(sm.channel, math.Round(refAt(cur, sm.signal, at)))
		if err := store.PutDoc(ctx, distanceDoc, model, s.now()); err != nil {
			return plainError(http.StatusInternalServerError, "the calibration could not be kept")
		}
		return jsonResponse(http.StatusOK, model)
	}
	return plainError(http.StatusNotFound, "only a device or access point of yours that is being heard can be used to calibrate")
}

const maxCalibrationMetres = 30

// calibrationMetres reads the optional distance; no body is one metre.
func calibrationMetres(body []byte) (float64, string) {
	if len(body) == 0 {
		return 1, ""
	}
	var b struct {
		Metres float64 `json:"metres"`
	}
	if err := strictDecode(body, &b); err != nil || (b.Metres != 0 && (b.Metres < 0.5 || b.Metres > maxCalibrationMetres)) {
		return 0, "give the distance in metres, from 0.5 to 30"
	}
	if b.Metres == 0 {
		return 1, "" // {} is the one-metre calibration the page has always sent
	}
	return b.Metres, ""
}

// refAt is the signal at one metre that makes a reading at d metres fit the model, using the middle of its two exponents.
func refAt(m DistanceModel, signal, d float64) float64 {
	return signal + 10*((m.NearExp+m.FarExp)/2)*math.Log10(d)
}

// TrackPoint is one cycle of a track. At is the unix time of the cycle; Run counts unbroken stretches, so a device that was silent for several
// cycles starts a new run and the chart leaves a gap there instead of drawing a line through nothing.
type TrackPoint struct {
	At    int64   `json:"at"`
	Dbm   int     `json:"dbm"`
	NearM float64 `json:"near_m"`
	FarM  float64 `json:"far_m"`
	Run   int     `json:"run"`
}

// TrackView is the answer of TrackPath: one device's signal, oldest first. Stored says the points came from the disk (the owner turned the
// track on, ADR-0055); otherwise they are this run's memory and are gone when listening stops. Now is the unix time of the answer.
type TrackView struct {
	ID     string       `json:"id"`
	Mine   bool         `json:"mine"`
	Label  string       `json:"label,omitempty"`
	Trend  string       `json:"trend"`
	Stored bool         `json:"stored"`
	Now    int64        `json:"now"`
	Points []TrackPoint `json:"points"`
}

// gapCycles is how many cycles of silence end a run of a track.
const gapCycles = 2.5

// track answers one device's track: from the disk when the owner keeps tracks, else from this run's memory. A device with no points is not found.
func (s *Stack) track(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	set := s.settings(ctx, h)
	tag, pts, stored, err := s.trackSource(ctx, rec, store, id, set.TrackHours)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	if len(pts) == 0 {
		return plainError(http.StatusNotFound, "this device has no track yet")
	}
	dev, found, err := store.Device(ctx, tag)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	if !found {
		return plainError(http.StatusNotFound, "this device has no track yet")
	}
	model := s.distanceModel(ctx, store)
	gap := int64(float64(set.Plan().CycleSeconds) * gapCycles)
	v := TrackView{ID: id, Mine: dev.Owner, Stored: stored, Now: s.now().Unix(), Points: make([]TrackPoint, 0, len(pts))}
	if dev.Owner {
		v.Label = dev.Label
	}
	run := 0
	sig := make([]float64, 0, len(pts))
	for i, p := range pts {
		if i > 0 && p.at-pts[i-1].at > gap {
			run++
		}
		near, far := model.Range(p.signal, p.channel)
		v.Points = append(v.Points, TrackPoint{At: p.at, Dbm: int(math.Round(p.signal)), NearM: round1(near), FarM: round1(far), Run: run})
		sig = append(sig, p.signal)
	}
	v.Trend = TrendOf(sig[max(0, len(sig)-liveTrailCycles):])
	return jsonResponse(http.StatusOK, v)
}

// trackSource is where a device's points come from: the disk when tracks are kept, else this run's memory.
func (s *Stack) trackSource(ctx context.Context, rec *Recorder, store *Store, id string, hours int) ([]byte, []trackPt, bool, error) {
	if hours <= 0 {
		tag, pts, _, ok := rec.TrackSnapshot(id)
		if !ok {
			return nil, nil, false, nil
		}
		return tag, pts, false, nil
	}
	tag, err := store.TagByShortID(ctx, id)
	if err != nil || tag == nil {
		return nil, nil, true, err
	}
	pts, err := store.TrackPoints(ctx, tag, s.now().Add(-time.Duration(hours)*time.Hour))
	return tag, pts, true, err
}
