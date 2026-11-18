package radio

import (
	"context"
	"time"

	"llm-proxy-stacks/sdk"
)

// The mesh router's list of joined devices (Constitution I.15, ADR-0059): while a watch runs, the run asks the core at most once every
// meshAskEvery for the router's latest list, and the picture shows, for each device the list has, which box it is joined to and that box's
// signal of it. It is the picture's only: kept in memory, cleared when the run ends, never stored, never a feature of the classifier and never
// evidence that a device is the owner's (a device on the guest network is shown as such).
const meshAskEvery = 30 * time.Second

// meshSeen is one device in the router's latest list.
type meshSeen struct {
	box     []byte
	boxName string
	main    bool
	band    string
	guest   bool
	rssi    int
	at      int64
}

// RouterSeen is what the router says of a dot: the box it is joined to (its short id and the operator's name for it, Main when it is the
// gateway), the band, whether it is the guest network, the box's signal, when the list was read, and whether that signal just moved.
type RouterSeen struct {
	Box     string `json:"box"`
	BoxName string `json:"box_name,omitempty"`
	Main    bool   `json:"main,omitempty"`
	Band    string `json:"band"`
	Guest   bool   `json:"guest,omitempty"`
	Dbm     int    `json:"dbm"`
	At      int64  `json:"at"`
	Moving  bool   `json:"moving,omitempty"`
}

// SetMesh gives the recorder the core's meshclients port; nil leaves the router's list off.
func (r *Recorder) SetMesh(m stacksdk.MeshClients) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.meshPort = m
}

// RefreshMesh asks for the router's list when the last ask is older than meshAskEvery. The core limits its own reads; this keeps the run from
// asking at every window. A list that cannot be read leaves the last one, which says when it was read.
func (r *Recorder) RefreshMesh(ctx context.Context, now time.Time) {
	r.cmu.Lock()
	port := r.meshPort
	due := port != nil && now.Sub(r.meshAsked) >= meshAskEvery
	if due {
		r.meshAsked = now
	}
	r.cmu.Unlock()
	if !due {
		return
	}
	e, err := r.currentEpoch(ctx)
	if err != nil {
		return
	}
	view, err := port.Clients(ctx, purpose(tagDevice, e))
	if err != nil {
		return
	}
	r.absorbMesh(view)
}

// absorbMesh replaces the list and adds each device's signal to its short ring of router readings, bounded like the focus readings.
func (r *Recorder) absorbMesh(view stacksdk.MeshView) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	at := view.At.Unix()
	if r.meshTrail == nil {
		r.meshTrail = map[string][]focusReading{}
	}
	fresh := at != r.meshListAt
	r.meshListAt = at
	idx := make(map[string]meshSeen, len(view.Devices))
	for _, d := range view.Devices {
		if len(idx) >= liveMaxDevices {
			break
		}
		tag := string(d.Tag)
		idx[tag] = meshSeen{box: d.Box, boxName: d.BoxName, main: d.Main, band: d.Band, guest: d.Guest, rssi: d.RSSI, at: at}
		if !fresh {
			continue
		}
		pts := append(r.meshTrail[tag], focusReading{at: at, signal: float64(d.RSSI)})
		r.meshTrail[tag] = pts[max(0, len(pts)-focusReadings):]
	}
	oldest := view.At.Add(-focusKeepFor).Unix()
	for tag, pts := range r.meshTrail {
		if pts[len(pts)-1].at < oldest {
			delete(r.meshTrail, tag)
		}
	}
	r.meshIdx = idx
}

// RouterOf is what the router's latest list says of a device, nil when it is not in it.
func (r *Recorder) RouterOf(tag []byte) *RouterSeen {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	m, ok := r.meshIdx[string(tag)]
	if !ok {
		return nil
	}
	return &RouterSeen{Box: shortID(m.box), BoxName: m.boxName, Main: m.main, Band: m.band, Guest: m.guest, Dbm: m.rssi, At: m.at,
		Moving: movingOf(0, false, r.meshTrail[string(tag)])}
}

// hasMesh says whether the recorder was given the router's list.
func (r *Recorder) hasMesh() bool {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	return r.meshPort != nil
}
