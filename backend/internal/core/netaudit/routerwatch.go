package netaudit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
)

const (
	// routerSignInFailures failed sign-ins to the router's web page within routerSignInWindow are told. The figure is a starting
	// point: the owner's capture (2026-11-06) shows a mistyped password and a wrong captcha are each one line, so five in ten
	// minutes is more than a slip of the hand.
	routerSignInFailures = 5
	routerSignInWindow   = 10 * time.Minute
	// routerDeauthBurst disconnects within routerDeauthWindow are told. The same capture holds seven in four minutes while a phone
	// had its Wi-Fi switched off and on twice, so twenty in a minute is well past ordinary use.
	routerDeauthBurst  = 20
	routerDeauthWindow = time.Minute
	// routerDeviceDeauths disconnects of one device within a day make a finding. It is a guess: a phone that roams between the router
	// and a mesh node is disconnected many times a day without a fault, so the limit is high and the finding low.
	routerDeviceDeauths = 100
	// routerMaxDevices is how many devices' disconnects are counted; one beyond it is not counted, so a forged flood of hardware
	// addresses cannot grow the table. routerMaxBursts bounds the list of bursts of each kind.
	routerMaxDevices = 256
	routerMaxBursts  = 32
	// routerDay is how long a count or a burst is kept.
	routerDay = 24 * time.Hour
	// routerAlertGap is the least time between two messages of one kind; a burst heard meanwhile is told when it has passed.
	routerAlertGap = time.Hour
	// routerWatchEvery is how often the watch looks for something to tell.
	routerWatchEvery = 30 * time.Second
)

// RouterWatchDeps are what the watch needs. Notify sends one message to the operator's connectors through the alert log and fails
// when it could not be delivered, so the message is tried again at the next look.
type RouterWatchDeps struct {
	Enabled func() bool
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
	Now     func() time.Time
}

// RouterWatch tells the operator, in fixed words, when the router's log shows a burst of failed sign-ins to its web page or a burst
// of Wi-Fi disconnects (what a forged-disconnect attack looks like). It keeps the times of the last few lines of each kind and
// nothing else: no address, no hardware address, no line (Constitution I.8). The log is unauthenticated UDP, so a forged line can
// cause a message to be sent and nothing else.
type RouterWatch struct {
	deps RouterWatchDeps

	mu      sync.Mutex
	fails   []time.Time // the last routerSignInFailures failed sign-ins, oldest first
	deauths []time.Time // the last routerDeauthBurst disconnects, oldest first
	pending map[routerBurst]bool
	told    map[routerBurst]time.Time
	bursts  map[routerBurst][]time.Time // when each burst was seen, for the last day
	devices map[string]deviceCount      // disconnects per hardware address
}

// deviceCount is one device's disconnects since a time; it starts again a day after it began.
type deviceCount struct {
	since time.Time
	n     int
}

// RouterSnapshot is what the nightly report reads: the bursts of the last day and each device's disconnects.
type RouterSnapshot struct {
	SignInBursts int
	DeauthBursts int
	Devices      map[string]int
}

type routerBurst string

const (
	signInBurst routerBurst = "sign_in"
	deauthBurst routerBurst = "deauth"
)

func NewRouterWatch(deps RouterWatchDeps) *RouterWatch {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &RouterWatch{deps: deps, pending: map[routerBurst]bool{}, told: map[routerBurst]time.Time{}, bursts: map[routerBurst][]time.Time{}, devices: map[string]deviceCount{}}
}

// Add counts one line heard at the given time. It never blocks and sends nothing itself.
func (w *RouterWatch) Add(l routerlog.Watched, at time.Time) {
	if !w.deps.Enabled() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	switch l.Kind {
	case routerlog.SignInFailed:
		w.fails = w.note(w.fails, at, routerSignInFailures, routerSignInWindow, signInBurst)
	case routerlog.Deauth:
		w.countDevice(l.MAC, at)
		w.deauths = w.note(w.deauths, at, routerDeauthBurst, routerDeauthWindow, deauthBurst)
	}
}

