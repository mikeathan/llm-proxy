package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sync"
	"time"

	outside "llm-proxy/internal/core/netaudit/outsidelookup"
)

// CheckInternetExposedPort is a port an outside scan database lists as open on the network's public address. Anyone on the
// internet can try to connect to it. The finding is on the router, which is where the internet reaches the network.
const CheckInternetExposedPort = "internet_exposed_port"

// Outcomes of the last look, and the reasons it could not be made. The words for them are the screen's; the codes are stable.
const (
	OutcomeLooked     = "looked"
	OutcomeCannotLook = "cannot_look"
	OutcomeFailed     = "failed"

	ReasonRouterAsk    = "router_not_asked"
	ReasonUPnPOff      = "upnp_off"
	ReasonNoAddress    = "no_address"
	ReasonPrivate      = "private_address"
	ReasonShared       = "shared_address"
	ReasonLinkLocal    = "link_local_address"
	ReasonLookupFailed = "lookup_failed"
	ReasonInternetOff  = "internet_off"

	// SourceRouter and SourceEntered say where the address of a look came from.
	SourceRouter  = "router"
	SourceEntered = "entered"
)

// exposureReuse is how long a look that was made is reused for the same address. The outside service refreshes about weekly, and
// the router check also runs for a rescan of one device, so asking again within hours would only send the address out again.
const exposureReuse = 6 * time.Hour

// OutsideLookup asks an outside scan database for the open ports of a public address: *outside.Client.
type OutsideLookup interface {
	OpenPorts(ctx context.Context, addr netip.Addr) ([]int, error)
}

