package radio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy-stacks/sdk"
)

// The purposes the stack asks the vault for. A tag of one purpose cannot be matched to a tag of another.
const (
	tagDevice = "radio_device"
	tagAP     = "radio_ap"
	sealOwner = "radio_owner"
)

// Recorder turns a parsed frame into rows. It is the only thing that sees an address: it hands the vault the address and stores the tag it
// gets back (and, for the owner's own devices only, the sealed address). A name a device asked for in a probe request is never read here.
type Recorder struct {
	vault stacksdk.Vault
	store *Store

	mu     sync.Mutex
	loaded bool
	epoch  int

	// cmu guards the open cycle, the owner's network names and the tags of the owner's confirmed access points.
	cmu      sync.Mutex
	cycle    map[string]*agg
	sighted  map[string]*sighting // what is already written of each sender this cycle, so a repeat frame inside it costs no write
	ownNames map[string]bool
	ownAPs   map[string]bool
	apNets   map[MAC]apNet       // the networks beacons announced this cycle, until the cycle ends (the address is only ever in memory)
	netNames map[string]string   // device tag to the network name it broadcasts: memory only, per run, never written (ADR-0053)
	links    map[MAC]map[MAC]int // this cycle: device to access point to frames exchanged (links.go)
	idents   map[string]Identity // this cycle: what each sender's frames said about the product (links.go)
	knownAPs map[MAC]struct{}    // this run: the access points heard beacon, bounded (links.go)
	ownAll   map[string]bool     // the owner's confirmed access points, guest ones included (lan.go)

	// lmu guards the LAN join (lan.go).
	lmu        sync.Mutex
	lan        stacksdk.LAN
	lanIndex   map[string]LANInfo
	lanAt      time.Time
	live       map[string]*liveDev
	tracks     map[string][]trackPt // memory only, per run (live.go)
	trackHours atomic.Int32         // 0: nothing finer than a day goes to the disk (ADR-0055)
	liveCycle  int
	lastCycle  int64                     // unix time of the last finished cycle of this run
	focus      map[string][]focusReading // the focus windows' readings: memory only, per run, bounded (focus.go)
	// The router's list of joined devices (mesh.go): the port, when it was last asked, the latest list and each device's router readings.
	meshPort   stacksdk.MeshClients
	meshAsked  time.Time
	meshListAt int64
	meshIdx    map[string]meshSeen
	meshTrail  map[string][]focusReading
}

// NewRecorder returns a Recorder over the vault and the store.
func NewRecorder(v stacksdk.Vault, s *Store) *Recorder { return &Recorder{vault: v, store: s} }

// AddOwnerDevice marks an address as the owner's own device: the row keeps the sealed address and the label until the owner deletes it.
func (r *Recorder) AddOwnerDevice(ctx context.Context, mac MAC, label string, at time.Time) error {
	return r.AddOwnerDeviceClass(ctx, mac, label, "", at)
}

// AddOwnerDeviceClass is AddOwnerDevice with the operator's own word on what class it is. A class given here is ground truth: the rules never
// change it. Left empty, the rules decide.
func (r *Recorder) AddOwnerDeviceClass(ctx context.Context, mac MAC, label string, class Class, at time.Time) error {
	tag, err := r.tag(ctx, tagDevice, mac[:])
	if err != nil {
		return err
	}
	sealed, err := r.vault.Seal(ctx, sealOwner, mac[:])
	if err != nil {
		return err
	}
	if err := r.store.SeenDevice(ctx, Seen{Tag: tag, Owner: true, Sealed: sealed, Label: label, Class: string(class), Maker: MakerOf(mac), At: at}); err != nil {
		return err
	}
	r.cmu.Lock()
	delete(r.sighted, string(tag)) // the open cycle must see it as the owner's from the next frame on
	r.cmu.Unlock()
	return nil
}

