package radio

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	ownNames map[string]bool
	ownAPs   map[string]bool
	live     map[string]*liveDev
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
	return r.store.SeenDevice(ctx, Seen{Tag: tag, Owner: true, Sealed: sealed, Label: label, Class: string(class), At: at})
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
}

// DropCycle discards the open cycle: a run that stopped halfway through one has not listened on every channel, so counting it would bias every
// share it feeds.
func (r *Recorder) DropCycle() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.cycle = nil
}

// EndCycle writes the finished cycle and then lets the classifier look at what it changed.
func (r *Recorder) EndCycle(ctx context.Context, at time.Time) error {
	r.cmu.Lock()
	heard := r.cycle
	r.cycle = nil
	r.absorbCycle(heard)
	r.cmu.Unlock()
	if heard == nil {
		return nil
	}
	if err := r.store.FlushCycle(ctx, at, heard); err != nil {
		return err
	}
	_, err := r.store.ClassifyAll(ctx, at)
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
	set := map[string]bool{}
	for _, a := range aps {
		if a.Confirmed {
			set[string(a.Tag)] = true
		}
	}
	r.cmu.Lock()
	r.ownAPs = set
	r.cmu.Unlock()
	return nil
}

// sight records the sender and says whether it is one of the owner's devices.
func (r *Recorder) sight(ctx context.Context, tag []byte, f Frame, at time.Time) (bool, error) {
	owner, err := r.store.TouchOwner(ctx, tag, f.Signal, f.SignalOK, at)
	if err != nil || owner {
		return owner, err
	}
	return false, r.store.SeenDevice(ctx, Seen{Tag: tag, Signal: f.Signal, SignalOK: f.SignalOK, At: at})
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
	return r.RefreshOwnAPs(ctx)
}
