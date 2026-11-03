package nearby

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/names"
	"llm-proxy/internal/core/netaudit/upnp"
	"llm-proxy/internal/platform/mcast"
	"llm-proxy/models"
)

// Outcomes of a look: fixed phrases, never text a device sent.
const (
	OutcomeHeard        = "heard"
	OutcomeNothingHeard = "nothing_heard"
	OutcomeOff          = "off"
	OutcomeLANOff       = "lan_off"
	OutcomeNoInterface  = "no_interface"
	OutcomeRefused      = "refused"
	OutcomeUnavailable  = "unavailable"
)

const (
	// MinGap is the shortest time between two looks; one runs at a time. Both are checked here, on the server.
	MinGap = 30 * time.Second
	// MaxDescriptions bounds the description pages fetched in one look.
	MaxDescriptions = 32
	// Keep is how long a device unheard is kept in the list; MaxDevices bounds the list.
	Keep       = 30 * 24 * time.Hour
	MaxDevices = 256
	window     = 4 * time.Second
)

var (
	// ErrOff means Nearby is switched off.
	ErrOff = errors.New("Nearby is off")
	// ErrBusy means a look is already running.
	ErrBusy = errors.New("a look is already running")
	// ErrTooSoon means the last look started less than MinGap ago.
	ErrTooSoon = errors.New("the last look was moments ago")
)

// Store keeps the result. A missing file is an empty state.
type Store interface {
	Read() (models.NearbyState, error)
	Write(models.NearbyState) error
}

// Deps are what the collector needs; the application wires them in one place.
type Deps struct {
	Enabled func() bool
	// LANAllowed is the Guardrails local-network switch: with it off nothing is sent.
	LANAllowed func() bool
	Interfaces func() ([]mcast.Interface, error)
	Exchange   func(ctx context.Context, ifc mcast.Interface, qs []mcast.Question, lim mcast.Limits, accept func(netip.Addr) bool) ([]mcast.Datagram, error)
	// Accept is where the scan scope, this computer's own addresses and the blocked list are applied to a sender.
	Accept   func(netip.Addr) bool
	Identify func(ctx context.Context, location string, replier netip.Addr) (upnp.Identity, error)
	Store    Store
	Now      func() time.Time
	OnError  func(error)
}

// Collector runs the looks and keeps what they heard.
type Collector struct {
	deps Deps

	mu      sync.Mutex
	running bool
	started time.Time
	trigger chan struct{}
}

func New(deps Deps) *Collector {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Collector{deps: deps, trigger: make(chan struct{}, 1)}
}

// Run does the looks that Request asks for, until ctx ends. It is the one goroutine of the collector and is started with the
// application (Constitution II.2, II.14).
func (c *Collector) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.trigger:
			c.execute(ctx)
			c.end()
		}
	}
}

// Request asks Run for a look and returns at once. It refuses, here on the server, when Nearby is off, a look is running, or the
// last one started less than MinGap ago.
func (c *Collector) Request() error {
	if !c.deps.Enabled() {
		return ErrOff
	}
	if err := c.begin(); err != nil {
		return err
	}
	c.trigger <- struct{}{}
	return nil
}

// Running reports whether a look is in progress.
func (c *Collector) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// State is what is kept so far.
func (c *Collector) State() (models.NearbyState, error) { return c.deps.Store.Read() }

// execute makes one look and keeps what it heard, and returns the outcome phrase: sends the two questions on each eligible
// interface, reads the replies, fetches the description pages the SSDP replies point to, and merges what was heard into the kept
// state. Silence is an outcome, never an error and never "nothing is there". A look that ctx cancelled keeps nothing.
func (c *Collector) execute(ctx context.Context) string {
	res := &lookResult{heard: map[netip.Addr]*models.NearbyDevice{}, identified: map[netip.Addr]bool{}}
	outcome := c.run(ctx, res)
	if ctx.Err() == nil {
		c.merge(outcome, res)
	}
	return outcome
}

func (c *Collector) begin() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.running:
		return ErrBusy
	case !c.started.IsZero() && c.deps.Now().Sub(c.started) < MinGap:
		return ErrTooSoon
	}
	c.running, c.started = true, c.deps.Now()
	return nil
}

func (c *Collector) end() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
}

// lookResult is what one look heard: the devices, and which of them had their description page read (so that what the page says,
// including "not a gateway", replaces what was kept).
type lookResult struct {
	heard      map[netip.Addr]*models.NearbyDevice
	identified map[netip.Addr]bool
}

func (c *Collector) run(ctx context.Context, res *lookResult) string {
	if !c.deps.LANAllowed() {
		return OutcomeLANOff
	}
	ifaces, err := c.deps.Interfaces()
	if err != nil || len(ifaces) == 0 {
		return OutcomeNoInterface
	}
	locations := map[netip.Addr]string{}
	qs := []mcast.Question{{To: MDNSGroup, Payload: MDNSQuestion}, {To: SSDPGroup, Payload: SSDPSearch}}
	for _, ifc := range ifaces {
		// A reply from the address we send from is this computer's own responder answering, never another device.
		accept := func(a netip.Addr) bool { return a != ifc.Addr && c.deps.Accept(a) }
		replies, err := c.deps.Exchange(ctx, ifc, qs, mcast.Limits{Window: window}, accept)
		if err != nil {
			return refusalOutcome(err)
		}
		c.absorb(replies, res.heard, locations)
	}
	c.identify(ctx, res, locations)
	if len(res.heard) == 0 {
		return OutcomeNothingHeard
	}
	return OutcomeHeard
}