// Record stores what one frame says: the sender's sighting, an access point's beacon, and a join, leave or refusal as a count.
func (r *Recorder) Record(ctx context.Context, f Frame, at time.Time) error {
	_, err := r.RecordSeen(ctx, f, at)
	return err
}

// RecordSeen is Record that also says whether the frame was sent by one of the owner's devices, so a window can count them.
func (r *Recorder) RecordSeen(ctx context.Context, f Frame, at time.Time) (bool, error) {
	if f.Kind == KindBeacon || f.Kind == KindProbeResponse {
		return false, r.recordBeacon(ctx, f, at)
	}
	if !f.HasTransmitter || f.Transmitter[0]&1 != 0 { // a sender is never a group address
		return false, nil
	}
	tag, err := r.tag(ctx, tagDevice, f.Transmitter[:])
	if err != nil {
		return false, err
	}
	owner, err := r.sight(ctx, tag, f, at)
	if err != nil {
		return false, err
	}
	r.addToCycle(tag, f)
	if f.Kind == KindAssociationRequest && f.HasBSSID {
		if err := r.noteJoin(ctx, tag, owner, f.BSSID, at); err != nil {
			return owner, err
		}
	}
	if kind := eventKind(f.Kind); kind != "" {
		return owner, r.store.AddEvent(ctx, kind, tag, owner, at)
	}
	return owner, nil
}

// BeginCycle opens a cycle: until EndCycle, what each sender did is gathered, one entry a device however many frames it sent.
func (r *Recorder) BeginCycle() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.cycle = map[string]*agg{}
	r.apNets, r.links, r.idents = nil, nil, nil
	r.sighted = map[string]*sighting{}
}

// DropCycle discards the open cycle: a run that stopped halfway through one has not listened on every channel, so counting it would bias every
// share it feeds.
func (r *Recorder) DropCycle() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.cycle, r.sighted, r.apNets, r.links, r.idents = nil, nil, nil, nil, nil
}

// EndCycle writes the finished cycle and then lets the classifier look at what it changed.
func (r *Recorder) EndCycle(ctx context.Context, at time.Time) error {
	r.cmu.Lock()
	heard, nets, links, idents := r.cycle, r.apNets, r.links, r.idents
	r.cycle, r.apNets, r.links, r.idents = nil, nil, nil, nil
	r.absorbCycle(heard, at)
	r.cmu.Unlock()
	if heard == nil {
		return nil
	}
	if err := r.store.FlushCycle(ctx, at, heard); err != nil {
		return err
	}
	if err := r.writeNetworks(ctx, nets); err != nil {
		return err
	}
	if err := r.writeLinks(ctx, links); err != nil {
		return err
	}
	if err := r.writeIdentities(ctx, idents); err != nil {
		return err
	}
	if err := r.keepTrack(ctx, at, heard); err != nil {
		return err
	}
	_, err := r.store.ClassifyAllWith(ctx, at, r.LANIndex(ctx))
	return err
}

func (r *Recorder) addToCycle(tag []byte, f Frame) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.cycle == nil {
		return
	}
	a := r.cycle[string(tag)]
	if a == nil {
		a = &agg{}
		r.cycle[string(tag)] = a
	}
	a.add(f)
	if f.Transmitter[0]&2 != 0 { // a locally administered address: a phone's per-network random one
		a.random = true
	}
	r.noteIdentity(tag, f.Ident)
	r.noteLink(f)
}

// eventJoinForeign counts a device of the owner's asking to join an access point that is not one of the owner's confirmed ones.
const eventJoinForeign = "join_foreign"

