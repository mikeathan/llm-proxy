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
	if r.lan == nil {
		return nil
	}
	if r.lanIndex != nil && time.Since(r.lanAt) < lanCacheFor {
		return r.lanIndex
	}
	e, err := r.currentEpoch(ctx)
	if err != nil {
		return r.lanIndex
	}
	list, err := r.lan.Devices(ctx, purpose(tagDevice, e))
	if err != nil {
		return r.lanIndex
	}
	idx := make(map[string]LANInfo, len(list))
	for _, d := range list {
		cur, has := idx[string(d.Tag)]
		if has && !cur.Router {
			continue // a device's own entry wins over the router's neighbouring addresses
		}
		idx[string(d.Tag)] = LANInfo{Name: d.Name, Router: d.Router && (!has || cur.Router)}
	}
	r.lanIndex, r.lanAt = idx, time.Now()
	return idx
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