// ExposureStatus is what the last look did, kept for the screen. It is in memory only: after a restart it is empty until the
// next look.
type ExposureStatus struct {
	At      time.Time `json:"at"`
	Outcome string    `json:"outcome,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	// Ports is how many open ports the service listed on a look that was made.
	Ports int `json:"ports"`
	// Source is where the address came from on a look that was tried: the router's own statement, or the one the operator entered.
	Source string `json:"source,omitempty"`
	// Differs is set when the router states a public address other than the one entered, so the entered one may be out of date.
	Differs bool `json:"differs,omitempty"`
}

// Exposure is the opt-in look at what the internet can reach. It reads the router's own statement of its internet address
// (UPnP), and only when the operator has switched it on and that address is a public one does it send that address, once, to
// the outside service.
type Exposure struct {
	enabled func() bool
	lookup  OutsideLookup
	// entered is the address the operator typed in; nil when none can be.
	entered func() string
	now     func() time.Time
	mu      sync.Mutex
	status  ExposureStatus
	// last is the answer of the look that was made, reused for exposureReuse.
	last struct {
		addr  netip.Addr
		ports []int
		at    time.Time
	}
}

// NewExposure builds the check. enabled is read on every look, so the switch takes effect at once.
func NewExposure(enabled func() bool, lookup OutsideLookup, now func() time.Time) *Exposure {
	return &Exposure{enabled: enabled, lookup: lookup, now: now}
}

// WithEnteredAddress gives the check the address the operator typed in, used only when the router states no public address.
func (e *Exposure) WithEnteredAddress(entered func() string) *Exposure {
	e.entered = entered
	return e
}

// Status is what the last look did.
func (e *Exposure) Status() ExposureStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// record keeps what a look did, stamped now unless st carries the time of the look it reuses.
func (e *Exposure) record(st ExposureStatus) {
	if st.At.IsZero() {
		st.At = e.now()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = st
}

// Wants says whether the look is switched on, and so whether the router should be asked for its internet address in the probe it is given.
func (e *Exposure) Wants() bool { return e.enabled() }

// Begin marks a new attempt as not yet looked. The router check calls it first on every run, so that whatever stops the run
// before the look (no router address, a router outside the allowed networks, a refused probe) leaves the status saying the
// look was not made, and the earlier findings are carried (Carry) instead of being reported as fixed. Only a look that is made,
// or an earlier one reused, sets it to looked.
func (e *Exposure) Begin() {
	if e.enabled() {
		e.record(ExposureStatus{Outcome: OutcomeCannotLook, Reason: ReasonRouterAsk})
	}
}

// Findings makes the look for the router at gw, using what its UPnP report said. Nothing is sent unless the switch is on and
// the router stated a public address. A look that could not be made gives no findings and is recorded as such; it is never
// read as "nothing is open".
func (e *Exposure) Findings(ctx context.Context, gw netip.Addr, report RouterReport) []Finding {
	if !e.enabled() {
		return nil
	}
	pick, reason := e.choose(report)
	if reason != "" {
		e.record(ExposureStatus{Outcome: OutcomeCannotLook, Reason: reason})
		return nil
	}
	made := ExposureStatus{Outcome: OutcomeLooked, Source: pick.source, Differs: pick.differs}
	ports, at, reused := e.reusable(pick.addr)
	if reused {
		made.At, made.Ports = at, len(ports)
		e.record(made)
	} else {
		var err error
		if ports, err = e.lookup.OpenPorts(ctx, pick.addr); err != nil {
			made.Outcome, made.Reason = OutcomeFailed, failureReason(err)
			e.record(made)
			return nil
		}
		made.Ports = len(ports)
		e.record(made)
		e.remember(pick.addr, ports)
	}
	out := make([]Finding, 0, len(ports))
	for _, p := range ports {
		out = append(out, exposedPortFinding(gw, p, report.Mappings))
	}
	return out
}

// failureReason says why a lookup failed: the network settings forbid it, or the service could not be asked.
func failureReason(err error) string {
	if errors.Is(err, outside.ErrInternetOff) {
		return ReasonInternetOff
	}
	return ReasonLookupFailed
}

// reusable is the answer of the last look that was made, if it was for this address and recent.
func (e *Exposure) reusable(addr netip.Addr) ([]int, time.Time, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.last.addr != addr || e.now().Sub(e.last.at) >= exposureReuse {
		return nil, time.Time{}, false
	}
	return e.last.ports, e.last.at, true
}

func (e *Exposure) remember(addr netip.Addr, ports []int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.last.addr, e.last.ports, e.last.at = addr, ports, e.now()
}

// address is the one a look is made for and where it came from. differs is set when the router's address is not the entered one.
type address struct {
	addr    netip.Addr
	source  string
	differs bool
}

// choose picks the address to look up. The router's own public statement wins; the entered address is used only when the router
// gives no public one (UPnP off, or an address behind another router or a provider's shared one). Without either, the reason
// the router could not help is returned.
func (e *Exposure) choose(report RouterReport) (address, string) {
	fromRouter, reason := e.routerAddress(report)
	entered := e.enteredAddress()
	switch {
	case reason == "":
		return address{addr: fromRouter, source: SourceRouter, differs: entered.IsValid() && entered != fromRouter}, ""
	case entered.IsValid():
		return address{addr: entered, source: SourceEntered}, ""
	}
	return address{}, reason
}

// enteredAddress is the address the operator typed, if there is one and it is public: a stored value that no longer is one is
// ignored, so nothing but a public address is ever sent.
func (e *Exposure) enteredAddress() netip.Addr {
	if e.entered == nil {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(e.entered())
	if err != nil || outside.Classify(addr) != outside.Public {
		return netip.Addr{}
	}
	return addr
}

// routerAddress is the router's internet address, as the router sensor read it when asked, if it is one the outside world can reach, or the
// reason it is not.
func (e *Exposure) routerAddress(report RouterReport) (netip.Addr, string) {
	if !report.Answered || !report.Gateway {
		return netip.Addr{}, ReasonUPnPOff
	}
	addr := report.ExternalAddr
	if !report.HasService || !addr.IsValid() {
		return netip.Addr{}, ReasonNoAddress
	}
	switch outside.Classify(addr) {
	case outside.Public:
		return addr, ""
	case outside.Private:
		return netip.Addr{}, ReasonPrivate
	case outside.Shared:
		return netip.Addr{}, ReasonShared
	case outside.LinkLocal:
		return netip.Addr{}, ReasonLinkLocal
	}
	return netip.Addr{}, ReasonNoAddress
}

// exposedPortFinding states what was seen and whether a UPnP mapping on the router accounts for it. The wording is fixed: it
// holds a port number and nothing the service or the router said, and not the public address. It never names a cause.
func exposedPortFinding(gw netip.Addr, port int, maps []RouterMapping) Finding {
	f := Finding{Device: gw.String(), Port: port, Check: CheckInternetExposedPort, Severity: SeverityMedium}
	if slices.Contains(sensitiveForwards, port) {
		f.Severity = SeverityHigh
	}
	if forwardedByUPnP(port, maps) {
		f.Evidence = fmt.Sprintf("An outside scan database lists port %d as open on this network's public address. Your router forwards this port because something asked it to through UPnP. Anyone on the internet can try to connect to it. The database's information can be up to a week old", port)
	} else {
		f.Evidence = fmt.Sprintf("An outside scan database lists port %d as open on this network's public address. No UPnP mapping on your router accounts for it: check the router's own port-forwarding list and any device in front of the router. Anyone on the internet can try to connect to it. The database's information can be up to a week old", port)
	}
	return f
}

func forwardedByUPnP(port int, maps []RouterMapping) bool {
	return slices.ContainsFunc(maps, func(m RouterMapping) bool { return m.Enabled && m.ExternalPort == port })
}

// Carry returns the earlier findings of this check when the last look was tried and could not be made, so that a night the
// service could not be reached, or UPnP was off, does not report every exposed port as fixed. It carries nothing when the
// switch is off, when no look has been tried, or when the last look was made (its result replaces the earlier one).
func (e *Exposure) Carry(earlier []Finding) []Finding {
	if !e.enabled() {
		return nil
	}
	if s := e.Status(); s.Outcome != OutcomeCannotLook && s.Outcome != OutcomeFailed {
		return nil
	}
	var out []Finding
	for _, f := range earlier {
		if f.Check == CheckInternetExposedPort {
			out = append(out, f)
		}
	}
	return out
}