// noteJoin records that a device asked to join an access point. A join to one of the owner's confirmed access points marks the device as the
// owner's evidence; a join by one of the owner's own devices to any other is counted as an event (no address, no name), but only once the owner
// has confirmed an access point at all, because before that every network is unknown.
func (r *Recorder) noteJoin(ctx context.Context, device []byte, owner bool, bssid MAC, at time.Time) error {
	ap, err := r.tag(ctx, tagAP, bssid[:])
	if err != nil {
		return err
	}
	r.cmu.Lock()
	own, anyOwn := r.ownAPs[string(ap)], len(r.ownAPs) > 0
	r.cmu.Unlock()
	switch {
	case own:
		return r.store.MarkJoinedOwn(ctx, device)
	case owner && anyOwn:
		return r.store.AddEvent(ctx, eventJoinForeign, device, true, at)
	}
	return nil
}

// SetOwnNetworks sets the names the owner's own networks go by. A beacon with one of these names proposes an access point; only the owner's
// confirmation makes it count.
func (r *Recorder) SetOwnNetworks(names []string) {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		if n != "" {
			set[n] = true
		}
	}
	r.cmu.Lock()
	r.ownNames = set
	r.cmu.Unlock()
}

// RefreshOwnAPs reloads the tags of the owner's confirmed access points; call it after a confirmation and after the epoch changes.
func (r *Recorder) RefreshOwnAPs(ctx context.Context) error {
	aps, err := r.store.OwnAPs(ctx)
	if err != nil {
		return err
	}
	set, all := map[string]bool{}, map[string]bool{}
	for _, a := range aps {
		if a.Confirmed && !a.Guest {
			set[string(a.Tag)] = true // a guest network is the owner's, but a device on it is a visitor's
		}
		if a.Confirmed {
			all[string(a.Tag)] = true
		}
	}
	r.cmu.Lock()
	r.ownAPs, r.ownAll = set, all
	r.cmu.Unlock()
	return nil
}

// sighting is what a cycle has already written of one sender: whose it is, the day, and the signal range stored. A later frame of the same
// sender on the same day with a signal inside that range would change no stored value, so it is not written (a busy window repeats the same
// few devices thousands of times).
type sighting struct {
	owner  bool
	day    int64
	lo, hi int
	hasSig bool
}

// covers says whether writing this frame would change nothing already stored for the sender.
func (g *sighting) covers(day int64, f Frame) bool {
	return g.day == day && (!f.SignalOK || (g.hasSig && f.Signal >= g.lo && f.Signal <= g.hi))
}

func (g *sighting) widen(f Frame) {
	if !f.SignalOK {
		return
	}
	if !g.hasSig {
		g.lo, g.hi, g.hasSig = f.Signal, f.Signal, true
		return
	}
	g.lo, g.hi = min(g.lo, f.Signal), max(g.hi, f.Signal)
}

// sight records the sender and says whether it is one of the owner's devices. A sender already written this cycle with a signal inside the
// stored range is answered from memory.
func (r *Recorder) sight(ctx context.Context, tag []byte, f Frame, at time.Time) (bool, error) {
	day := dayNumber(at)
	r.cmu.Lock()
	g := r.sighted[string(tag)]
	if g != nil && g.covers(day, f) {
		owner := g.owner
		r.cmu.Unlock()
		return owner, nil
	}
	r.cmu.Unlock()
	owner, err := r.store.TouchOwner(ctx, tag, f.Signal, f.SignalOK, at)
	if err == nil && !owner {
		err = r.store.SeenDevice(ctx, Seen{Tag: tag, Maker: MakerOf(f.Transmitter), Signal: f.Signal, SignalOK: f.SignalOK, At: at})
	}
	if err != nil {
		return owner, err
	}
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.sighted != nil {
		if g == nil || g.day != day {
			g = &sighting{owner: owner, day: day}
			r.sighted[string(tag)] = g
		}
		g.widen(f)
	}
	return owner, nil
}

func eventKind(k FrameKind) string {
	switch k {
	case KindAuthentication:
		return "authentication"
	case KindAssociationRequest:
		return "association"
	case KindDeauthentication:
		return "deauthentication"
	case KindDisassociation:
		return "disassociation"
	}
	return ""
}

