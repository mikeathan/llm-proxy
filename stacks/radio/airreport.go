package radio

import (
	"bytes"
	"context"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// The air report (ADR-0061): what the stack heard of the devices of the network inventory, as fixed values the core joins back to its own
// devices by tag. Only devices the core gave through the lan port are in it (never a neighbour's, never the router's neighbouring addresses,
// never this computer's own card), and nothing a frame said: no name, no maker, no model, no address. It reads what the stack already holds
// and sends nothing.

const (
	// airListenedHours is how far back a finished listening cycle counts as "the sensor listened lately".
	airListenedHours = 24
	// lastChannel24 is the highest 2.4 GHz channel; the plan lists no 6 GHz channel, so every other channel is 5 GHz.
	lastChannel24 = 14
	band24        = "2G"
	band5         = "5G"
	// foreignSinceDays is how many days back a join to a network that is not yours is offered as a finding, as the digest counts it.
	foreignSinceDays = 1
	// findingForeign is the finding id offered per device.
	findingForeign = "radio_join_foreign_network"
)

var _ stacksdk.AirReporter = (*Stack)(nil)

// Air answers the air report. A stack that has never recorded answers that it has not listened, with no device.
func (s *Stack) Air(ctx context.Context) (stacksdk.AirReport, error) {
	out := stacksdk.AirReport{Devices: []stacksdk.AirFact{}}
	h, started := s.snapshot()
	if !started {
		return out, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	if !s.settings(ctx, h).Enabled {
		return out, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is switched off")
	}
	if st, err := s.existingStore(); err != nil || st == nil {
		return out, nil
	}
	rec, store, err := s.recorder(h)
	if err != nil {
		return out, stacksdk.Errorf(stacksdk.CodeInternal, "the radio database could not be opened")
	}
	if out.Listened, err = s.listenedLately(ctx, store); err != nil {
		return out, stacksdk.Errorf(stacksdk.CodeInternal, "the radio database could not be read")
	}
	if at := rec.LastCycleAt(); at > 0 {
		out.LastCycle = time.Unix(at, 0).UTC()
	}
	out.Bands = bandsOf(s.settings(ctx, h).Plan().Channels)
	v, set, err := s.liveView(ctx, h, rec, store)
	if err != nil {
		return out, stacksdk.Errorf(stacksdk.CodeInternal, "the air could not be read")
	}
	out.Devices = inventoryFacts(v, set, rec.LANIndex(ctx))
	_ = rec.SyncOwnNetworks(ctx) // the names to report under, and this epoch's purpose asked of the port
	if out.APs, err = rec.airAPs(ctx, s.now()); err != nil {
		return out, stacksdk.Errorf(stacksdk.CodeInternal, "the access points could not be read")
	}
	return out, nil
}

// listenedLately says a listening cycle finished in the last airListenedHours.
func (s *Stack) listenedLately(ctx context.Context, store *Store) (bool, error) {
	hours, err := store.airHours(ctx, hourNumber(s.now())-airListenedHours+1)
	if err != nil {
		return false, err
	}
	for _, n := range hours {
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

// inventoryFacts is one fact per device of the live picture that the inventory gave by tag, at most MaxAirFacts.
func inventoryFacts(v LiveView, set liveSet, lan map[string]LANInfo) []stacksdk.AirFact {
	out := []stacksdk.AirFact{}
	for _, d := range v.Devices {
		row, ok := set.byShort[d.ID]
		if !ok || len(out) >= stacksdk.MaxAirFacts {
			continue
		}
		info, onLAN := lan[string(row.Tag)]
		if !onLAN || info.Router || info.Self != "" {
			continue
		}
		out = append(out, factOf(d, row))
	}
	return out
}

// factOf is what the picture says of one device. A device drawn from its stored days only (Usual) was not heard lately: it has no time, place
// or band, because its usual signal is not where it is now.
func factOf(d LiveDevice, row FeatureRow) stacksdk.AirFact {
	f := stacksdk.AirFact{Tag: slices.Clone(row.Tag), Kind: string(DeviceUnknown)}
	if d.Ident != nil {
		f.Kind, f.OnYourWiFi = string(d.Ident.Kind), d.Ident.OnYourWiFi
	}
	if d.Usual || !d.Heard || d.LastAt == 0 {
		return f
	}
	f.Heard = time.Unix(d.LastAt, 0).UTC()
	f.Ring, f.Moving, f.Band = d.Ring, d.Moving, bandOf(d.Channel)
	return f
}

// bandOf is the band of a channel, empty when the channel is not known.
func bandOf(channel int) string {
	switch {
	case channel <= 0:
		return ""
	case channel <= lastChannel24:
		return band24
	}
	return band5
}

// bandsOf is the bands a plan's channels cover, 2.4 GHz first.
func bandsOf(channels []int) []string {
	var out []string
	for _, b := range []string{band24, band5} {
		if slices.ContainsFunc(channels, func(c int) bool { return bandOf(c) == b }) {
			out = append(out, b)
		}
	}
	return out
}

// Findings offers, for each device of the network inventory that asked to join a network that is not yours lately, the finding by the tag the
// lan port gave it (ADR-0061): the core puts it on that device in the nightly report. Nothing else is offered: what the stack hears of access
// points and other people's devices stays on the Radio page.
func (s *Stack) Findings(ctx context.Context) ([]stacksdk.StackFinding, error) {
	h, started := s.snapshot()
	if !started || !s.settings(ctx, h).Enabled {
		return nil, nil // a stack switched off offers nothing, as its air report says nothing
	}
	if st, err := s.existingStore(); err != nil || st == nil {
		return nil, nil
	}
	rec, store, err := s.recorder(h)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "the radio database could not be opened")
	}
	counts, err := store.eventCounts(ctx, eventJoinForeign, dayNumber(s.now())-foreignSinceDays)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "the radio database could not be read")
	}
	lan := rec.LANIndex(ctx)
	var out []stacksdk.StackFinding
	for tag, n := range counts {
		if info, ok := lan[tag]; ok && !info.Router && info.Self == "" && n > 0 {
			out = append(out, stacksdk.StackFinding{ID: findingForeign, Device: []byte(tag), Count: n})
		}
	}
	slices.SortFunc(out, func(a, b stacksdk.StackFinding) int { return bytes.Compare(a.Device, b.Device) })
	return out, nil
}
