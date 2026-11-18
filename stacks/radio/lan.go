package radio

import (
	"context"
	"time"

	"llm-proxy-stacks/sdk"
)

// The join to the network's inventory (ADR-0058): the core tags every device it knows on the local network under this stack's own purpose and
// epoch, so a tag here equals the tag of a device heard over the air with the same address. Nothing is stored: the names are the operator's,
// shown on the Radio page only, and never put in the digest, a report or an alert.

const lanCacheFor = time.Minute

// LANInfo is what the network inventory says about a device heard over the air.
type LANInfo struct {
	Name   string
	Router bool
	// Presence is what the network's presence check says of the device now (empty when it says nothing).
	Presence stacksdk.LANPresence
	// Self is the name of this computer's own interface when the device is this computer's Wi-Fi card (host.go); it wins over the inventory.
	Self string
}

// SetLAN gives the recorder the core's LAN port; nil leaves the join off.
func (r *Recorder) SetLAN(l stacksdk.LAN) {
	r.lmu.Lock()
	defer r.lmu.Unlock()
	r.lan, r.lanIndex, r.lanAt = l, nil, time.Time{}
}

// LANIndex is the inventory by tag, read at most once a minute. An inventory that cannot be read is an empty index: the join is evidence, never a
// condition for listening.
func (r *Recorder) LANIndex(ctx context.Context) map[string]LANInfo {
	r.lmu.Lock()
	defer r.lmu.Unlock()
	if r.lan == nil && len(r.host) == 0 {
		return nil
	}
	if r.lanIndex != nil && time.Since(r.lanAt) < lanCacheFor {
		return r.lanIndex
	}
	e, err := r.currentEpoch(ctx)
	if err != nil {
		return r.lanIndex
	}
	idx, err := r.inventoryIndex(ctx, e)
	if err != nil {
		return r.lanIndex
	}
	r.addHost(ctx, idx, e)
	r.lanIndex, r.lanAt = idx, time.Now()
	return idx
}

// inventoryIndex is the network inventory by tag; empty when the join is off. lmu is held.
func (r *Recorder) inventoryIndex(ctx context.Context, epoch int) (map[string]LANInfo, error) {
	idx := map[string]LANInfo{}
	if r.lan == nil {
		return idx, nil
	}
	list, err := r.lan.Devices(ctx, purpose(tagDevice, epoch))
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		cur, has := idx[string(d.Tag)]
		if has && !cur.Router {
			continue // a device's own entry wins over the router's neighbouring addresses
		}
		idx[string(d.Tag)] = LANInfo{Name: d.Name, Router: d.Router && (!has || cur.Router), Presence: lanPresence(d.Presence)}
	}
	return idx, nil
}

// lanPresence keeps a presence the SDK knows and drops any other word.
func lanPresence(p stacksdk.LANPresence) stacksdk.LANPresence {
	if !p.Valid() {
		return stacksdk.PresenceUnknown
	}
	return p
}

// addHost marks this computer's own wireless interfaces in the index (host.go); they win over the inventory. lmu is held.
func (r *Recorder) addHost(ctx context.Context, idx map[string]LANInfo, epoch int) {
	for _, h := range r.host {
		if t, err := r.vault.Tag(ctx, purpose(tagDevice, epoch), h.MAC[:]); err == nil {
			idx[string(t)] = LANInfo{Self: h.Iface}
		}
	}
}

// isOwnAPAny says whether an access point is one of the owner's confirmed ones, guest networks included.
func (r *Recorder) isOwnAPAny(ctx context.Context, ap MAC) (bool, error) {
	t, err := r.tag(ctx, tagAP, ap[:])
	if err != nil {
		return false, err
	}
	r.cmu.Lock()
	defer r.cmu.Unlock()
	return r.ownAll[string(t)], nil
}
