package netaudit

import (
	"context"
	"fmt"
	"time"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/models"
)

const (
	// phoneHomeAlertEvery is how often the watch is looked at for something to tell.
	phoneHomeAlertEvery = 15 * time.Minute
	// maxPhoneHomeAlertsPerTick bounds the messages of one look, so a burst of devices cannot flood the phone; the rest wait for the next.
	maxPhoneHomeAlertsPerTick = 5
)

// PhoneHomeAlertDeps are what the alerts need. Notify sends one message to the operator's connectors through the alert log and fails
// when it could not be delivered, so the alert is tried again at the next look.
type PhoneHomeAlertDeps struct {
	Watch     *phonehome.Watch
	Enabled   func() bool
	Inventory func() (models.DeviceInventory, error)
	Notify    func(ctx context.Context, message string) error
	OnError   func(error)
	Now       func() time.Time
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
	for _, s := range phonehome.Evaluate(a.deps.Watch.Snapshot(), PhoneHomeDevices(inv), now) {
		ip, known := ips[s.MAC]
		key := s.MAC + "|" + string(s.Kind) + "|" + namesDigest(s.Names)
		if !known || s.Kind == phonehome.AroundDNS || a.deps.Watch.WasTold(key) {
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

func (a *PhoneHomeAlerts) report(err error) {
	if a.deps.OnError != nil {
		a.deps.OnError(err)
	}
}

// phoneHomeMessage is the alert in fixed words: the device's address and the vetted DNS names.
func phoneHomeMessage(s phonehome.Signal, ip string) string {
	if s.Kind == phonehome.Beaconing {
		return fmt.Sprintf("%s keeps asking for a new name at a steady beat: %s. Open the Network page to review it.", ip, listNames(s.Names))
	}
	return fmt.Sprintf("%s asked for a name it had not asked for in its first week: %s. Open the Network page to review it.", ip, listNames(s.Names))
}
