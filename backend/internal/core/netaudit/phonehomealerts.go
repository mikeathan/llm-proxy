package netaudit

import (
	"context"
	"fmt"
	"slices"
	"time"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/models"
)

const (
	// phoneHomeAlertEvery is how often the watch is looked at for something to tell.
	phoneHomeAlertEvery = 15 * time.Minute
	// phoneHomeAlertGap is the least time between two messages to the operator about one device and one kind of thing.
	phoneHomeAlertGap = 24 * time.Hour
	// maxPhoneHomeAlertsPerTick bounds the messages of one look, so a burst of devices cannot flood the phone; the rest wait for the next.
	maxPhoneHomeAlertsPerTick = 5
)

// PhoneHomeAlertDeps are what the alerts need. Notify sends one message to the operator's connectors through the alert log and fails
// when it could not be delivered, so the alert is tried again at the next look.
type PhoneHomeAlertDeps struct {
	Watch     *phonehome.Watch
	Enabled   func() bool
	Inventory func() (models.DeviceInventory, error)
	// KnownBad says whether a name is on the list of hosts known to serve malware (and which listed name matched); nil means no list.
	KnownBad func(name string) (string, bool)
	Notify   func(ctx context.Context, message string) error
	OnError  func(error)
	Now      func() time.Time
}

// PhoneHomeAlerts tells the operator, in fixed words, when a device asks for a name it had not asked for in its first week or asks for
// a new name at a steady beat. A device that avoids the router's DNS is a finding for the nightly report, never a message. Each set of
// names is told once, and a larger set is told again. The device is named by its address, never by the name the operator gave it.
type PhoneHomeAlerts struct{ deps PhoneHomeAlertDeps }

func NewPhoneHomeAlerts(deps PhoneHomeAlertDeps) *PhoneHomeAlerts {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &PhoneHomeAlerts{deps: deps}
}

// Run looks every few minutes until ctx ends.
func (a *PhoneHomeAlerts) Run(ctx context.Context) {
	tick := time.NewTicker(phoneHomeAlertEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.Tick(ctx)
		}
	}
}

// Tick looks once. It sends at most maxPhoneHomeAlertsPerTick messages and marks each told only after it was delivered.
func (a *PhoneHomeAlerts) Tick(ctx context.Context) {
	if !a.deps.Enabled() {
		return
	}
	inv, err := a.deps.Inventory()
	if err != nil {
		a.report(fmt.Errorf("failed to read the device list for the phone-home alerts: %w", err))
		return
	}
	ips := map[string]string{}
	for _, d := range inv.Devices {
		if d.IP != "" {
			ips[d.MAC] = d.IP
		}
	}
	now := a.deps.Now()
	sent := 0
	for _, s := range a.signals(inv, now) {
		ip, known := ips[s.MAC]
		key := s.MAC + "|" + string(s.Kind) + "|" + namesDigest(s.Names)
		if !known || a.deps.Watch.WasTold(key) || a.heldBack(s, now) {
			continue
		}
		if sent >= maxPhoneHomeAlertsPerTick || ctx.Err() != nil {
			return
		}
		sent++
		if err := a.deps.Notify(ctx, phoneHomeMessage(s, ip)); err != nil {
			continue
		}
		a.deps.Watch.MarkTold(key, now)
	}
}

// heldBack says whether a device was already told of this kind of thing within the last day. A device that meets several new names over
// a few days would otherwise send a message for each growth of the set; the nightly report lists them all. A name on the malware list
// is never held back.
func (a *PhoneHomeAlerts) heldBack(s phonehome.Signal, now time.Time) bool {
	if s.Kind == phonehome.KnownBadName {
		return false
	}
	return a.deps.Watch.ToldSince(s.MAC+"|"+string(s.Kind)+"|", now.Add(-phoneHomeAlertGap))
}

// signals are what is worth a message: new places and beats, and names on the malware list. A device that avoids the router's DNS is
// left to the nightly report.
func (a *PhoneHomeAlerts) signals(inv models.DeviceInventory, now time.Time) []phonehome.Signal {
	state := a.deps.Watch.Snapshot()
	all := phonehome.WithKnownBad(phonehome.Evaluate(state, PhoneHomeDevices(inv), now), phonehome.KnownBad(state, a.deps.KnownBad, now))
	return slices.DeleteFunc(all, func(s phonehome.Signal) bool { return s.Kind == phonehome.AroundDNS })
}

func (a *PhoneHomeAlerts) report(err error) {
	if a.deps.OnError != nil {
		a.deps.OnError(err)
	}
}

// phoneHomeMessage is the alert in fixed words: the device's address and the vetted DNS names.
func phoneHomeMessage(s phonehome.Signal, ip string) string {
	if s.Kind == phonehome.KnownBadName {
		return fmt.Sprintf("%s asked for a name on abuse.ch's list of hosts that serve malware: %s. Open the Network page to review it.", ip, listNames(s.Names))
	}
	if s.Kind == phonehome.Beaconing {
		return fmt.Sprintf("%s keeps asking for a new name at a steady beat: %s. Open the Network page to review it.", ip, listNames(s.Names))
	}
	return fmt.Sprintf("%s asked for a name it had not asked for in its first week: %s. Open the Network page to review it.", ip, listNames(s.Names))
}