func (r *Recorder) recordBeacon(ctx context.Context, f Frame, at time.Time) error {
	if !f.HasBSSID {
		return errors.New("radio: a beacon without a BSSID")
	}
	tag, err := r.tag(ctx, tagAP, f.BSSID[:])
	if err != nil {
		return err
	}
	r.cmu.Lock()
	proposes := f.SSID != "" && r.ownNames[f.SSID]
	r.cmu.Unlock()
	if proposes {
		sealed, err := r.vault.Seal(ctx, sealOwner, f.BSSID[:])
		if err != nil {
			return err
		}
		if err := r.store.ProposeOwnAP(ctx, OwnAP{Tag: tag, Sealed: sealed, Name: f.SSID, Channel: f.Channel, Security: string(f.Security.Mode), PMF: pmfOf(f.Security)}, at); err != nil {
			return err
		}
	}
	r.noteNetwork(f)
	return r.store.SeenBeacon(ctx, Beacon{Tag: tag, Name: f.SSID, Security: string(f.Security.Mode), Channel: f.Channel, Signal: f.Signal, SignalOK: f.SignalOK, At: at})
}

// pmfOf says whether a beacon's security protects management frames (802.11w): the state a deauthentication flood depends on.
func pmfOf(s Security) string {
	switch {
	case s.PMFRequired:
		return PMFRequired
	case s.PMFCapable:
		return PMFCapable
	}
	return PMFOff
}

// purpose is the vault purpose of a tag in the current epoch. Epoch 0 is the plain name, so a database made before any rotation still matches.
func purpose(base string, epoch int) string {
	if epoch == 0 {
		return base
	}
	return fmt.Sprintf("%s_e%d", base, epoch)
}

func (r *Recorder) currentEpoch(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded {
		e, err := r.store.Epoch(ctx)
		if err != nil {
			return 0, err
		}
		r.epoch, r.loaded = e, true
	}
	return r.epoch, nil
}

// tag is the vault's tag of an address for a purpose in the current epoch.
func (r *Recorder) tag(ctx context.Context, base string, value []byte) ([]byte, error) {
	e, err := r.currentEpoch(ctx)
	if err != nil {
		return nil, err
	}
	return r.vault.Tag(ctx, purpose(base, e), value)
}

// ForgetNeighbours removes everything about other people's devices and access points and starts a new epoch, so no earlier tag matches again, not
// even one from a copy of the file. The owner's devices are tagged again from their sealed addresses and keep their rows and labels. It
// holds the recorder's lock for the whole change, so no frame is recorded under a half-rotated epoch.
func (r *Recorder) ForgetNeighbours(ctx context.Context) error {
	if _, err := r.currentEpoch(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	next := r.epoch + 1
	owners, err := r.store.OwnerRows(ctx)
	if err != nil {
		return err
	}
	retag := make(map[string][]byte, len(owners))
	for _, o := range owners {
		if len(o.Sealed) == 0 {
			continue // marked from the radar: only its tag was ever known, so it cannot be tagged again and ends with the rotation
		}
		mac, err := r.vault.Open(ctx, sealOwner, o.Sealed)
		if err != nil {
			continue // a row that will not open is dropped by Rotate
		}
		nt, err := r.vault.Tag(ctx, purpose(tagDevice, next), mac)
		if err != nil {
			return err
		}
		retag[string(o.Tag)] = nt
	}
	aps, err := r.store.OwnAPs(ctx)
	if err != nil {
		return err
	}
	retagAP := make(map[string][]byte, len(aps))
	for _, a := range aps {
		mac, err := r.vault.Open(ctx, sealOwner, a.Sealed)
		if err != nil {
			continue
		}
		nt, err := r.vault.Tag(ctx, purpose(tagAP, next), mac)
		if err != nil {
			return err
		}
		retagAP[string(a.Tag)] = nt
	}
	if err := r.store.Rotate(ctx, next, retag, retagAP); err != nil {
		return err
	}
	r.epoch = next
	r.cmu.Lock()
	r.sighted = map[string]*sighting{} // every tag of the old epoch is gone
	r.cmu.Unlock()
	return r.RefreshOwnAPs(ctx)
}

// apNet is what a beacon said about the access point that sent it.
type apNet struct {
	name  string
	gen   int
	ident Identity
}

// maxCycleNets bounds the networks one cycle remembers: a flood of forged beacons cannot grow it.
const maxCycleNets = 512

// noteNetwork remembers a beacon's network name and generation until the cycle ends, when they are written onto the access point's device row.
func (r *Recorder) noteNetwork(f Frame) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.noteKnownAP(f.BSSID)
	if r.cycle == nil || (f.SSID == "" && f.Beacon.Generation == 0 && f.Ident == (Identity{})) {
		return
	}
	if r.apNets == nil {
		r.apNets = map[MAC]apNet{}
	}
	if _, known := r.apNets[f.BSSID]; !known && len(r.apNets) >= maxCycleNets {
		return
	}
	n := r.apNets[f.BSSID]
	if f.SSID != "" {
		n.name = f.SSID
	}
	n.gen = max(n.gen, f.Beacon.Generation)
	n.ident = mergeIdentity(n.ident, f.Ident)
	r.apNets[f.BSSID] = n
}

