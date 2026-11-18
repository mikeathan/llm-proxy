package radio

import "context"

// What one cycle learns about who is connected to which network, what each device says it is, and which access points are radios of one box
// (ADR-0058). It is gathered in memory while the cycle runs, keyed by address because the address is what a frame carries, and written as tags
// when the cycle ends; no address outlives the cycle except in the run's bounded list of access points.

const (
	tagBox = "radio_box"
	// maxCycleLinks bounds the devices one cycle remembers a network for, maxLinkNets the networks per device, maxKnownAPs the access points a
	// run remembers: a flood of forged frames cannot grow any of them.
	maxCycleLinks = 2048
	maxLinkNets   = 4
	maxKnownAPs   = 1024
	maxCycleIdent = 2048
)

// isGroup says whether an address is a group (multicast or broadcast) address, which never names one device.
func isGroup(m MAC) bool { return m[0]&1 != 0 }

// linkOf is the device and the access point a frame shows talking to each other, when it shows that. known says whether an address is an access
// point this run has heard beacon.
func linkOf(f Frame, known func(MAC) bool) (client, ap MAC, ok bool) {
	switch f.Kind {
	case KindData:
		client, ap, ok = dataLink(f)
	case KindAssociationRequest:
		client, ap, ok = f.Transmitter, f.BSSID, f.HasBSSID
	case KindControl: // RTS and block acknowledgements carry both ends; one of them must be a known access point
		client, ap, ok = controlLink(f, known)
	}
	return client, ap, ok && client != ap && !isGroup(client) && !isGroup(ap)
}

// dataLink: a data frame names its network; when the access point sends, the device is the receiver.
func dataLink(f Frame) (client, ap MAC, ok bool) {
	if !f.HasBSSID || !f.HasTransmitter {
		return MAC{}, MAC{}, false
	}
	if f.Transmitter == f.BSSID {
		return f.Receiver, f.BSSID, true
	}
	return f.Transmitter, f.BSSID, true
}

func controlLink(f Frame, known func(MAC) bool) (client, ap MAC, ok bool) {
	switch {
	case !f.HasTransmitter:
		return MAC{}, MAC{}, false
	case known(f.Transmitter):
		return f.Receiver, f.Transmitter, true
	case known(f.Receiver):
		return f.Transmitter, f.Receiver, true
	}
	return MAC{}, MAC{}, false
}

// noteLink remembers that a device exchanged a frame with an access point in this cycle. cmu is held.
func (r *Recorder) noteLink(f Frame) {
	if r.cycle == nil {
		return
	}
	client, ap, ok := linkOf(f, func(m MAC) bool { _, k := r.knownAPs[m]; return k })
	if !ok {
		return
	}
	if r.links == nil {
		r.links = map[MAC]map[MAC]int{}
	}
	nets := r.links[client]
	if nets == nil {
		if len(r.links) >= maxCycleLinks {
			return
		}
		nets = map[MAC]int{}
		r.links[client] = nets
	}
	if _, has := nets[ap]; !has && len(nets) >= maxLinkNets {
		return
	}
	nets[ap]++
}

// noteKnownAP adds a beacon's sender to the run's access points. cmu is held.
func (r *Recorder) noteKnownAP(m MAC) {
	if r.knownAPs == nil {
		r.knownAPs = map[MAC]struct{}{}
	}
	if _, has := r.knownAPs[m]; has || len(r.knownAPs) >= maxKnownAPs {
		return
	}
	r.knownAPs[m] = struct{}{}
}

// noteIdentity merges what a frame said about the product into the cycle's record for its sender. cmu is held.
func (r *Recorder) noteIdentity(tag []byte, id Identity) {
	if r.cycle == nil || id == (Identity{}) {
		return
	}
	if r.idents == nil {
		r.idents = map[string]Identity{}
	}
	cur, has := r.idents[string(tag)]
	if !has && len(r.idents) >= maxCycleIdent {
		return
	}
	r.idents[string(tag)] = mergeIdentity(cur, id)
}

// mergeIdentity keeps every field either side announced, the later one winning.
func mergeIdentity(a, b Identity) Identity {
	if b.Manufacturer != "" {
		a.Manufacturer = b.Manufacturer
	}
	if b.Model != "" {
		a.Model = b.Model
	}
	if b.Category != 0 {
		a.Category = b.Category
	}
	if b.Hint != "" {
		a.Hint = b.Hint
	}
	if b.Signature != 0 {
		a.Signature = b.Signature
	}
	return a
}

// busiest is the access point a device exchanged most frames with (the lowest address on a tie, so the answer does not depend on map order).
func busiest(nets map[MAC]int) MAC {
	var best MAC
	n := -1
	for m, c := range nets {
		if c > n || (c == n && string(m[:]) < string(best[:])) {
			best, n = m, c
		}
	}
	return best
}

// writeLinks tags each device and the access point it used most this cycle and writes the link; a link to one of the owner's confirmed, non-guest
// access points is marked.
func (r *Recorder) writeLinks(ctx context.Context, links map[MAC]map[MAC]int) error {
	rows := make([]LinkRow, 0, len(links))
	for client, nets := range links {
		ap := busiest(nets)
		ct, err := r.tag(ctx, tagDevice, client[:])
		if err != nil {
			return err
		}
		nt, err := r.tag(ctx, tagDevice, ap[:])
		if err != nil {
			return err
		}
		own, err := r.isOwnAP(ctx, ap)
		if err != nil {
			return err
		}
		rows = append(rows, LinkRow{Tag: ct, Net: nt, Own: own})
	}
	return r.store.SetLinks(ctx, rows)
}

func (r *Recorder) isOwnAP(ctx context.Context, ap MAC) (bool, error) {
	t, err := r.tag(ctx, tagAP, ap[:])
	if err != nil {
		return false, err
	}
	r.cmu.Lock()
	defer r.cmu.Unlock()
	return r.ownAPs[string(t)], nil
}

// writeIdentities writes the cycle's identity facts onto the device rows.
func (r *Recorder) writeIdentities(ctx context.Context, idents map[string]Identity) error {
	rows := make([]IdentityRow, 0, len(idents))
	for tag, id := range idents {
		rows = append(rows, IdentityRow{Tag: []byte(tag), Manufacturer: id.Manufacturer, Model: id.Model, Category: id.Category, Hint: id.Hint, Signature: id.Signature})
	}
	return r.store.SetIdentities(ctx, rows)
}

// boxKey is what the radios of one router share. Routers give their networks (2.4 GHz, 5 GHz, guest, mesh backhaul) addresses from one block:
// the same address with the locally administered bit, or a few bits of the first byte, set, and the last three bits counting up. Clearing those
// bits gives one key per block of eight, which is stable whichever radio is heard first. Different makers never share a key, because the
// rest of the address (the maker's block) differs.
func boxKey(m MAC) MAC {
	k := m
	k[0] &^= 0x0e
	k[5] &^= 0x07
	return k
}