// note adds a time to a list that holds only the last n, and marks the burst when n lines fell within the window. The list is
// emptied then, so the next burst takes n new lines.
func (w *RouterWatch) note(times []time.Time, at time.Time, n int, window time.Duration, b routerBurst) []time.Time {
	times = append(times, at)
	if len(times) > n {
		times = times[len(times)-n:]
	}
	if len(times) == n && at.Sub(times[0]) <= window {
		w.pending[b] = true
		w.bursts[b] = append(w.bursts[b], at)
		if len(w.bursts[b]) > routerMaxBursts {
			w.bursts[b] = w.bursts[b][1:]
		}
		return nil
	}
	return times
}

// countDevice adds one disconnect to a device's count. A device beyond the table's size is not counted.
func (w *RouterWatch) countDevice(mac string, at time.Time) {
	if mac == "" {
		return
	}
	c, seen := w.devices[mac]
	if !seen && len(w.devices) >= routerMaxDevices {
		return
	}
	if !seen || at.Sub(c.since) > routerDay {
		c = deviceCount{since: at}
	}
	c.n++
	w.devices[mac] = c
}

// Snapshot is what the last day holds. It is empty while the watch is switched off.
func (w *RouterWatch) Snapshot(now time.Time) RouterSnapshot {
	out := RouterSnapshot{Devices: map[string]int{}}
	if !w.deps.Enabled() {
		return out
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	recent := func(b routerBurst) int {
		n := 0
		for _, at := range w.bursts[b] {
			if now.Sub(at) <= routerDay {
				n++
			}
		}
		return n
	}
	out.SignInBursts, out.DeauthBursts = recent(signInBurst), recent(deauthBurst)
	for mac, c := range w.devices {
		if now.Sub(c.since) <= routerDay {
			out.Devices[mac] = c.n
		}
	}
	return out
}

// Run looks every few seconds until ctx ends.
func (w *RouterWatch) Run(ctx context.Context) {
	tick := time.NewTicker(routerWatchEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.Tick(ctx)
		}
	}
}

// Tick tells each burst that is waiting, unless one of its kind was told within the gap. A burst is marked told only after the
// message was delivered.
func (w *RouterWatch) Tick(ctx context.Context) {
	now := w.deps.Now()
	for _, b := range []routerBurst{signInBurst, deauthBurst} {
		if ctx.Err() != nil || !w.due(b, now) {
			continue
		}
		msg := routerSignInMessage()
		if b == deauthBurst {
			msg = routerDeauthMessage()
		}
		if err := w.deps.Notify(ctx, msg); err != nil {
			if w.deps.OnError != nil {
				w.deps.OnError(fmt.Errorf("failed to send the router log alert: %w", err))
			}
			continue
		}
		w.mu.Lock()
		w.told[b], w.pending[b] = now, false
		w.mu.Unlock()
	}
}

func (w *RouterWatch) due(b routerBurst, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.pending[b] {
		return false
	}
	if !w.deps.Enabled() { // switched off since: forget it
		w.pending[b] = false
		return false
	}
	at, ok := w.told[b]
	return !ok || now.Sub(at) >= routerAlertGap
}

func routerSignInMessage() string {
	return fmt.Sprintf("The router's log shows %d failed sign-ins to its web page within %d minutes. If that was not you, someone on the network is guessing its password: change it and check which device is trying.", routerSignInFailures, int(routerSignInWindow.Minutes()))
}

func routerDeauthMessage() string {
	return fmt.Sprintf("The router's log shows %d or more Wi-Fi disconnects within a minute, far more than ordinary use. A device may be stuck in a loop, or someone nearby may be sending forged disconnect frames to knock devices off the Wi-Fi.", routerDeauthBurst)
}