// writeNetworks tags the remembered access points, writes the Wi-Fi generation their beacons announced onto their device rows and keeps the
// network names in memory for the run: a neighbour's network name is shown while it is heard and is never written to the disk.
func (r *Recorder) writeNetworks(ctx context.Context, nets map[MAC]apNet) error {
	infos := make([]NetInfo, 0, len(nets))
	names := make(map[string]string, len(nets))
	idents := make([]IdentityRow, 0, len(nets))
	boxes := make([]BoxRow, 0, len(nets))
	mine := make(map[string]bool, len(nets))
	for mac, n := range nets {
		tag, err := r.tag(ctx, tagDevice, mac[:])
		if err != nil {
			return err
		}
		own, err := r.isOwnAPAny(ctx, mac)
		if err != nil {
			return err
		}
		mine[string(tag)] = own
		key := boxKey(mac)
		box, err := r.tag(ctx, tagBox, key[:])
		if err != nil {
			return err
		}
		infos = append(infos, NetInfo{Tag: tag, Gen: n.gen})
		boxes = append(boxes, BoxRow{Tag: tag, Box: box})
		if n.ident != (Identity{}) {
			idents = append(idents, IdentityRow{Tag: tag, Manufacturer: n.ident.Manufacturer, Model: n.ident.Model, Category: n.ident.Category, Hint: n.ident.Hint})
		}
		if n.name != "" {
			names[string(tag)] = n.name
		}
	}
	r.rememberNames(names)
	if err := r.store.SetWifiGenerations(ctx, infos); err != nil {
		return err
	}
	if err := r.store.SetBoxes(ctx, boxes); err != nil {
		return err
	}
	if err := r.store.SetOwnAPRows(ctx, mine); err != nil {
		return err
	}
	return r.store.SetIdentities(ctx, idents)
}

// rememberNames adds network names to the run's memory, bounded like the live picture.
func (r *Recorder) rememberNames(names map[string]string) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.netNames == nil {
		r.netNames = map[string]string{}
	}
	for tag, name := range names {
		if _, known := r.netNames[tag]; known || len(r.netNames) < liveMaxDevices {
			r.netNames[tag] = name
		}
	}
}

// NetworkName is the name the device with this tag broadcasts as a network, when it is an access point heard this run.
func (r *Recorder) NetworkName(tag []byte) string {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	return r.netNames[string(tag)]
}

// ForgetSighting drops what the open cycle remembers of one device, so its next frame is read afresh (for one that has just been marked as the owner's).
func (r *Recorder) ForgetSighting(tag []byte) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	delete(r.sighted, string(tag))
}
