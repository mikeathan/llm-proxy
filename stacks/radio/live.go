package radio

import (
	"context"
	"math"
	"net/http"
	"sort"

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
}

func firstChannel(mask uint64) int {
	if ch := channelsOf(mask); len(ch) > 0 {
		return ch[0]
	}
	return 0
}

// absorbCycle folds a finished cycle into the picture. cmu is held.
func (r *Recorder) absorbCycle(heard map[string]*agg) {
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
	r.absorbTracks(heard)
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
		d.trail = append(d.trail, float64(a.sum)/float64(a.n))
		if len(d.trail) > liveTrailCycles {
			d.trail = d.trail[len(d.trail)-liveTrailCycles:]
		}
	}
}

// absorbTracks adds the finished cycle to each heard device's track and forgets the tracks that have gone a whole track length without a point.
// cmu is held; r.liveCycle already counts this cycle.
func (r *Recorder) absorbTracks(heard map[string]*agg) {
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
		pts = append(pts, trackPt{cycle: r.liveCycle, signal: float64(a.sum) / float64(a.n), channel: firstChannel(a.channels)})
		if len(pts) > liveTrackCycles {
			pts = pts[len(pts)-liveTrackCycles:]
		}
		r.tracks[tag] = pts
	}
}

// ClearLive forgets the picture and every track: a run that ends leaves nothing behind.
func (r *Recorder) ClearLive() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.live, r.tracks, r.liveCycle = nil, nil, 0
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

// LiveSnapshot is every device in the picture, the open cycle's reading first.
func (r *Recorder) LiveSnapshot() []liveSample {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	seen := map[string]bool{}
	var out []liveSample
	for tag, d := range r.live {
		seen[tag] = true
		s := liveSample{tag: []byte(tag), trail: append([]float64(nil), d.trail...), channel: d.channel, heard: d.missed == 0}
		if a := r.cycle[tag]; a != nil && a.n > 0 {
			s.trail = append(s.trail, float64(a.sum)/float64(a.n))
			s.channel, s.heard = firstChannel(a.channels), true
		}
		s.signal = s.trail[len(s.trail)-1]
		out = append(out, s)
	}
	for tag, a := range r.cycle {
		if seen[tag] || a.n == 0 || len(out) >= liveMaxDevices {
			continue
		}
		m := float64(a.sum) / float64(a.n)
		out = append(out, liveSample{tag: []byte(tag), signal: m, trail: []float64{m}, channel: firstChannel(a.channels), heard: true})
	}
	return out
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
	// Usual is a device drawn from its stored days only: it is not being heard now, and Dbm is its mean over the days kept.
	Usual bool `json:"usual,omitempty"`
	// UsualDbm is the device's own mean over the days kept, when there are enough samples to call it a baseline; ShiftDB is how far the signal now is
	// from it, and Unusual says the shift is large against the device's own spread.
	UsualDbm int  `json:"usual_dbm,omitempty"`
	ShiftDB  int  `json:"shift_db,omitempty"`
	Unusual  bool `json:"unusual,omitempty"`
}

// LiveView is the answer of LivePath.
type LiveView struct {
	Listening bool           `json:"listening"`
	Cycles    int            `json:"cycles"`
	Model     DistanceModel  `json:"model"`
	Devices   []LiveDevice   `json:"devices"`
	Rings     map[string]int `json:"rings"`
}

const (
	// usualMinSamples is how many stored signal readings make a baseline; unusualMinDB and unusualSpreads are how far from it a signal must be.
	usualMinSamples = 10
	unusualMinDB    = 8.0
	unusualSpreads  = 2.0
	maxUsualDots    = 300
)

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
	ok        bool // enough samples to call it a baseline
}

