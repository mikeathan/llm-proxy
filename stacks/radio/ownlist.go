package radio

import (
	"context"
	"time"

	"llm-proxy-stacks/sdk"
)

// The owner's Wi-Fi networks are one list, the core's (ADR-0063): the names the owner watches and the access points the owner confirmed, both
// on the Network page. The stack reads it through the ownnetworks port, as tags under its own access-point purpose, and keeps no list of its
// own: a beacon under one of those names proposes an access point (a row in own_aps, for its channel, protection and guest flag), and only
// the core's list makes it the owner's. What the stack heard of those names goes back to the core in the air report (airAPs), where the one
// look-alike check judges it.

// SetNetworks gives the recorder the core's list of the owner's networks. Without it no name is watched and no access point is the owner's.
func (r *Recorder) SetNetworks(p stacksdk.OwnNetworks) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	r.netPort = p
}

// SyncOwnNetworks reads the core's list (the names and the confirmed access points, as this epoch's tags) and reloads which access points
// are the owner's. Call it when a run starts, at each cycle, after the epoch changes and after a guest flag changes. When the port cannot
// answer, the last list stays.
func (r *Recorder) SyncOwnNetworks(ctx context.Context) error {
	r.cmu.Lock()
	port := r.netPort
	r.cmu.Unlock()
	if port != nil {
		e, err := r.currentEpoch(ctx)
		if err != nil {
			return err
		}
		if nets, err := port.Networks(ctx, purpose(tagAP, e)); err == nil {
			r.applyOwnList(nets)
		}
	}
	return r.RefreshOwnAPs(ctx)
}

// syncAfterRotation re-reads the core's list under the new epoch's purpose, so the confirmed tags match the retagged rows. r.mu is held by
// ForgetNeighbours, so the epoch is read directly.
func (r *Recorder) syncAfterRotation(ctx context.Context) error {
	r.cmu.Lock()
	port := r.netPort
	r.cmu.Unlock()
	if port != nil {
		if nets, err := port.Networks(ctx, purpose(tagAP, r.epoch)); err == nil {
			r.applyOwnList(nets)
		} else {
			r.applyOwnList(nil) // the old epoch's tags match nothing now
		}
	}
	return r.RefreshOwnAPs(ctx)
}

// applyOwnList keeps the names and the confirmed tags of one answer.
func (r *Recorder) applyOwnList(nets []stacksdk.OwnNetwork) {
	names, confirmed := map[string]bool{}, map[string]bool{}
	for _, n := range nets[:min(len(nets), stacksdk.MaxOwnNetworks)] {
		if n.Name == "" {
			continue
		}
		names[n.Name] = true
		for _, t := range n.Confirmed {
			confirmed[string(t)] = true
		}
	}
	r.cmu.Lock()
	r.ownNames, r.ownConfirmed = names, confirmed
	r.cmu.Unlock()
}

// OwnNames are the names of the core's list, as last read.
func (r *Recorder) OwnNames() []string {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	out := make([]string, 0, len(r.ownNames))
	for n := range r.ownNames {
		out = append(out, n)
	}
	return out
}

// OwnAPs is the stored access points under the owner's names, each marked Confirmed when the core's list confirms it.
func (r *Recorder) OwnAPs(ctx context.Context) ([]OwnAP, error) {
	aps, err := r.store.OwnAPs(ctx)
	if err != nil {
		return nil, err
	}
	r.cmu.Lock()
	defer r.cmu.Unlock()
	for i := range aps {
		aps[i].Confirmed = r.ownConfirmed[string(aps[i].Tag)]
	}
	return aps, nil
}

// airAPHeardDays is how many days back an access point counts as heard lately for the core's look-alike check: today and yesterday.
const airAPHeardDays = 1

// airAPs is what the air report tells the core of the access points heard lately under the owner's names: each one's tag and its address
// sealed, both under this epoch's access-point purpose (the purpose the ownnetworks port was asked for), so the core can check one against
// the other and nothing else opens it. At most MaxAirAPs, in the store's order.
func (r *Recorder) airAPs(ctx context.Context, now time.Time) ([]stacksdk.AirAP, error) {
	aps, err := r.OwnAPs(ctx)
	if err != nil {
		return nil, err
	}
	e, err := r.currentEpoch(ctx)
	if err != nil {
		return nil, err
	}
	p, since := purpose(tagAP, e), dayNumber(now)-airAPHeardDays
	out := []stacksdk.AirAP{}
	for _, a := range aps {
		if len(out) >= stacksdk.MaxAirAPs {
			break
		}
		if a.LastDay < since || len(a.Sealed) == 0 {
			continue
		}
		mac, err := r.vault.Open(ctx, sealOwner, a.Sealed)
		if err != nil {
			continue // a row that will not open is from before a key rotation
		}
		sealed, err := r.vault.Seal(ctx, p, mac)
		if err != nil {
			return nil, err
		}
		out = append(out, stacksdk.AirAP{Name: a.Name, Tag: a.Tag, Sealed: sealed, Security: a.Security, Channel: a.Channel,
			Heard: dayTime(a.LastDay), ConfirmedBefore: a.ConfirmedBefore})
	}
	return out, nil
}