func refusalOutcome(err error) string {
	if errors.Is(err, mcast.ErrUnavailable) {
		return OutcomeUnavailable
	}
	return OutcomeRefused
}

// absorb parses each reply by what it looks like: an HTTP answer is SSDP, anything else is tried as mDNS.
func (c *Collector) absorb(replies []mcast.Datagram, heard map[netip.Addr]*models.NearbyDevice, locations map[netip.Addr]string) {
	for _, d := range replies {
		addr := d.From.Addr()
		if strings.HasPrefix(string(d.Data[:min(len(d.Data), 5)]), "HTTP/") {
			if loc, err := upnp.ParseSSDP(d.Data, addr); err == nil {
				device(heard, addr, c.deps.Now()).Sources = addSource(device(heard, addr, c.deps.Now()).Sources, "ssdp")
				locations[addr] = loc
			}
			continue
		}
		if f, err := ParseMDNS(d.Data); err == nil {
			dev := device(heard, addr, c.deps.Now())
			dev.Sources = addSource(dev.Sources, "mdns")
			dev.Services = mergeCapped(dev.Services, f.Services, maxServices)
			dev.Names = mergeCapped(dev.Names, f.Names, maxNames)
			dev.Ports = mergeCapped(dev.Ports, f.Ports, maxPorts)
		}
	}
}

func device(heard map[netip.Addr]*models.NearbyDevice, addr netip.Addr, now time.Time) *models.NearbyDevice {
	if d, ok := heard[addr]; ok {
		return d
	}
	d := &models.NearbyDevice{IP: addr.String(), FirstHeard: now, LastHeard: now}
	heard[addr] = d
	return d
}

func addSource(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

func mergeCapped[T comparable](have, add []T, limit int) []T {
	for _, v := range add {
		if len(have) < limit && !slices.Contains(have, v) {
			have = append(have, v)
		}
	}
	return have
}

// identify reads at most MaxDescriptions description pages, each from the address that said where it was.
func (c *Collector) identify(ctx context.Context, res *lookResult, locations map[netip.Addr]string) {
	addrs := make([]netip.Addr, 0, len(locations))
	for a := range locations {
		addrs = append(addrs, a)
	}
	slices.SortFunc(addrs, netip.Addr.Compare)
	for _, a := range addrs[:min(len(addrs), MaxDescriptions)] {
		if ctx.Err() != nil {
			return
		}
		id, err := c.deps.Identify(ctx, locations[a], a)
		if err != nil {
			continue
		}
		res.identified[a] = true
		d := res.heard[a]
		d.UPnPType = upnpType(id.DeviceType)
		d.Gateway = id.Gateway
		d.Maker = cleanText(id.Manufacturer)
		d.Model = cleanText(strings.TrimSpace(id.ModelName + " " + id.ModelNumber))
		if len(d.Names) == 0 {
			d.Names = mergeCapped(d.Names, []string{cleanText(id.FriendlyName)}, maxNames)
		}
	}
}

func cleanText(s string) string {
	return names.Clean(s)
}

// upnpType keeps the end of a device type URN ("InternetGatewayDevice:1") when it has the shape of one.
func upnpType(urn string) string {
	const prefix = "urn:schemas-upnp-org:device:"
	rest, ok := strings.CutPrefix(urn, prefix)
	if !ok || len(rest) > 48 || strings.Trim(rest, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789:") != "" {
		return ""
	}
	return rest
}

// merge adds what was heard to the kept state and writes it. A state that cannot be read is left alone and reported. A device
// that did not answer this time stays, with its old facts and its old time: silence is not absence. A page that was read
// replaces the old UPnP facts, so a device that stopped claiming to be a gateway stops being one.
func (c *Collector) merge(outcome string, res *lookResult) {
	state, err := c.deps.Store.Read()
	if err != nil {
		c.report(err)
		return
	}
	now := c.deps.Now()
	index := map[string]int{}
	for i, d := range state.Devices {
		index[d.IP] = i
	}
	for addr, h := range res.heard {
		i, known := index[h.IP]
		if !known {
			state.Devices = append(state.Devices, *h)
			continue
		}
		d := &state.Devices[i]
		d.LastHeard = now
		d.Services = mergeCapped(d.Services, h.Services, maxServices)
		d.Names = mergeCapped(d.Names, h.Names, maxNames)
		d.Ports = mergeCapped(d.Ports, h.Ports, maxPorts)
		for _, s := range h.Sources {
			d.Sources = addSource(d.Sources, s)
		}
		if res.identified[addr] {
			d.UPnPType, d.Maker, d.Model, d.Gateway = h.UPnPType, h.Maker, h.Model, h.Gateway
		}
	}
	state.Devices = trim(state.Devices, now)
	state.LastLook, state.LastOutcome = now, outcome
	if err := c.deps.Store.Write(state); err != nil {
		c.report(err)
	}
}

// trim drops devices unheard for Keep and the oldest beyond MaxDevices.
func trim(in []models.NearbyDevice, now time.Time) []models.NearbyDevice {
	out := in[:0]
	for _, d := range in {
		if now.Sub(d.LastHeard) <= Keep {
			out = append(out, d)
		}
	}
	slices.SortStableFunc(out, func(a, b models.NearbyDevice) int { return b.LastHeard.Compare(a.LastHeard) })
	return out[:min(len(out), MaxDevices)]
}

func (c *Collector) report(err error) {
	if c.deps.OnError != nil {
		c.deps.OnError(err)
	}
}