func baselineOf(r FeatureRow, listened map[int64]int) baseline {
	f := FeaturesOf(r, listened)
	return baseline{mean: f.SignalMean, std: f.SignalStd, row: r, ok: f.HasSignal && r.SigN >= usualMinSamples}
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

func (s *Stack) live(ctx context.Context, rec *Recorder, store *Store) (stacksdk.Response, error) {
	model := s.distanceModel(ctx, store)
	st := s.listenState(ctx, s.h)
	v := LiveView{Listening: st.Listening, Cycles: st.Cycles, Model: model, Devices: []LiveDevice{}, Rings: map[string]int{}}
	_, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	base := map[string]baseline{}
	for _, r := range rows {
		base[string(r.Tag)] = baselineOf(r, listened)
	}
	now := map[string]bool{}
	for _, sm := range rec.LiveSnapshot() {
		dev, ok, err := store.Device(ctx, sm.tag)
		if err != nil {
			return plainError(http.StatusInternalServerError, "the air could not be read")
		}
		if !ok {
			continue
		}
		now[string(sm.tag)] = true
		d := dotOf(model, sm.tag, sm.signal, sm.channel)
		d.Mine, d.Class, d.Trend, d.Heard = dev.Owner, classWord(dev.Class), TrendOf(sm.trail), sm.heard
		if dev.Owner {
			d.Label = dev.Label
		}
		if b := base[string(sm.tag)]; b.ok {
			compareToUsual(&d, sm.signal, b)
		}
		v.Rings[d.Ring]++
		v.Devices = append(v.Devices, d)
	}
	sort.Slice(v.Devices, func(i, j int) bool { return v.Devices[i].Dbm > v.Devices[j].Dbm })
	v.Devices = append(v.Devices, usualDots(model, base, now)...)
	return jsonResponse(http.StatusOK, v)
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
func usualDots(model DistanceModel, base map[string]baseline, now map[string]bool) []LiveDevice {
	var out []LiveDevice
	for tag, b := range base {
		if now[tag] || b.row.SigN == 0 || b.row.Cycles == 0 {
			continue
		}
		d := dotOf(model, b.row.Tag, b.mean, firstChannel(b.row.Channels))
		d.Mine, d.Class, d.Trend, d.Usual = b.row.Owner, classWord(b.row.Class), TrendUnknown, true
		if b.row.Owner {
			d.Label = b.row.Label
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Dbm > out[j].Dbm || (out[i].Dbm == out[j].Dbm && out[i].ID < out[j].ID)
	})
	return out[:min(len(out), maxUsualDots)]
}

// calibrate takes the owner's device as standing one metre from the sensor: its signal now is the signal at one metre for its band.
func (s *Stack) calibrate(ctx context.Context, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	for _, sm := range rec.LiveSnapshot() {
		if shortID(sm.tag) != id {
			continue
		}
		if dev, ok, err := store.Device(ctx, sm.tag); err != nil || !ok || !dev.Owner {
			break
		}
		model := s.distanceModel(ctx, store).WithRef(sm.channel, math.Round(sm.signal))
		if err := store.PutDoc(ctx, distanceDoc, model, s.now()); err != nil {
			return plainError(http.StatusInternalServerError, "the calibration could not be kept")
		}
		return jsonResponse(http.StatusOK, model)
	}
	return plainError(http.StatusNotFound, "only a device of yours that is being heard can be used to calibrate")
}

// TrackPoint is one cycle of a track. Ago counts cycles back from now (0 is the latest); a cycle the device was silent in has no point, so the
// gaps show in Ago.
type TrackPoint struct {
	Ago   int     `json:"ago"`
	Dbm   int     `json:"dbm"`
	NearM float64 `json:"near_m"`
	FarM  float64 `json:"far_m"`
}

// TrackView is the answer of TrackPath: one device's signal over this run, oldest first. It is memory only (ADR-0053).
type TrackView struct {
	ID     string       `json:"id"`
	Mine   bool         `json:"mine"`
	Label  string       `json:"label,omitempty"`
	Trend  string       `json:"trend"`
	Cycles int          `json:"cycles"`
	Points []TrackPoint `json:"points"`
}

// track answers one device's track. A device that is not in the run's memory (the run ended, or it was silent for a whole track) is not found.
func (s *Stack) track(ctx context.Context, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	tag, pts, now, ok := rec.TrackSnapshot(id)
	if !ok {
		return plainError(http.StatusNotFound, "this device has no track in this run")
	}
	dev, found, err := store.Device(ctx, tag)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	if !found {
		return plainError(http.StatusNotFound, "this device has no track in this run")
	}
	model := s.distanceModel(ctx, store)
	v := TrackView{ID: id, Mine: dev.Owner, Cycles: now, Points: make([]TrackPoint, 0, len(pts))}
	if dev.Owner {
		v.Label = dev.Label
	}
	sig := make([]float64, 0, len(pts))
	for _, p := range pts {
		near, far := model.Range(p.signal, p.channel)
		v.Points = append(v.Points, TrackPoint{Ago: now - p.cycle, Dbm: int(math.Round(p.signal)), NearM: round1(near), FarM: round1(far)})
		sig = append(sig, p.signal)
	}
	v.Trend = TrendOf(sig[max(0, len(sig)-liveTrailCycles):])
	return jsonResponse(http.StatusOK, v)
}
