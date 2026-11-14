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

	liveTrailCycles = 8   // cycles of signal a device keeps
	liveKeepCycles  = 6   // cycles a device may be silent before it leaves the picture
	liveMaxDevices  = 500 // the most devices the picture holds
	distanceDoc     = "radio_distance"
)

// liveDev is one device in the picture.
type liveDev struct {
	trail   []float64
	channel int
	missed  int
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

// ClearLive forgets the picture: a run that ends leaves nothing behind.
func (r *Recorder) ClearLive() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.live = nil
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
}

// LiveView is the answer of LivePath.
type LiveView struct {
	Listening bool           `json:"listening"`
	Cycles    int            `json:"cycles"`
	Model     DistanceModel  `json:"model"`
	Devices   []LiveDevice   `json:"devices"`
	Rings     map[string]int `json:"rings"`
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

func (s *Stack) live(ctx context.Context, rec *Recorder, store *Store) (stacksdk.Response, error) {
	model := s.distanceModel(ctx, store)
	st := s.listenState(ctx, s.h)
	v := LiveView{Listening: st.Listening, Cycles: st.Cycles, Model: model, Devices: []LiveDevice{}, Rings: map[string]int{}}
	for _, sm := range rec.LiveSnapshot() {
		dev, ok, err := store.Device(ctx, sm.tag)
		if err != nil {
			return plainError(http.StatusInternalServerError, "the air could not be read")
		}
		if !ok {
			continue
		}
		near, far := model.Range(sm.signal, sm.channel)
		d := LiveDevice{
			ID: shortID(sm.tag), Mine: dev.Owner, Class: dev.Class, Dbm: int(math.Round(sm.signal)), NearM: round1(near), FarM: round1(far),
			Ring: RingOf(near, far), Angle: angleOf(sm.tag), Trend: TrendOf(sm.trail), Channel: sm.channel, Heard: sm.heard,
		}
		if d.Class == "" {
			d.Class = string(ClassUnknown)
		}
		if dev.Owner {
			d.Label = dev.Label
		}
		v.Rings[d.Ring]++
		v.Devices = append(v.Devices, d)
	}
	sort.Slice(v.Devices, func(i, j int) bool { return v.Devices[i].Dbm > v.Devices[j].Dbm })
	return jsonResponse(http.StatusOK, v)
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
