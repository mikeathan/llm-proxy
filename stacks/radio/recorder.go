package radio

import (
	"context"
	"errors"
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
}

// NewRecorder returns a Recorder over the vault and the store.
func NewRecorder(v stacksdk.Vault, s *Store) *Recorder { return &Recorder{vault: v, store: s} }

// AddOwnerDevice marks an address as the owner's own device: the row keeps the sealed address and the label until the owner deletes it.
func (r *Recorder) AddOwnerDevice(ctx context.Context, mac MAC, label string, at time.Time) error {
	tag, err := r.vault.Tag(ctx, tagDevice, mac[:])
	if err != nil {
		return err
	}
	sealed, err := r.vault.Seal(ctx, sealOwner, mac[:])
	if err != nil {
		return err
	}
	return r.store.SeenDevice(ctx, Seen{Tag: tag, Owner: true, Sealed: sealed, Label: label, At: at})
}

// Record stores what one frame says: the sender's sighting, an access point's beacon, and a join, leave or refusal as a count.
func (r *Recorder) Record(ctx context.Context, f Frame, at time.Time) error {
	if f.Kind == KindBeacon || f.Kind == KindProbeResponse {
		return r.recordBeacon(ctx, f, at)
	}
	if !f.HasTransmitter || f.Transmitter[0]&1 != 0 { // a sender is never a group address
		return nil
	}
	tag, err := r.vault.Tag(ctx, tagDevice, f.Transmitter[:])
	if err != nil {
		return err
	}
	owner, err := r.sight(ctx, tag, f, at)
	if err != nil {
		return err
	}
	if kind := eventKind(f.Kind); kind != "" {
		return r.store.AddEvent(ctx, kind, tag, owner, at)
	}
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
	tag, err := r.vault.Tag(ctx, tagAP, f.BSSID[:])
	if err != nil {
		return err
	}
	return r.store.SeenBeacon(ctx, Beacon{Tag: tag, Name: f.SSID, Security: string(f.Security.Mode), Channel: f.Channel, At: at})
}
