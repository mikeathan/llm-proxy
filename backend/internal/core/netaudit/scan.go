package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"llm-proxy/internal/platform/scanscope"
)

// Typed refusals; match them with errors.Is.
var (
	// ErrBadMode means the requested mode does not exist.
	ErrBadMode = errors.New("unknown scan mode")
	// ErrScanBusy means another scan is already running; scans run one at a time.
	ErrScanBusy = errors.New("another scan is already running")
	// ErrNmapRequired means the request needs nmap and it cannot be used: the mode needs it, or the run is
	// unattended and a limited engine must never stand in silently.
	ErrNmapRequired = errors.New("nmap is required for this scan")
)

// Request is a scan as the tool receives it: raw targets the policy has not yet seen.
type Request struct {
	Targets []string
	Mode    Mode
	// Ports is an optional list such as "22,80,8000-8100"; empty means the engine's default set.
	Ports string
	// Thorough sweeps on more ports and then scans more ports of the live addresses, more gently; it takes longer. The
	// default is the quick scan.
	Thorough bool
	// Unattended marks a scheduled run, where no person can read a warning: it never falls back to the limited engine.
	Unattended bool
}

// ScanRequest is a scan after authorisation: addresses only, and a vetted port list.
type ScanRequest struct {
	Targets []netip.Addr
	Mode    Mode
	Ports   string
	// Thorough is Request.Thorough.
	Thorough bool
	// Prefiltered means the service already swept the targets and they all answered, so the engine need not check
	// which addresses are alive before looking at them.
	Prefiltered bool
	// HostTimeout is how long the engine may spend on one device; zero means the depth's standard. HostTimeoutFactor
	// multiplies whichever applies (a retry of a device that ran out of time uses 2); zero or one changes nothing.
	HostTimeout       time.Duration
	HostTimeoutFactor int
	// Timeout bounds the whole engine run; zero means the engine's own limit.
	Timeout time.Duration
}

// Result is what a scan found.
type Result struct {
	// Engine names the engine that did the work, so a limited result is never mistaken for an nmap one.
	Engine   string    `json:"engine"`
	Hosts    []Host    `json:"hosts"`
	Findings []Finding `json:"findings"`
	// Warnings are for the reader of the report, such as "limited engine: no service detection".
	Warnings []string `json:"warnings,omitempty"`
}

// Engine does the scanning. Implementations are the nmap engine and the limited Go prober.
type Engine interface {
	// Name labels results.
	Name() string
	// Available reports why the engine cannot be used right now; nil means it can.
	Available(ctx context.Context) error
	// Supports reports whether the engine can do a mode.
	Supports(Mode) bool
	// Scan runs the scan. The request is already authorised.
	Scan(ctx context.Context, req ScanRequest) (Result, error)
}

// Authorizer turns requested targets into the addresses a scan may touch.
type Authorizer interface {
	Authorize(ctx context.Context, targets []string) ([]netip.Addr, error)
}

// PolicyAuthorizer adapts a scan scope and a resolver to Authorizer. The policy is read for every request so a
// change of scope applies to the next scan.
type PolicyAuthorizer struct {
	Policy   func() scanscope.Policy
	Resolver scanscope.Resolver
}

// Authorize authorises the targets against the current policy.
func (a PolicyAuthorizer) Authorize(ctx context.Context, targets []string) ([]netip.Addr, error) {
	return a.Policy().Authorize(ctx, a.Resolver, targets)
}

// Service runs scans: it authorises the targets, picks the engine and keeps scans to one at a time.
type Service struct {
	authorizer Authorizer
	primary    Engine // nmap
	fallback   Engine // the limited Go prober; may be nil

	// deviceTimeout is the operator's per-device time limit (zero = each depth's standard); nil means standard.
	deviceTimeout func() time.Duration
	now           func() time.Time

	running sync.Mutex
}

// scanBudget is how long one scan may take in all, the retry included. An automation run is capped at 10 minutes and
// still has to write its report, so the scan stops well before that.
const scanBudget = 8 * time.Minute

// retryMinRemaining is the least time that must be left in the budget for a device that ran out of time to be tried
// again; a shorter retry would only time out too.
const retryMinRemaining = 90 * time.Second

// retryFactor is how many times longer a retry lets a slow device take.
const retryFactor = 2

// NewService builds a Service. fallback may be nil, which means a host without nmap cannot scan.
func NewService(authorizer Authorizer, primary, fallback Engine) *Service {
	return &Service{authorizer: authorizer, primary: primary, fallback: fallback, now: time.Now}
}

// WithDeviceTimeout makes scans use the operator's per-device time limit, read for every scan so a change applies to
// the next one.
func (s *Service) WithDeviceTimeout(limit func() time.Duration) *Service {
	s.deviceTimeout = limit
	return s
}

// Scan authorises and runs one scan. It refuses before touching the network when the mode or ports are invalid,
// scanning is off, a target is out of scope, another scan is running, or no suitable engine is available.
//
// Every scan sweeps the targets first with the limited engine, which finds the live addresses of a /24 in seconds, and
// gives only those to the engine that does the real work; discover is the sweep alone. A thorough scan sweeps on more
// ports (so a device that answers on one service is still found) and then looks harder at what it found: more ports,
// gentler timing. Probing every dead address instead took ten minutes for a /24 and marked every address up.
func (s *Service) Scan(ctx context.Context, req Request) (Result, error) {
	scan, err := s.prepare(ctx, req)
	if err != nil {
		return Result{}, err
	}
	started := s.now()
	if s.deviceTimeout != nil {
		scan.HostTimeout = s.deviceTimeout()
	}
	if !s.running.TryLock() {
		return Result{}, ErrScanBusy
	}
	defer s.running.Unlock()

	// Refuse before the network is touched when the engine this scan needs is not there. A discover with a sweeper is the
	// sweep alone and needs no engine of its own.
	engine, warnings, err := s.engineFor(ctx, req)
	if err != nil {
		return Result{}, err
	}
	swept, err := s.sweepFirst(ctx, req, scan)
	if err != nil {
		return Result{}, err
	}
	if swept.final != nil {
		return finish(*swept.final), nil
	}

	res, err := engine.Scan(ctx, swept.scan)
	if err != nil {
		return Result{}, err
	}
	res.Engine = engine.Name()
	res.Warnings = append(warnings, res.Warnings...)
	if swept.sweep != nil {
		res.Hosts = mergeHosts(swept.sweep.Hosts, res.Hosts)
		res.Hosts = s.retryUnscanned(ctx, engine, swept.scan, res.Hosts, scanBudget-s.now().Sub(started))
		if n := countUnscanned(res.Hosts); n > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%d live address(es) were not scanned to completion (the scan ran out of time on them), "+
				"so their open ports are unknown, not none.", n))
		}
	}
	return finish(res), nil
}

// prepare validates the request and authorises its targets; nothing has touched the network yet.
func (s *Service) prepare(ctx context.Context, req Request) (ScanRequest, error) {
	if !req.Mode.Valid() {
		return ScanRequest{}, fmt.Errorf("%w: %q (use one of %v)", ErrBadMode, req.Mode, Modes())
	}
	ports, err := ValidatePorts(req.Ports)
	if err != nil {
		return ScanRequest{}, err
	}
	addrs, err := s.authorizer.Authorize(ctx, req.Targets)
	if err != nil {
		return ScanRequest{}, err
	}
	return ScanRequest{Targets: addrs, Mode: req.Mode, Ports: ports, Thorough: req.Thorough}, nil
}

// engineFor picks the engine that does the real work, or nil when a quick discover is the sweep alone.
func (s *Service) engineFor(ctx context.Context, req Request) (Engine, []string, error) {
	if req.Mode == ModeDiscover && s.sweeper() != nil {
		return nil, nil, nil
	}
	return s.pickEngine(ctx, req)
}

// sweepOutcome is what the quick sweep decided: the request to give the engine, the sweep's own result (to merge back
// in), or a final result when the sweep answered the whole request.
type sweepOutcome struct {
	scan  ScanRequest
	sweep *Result
	final *Result
}

// sweepFirst runs the sweep unless there is no sweeper. A discover scan is complete after the sweep, and so is any scan in
// which nothing answered.
func (s *Service) sweepFirst(ctx context.Context, req Request, scan ScanRequest) (sweepOutcome, error) {
	if s.sweeper() == nil {
		return sweepOutcome{scan: scan}, nil
	}
	sweep, err := s.sweeper().Scan(ctx, ScanRequest{Targets: scan.Targets, Mode: ModeDiscover, Thorough: req.Thorough})
	if err != nil {
		return sweepOutcome{}, err
	}
	sweep.Engine = s.sweeper().Name()
	live := liveAddrs(sweep.Hosts)
	if req.Mode == ModeDiscover || len(live) == 0 {
		return sweepOutcome{final: &sweep}, nil
	}
	scan.Targets, scan.Prefiltered = live, true
	return sweepOutcome{scan: scan, sweep: &sweep}, nil
}

// sweeper is the engine that finds live addresses quickly: the limited Go prober, when there is one.
func (s *Service) sweeper() Engine { return s.fallback }

// finish fills in the findings a result does not carry yet.
func finish(res Result) Result {
	if res.Findings == nil {
		res.Findings = FindingsOf(res.Hosts)
	}
	return res
}

// retryUnscanned gives the live devices the engine ran out of time on one more try, each allowed twice as long, when
// enough of the scan's budget is left. A device the retry finishes replaces its unscanned entry; the rest stay flagged.
// A failed retry changes nothing: the first pass's result stands.
func (s *Service) retryUnscanned(ctx context.Context, engine Engine, first ScanRequest, hosts []Host, remaining time.Duration) []Host {
	var slow []netip.Addr
	for _, h := range hosts {
		if h.Unscanned {
			slow = append(slow, h.Addr)
		}
	}
	if len(slow) == 0 || remaining < retryMinRemaining {
		return hosts
	}
	retry := first
	retry.Targets, retry.HostTimeoutFactor, retry.Timeout = slow, retryFactor, remaining
	again, err := engine.Scan(ctx, retry)
	if err != nil {
		return hosts
	}
	byAddr := make(map[netip.Addr]Host, len(again.Hosts))
	for _, h := range again.Hosts {
		byAddr[h.Addr] = h
	}
	out := make([]Host, len(hosts))
	for i, h := range hosts {
		if found, ok := byAddr[h.Addr]; ok && h.Unscanned && !found.Unscanned {
			found.Up = true
			h = found
		}
		out[i] = h
	}
	return out
}

func countUnscanned(hosts []Host) int {
	n := 0
	for _, h := range hosts {
		if h.Unscanned {
			n++
		}
	}
	return n
}

func liveAddrs(hosts []Host) []netip.Addr {
	var out []netip.Addr
	for _, h := range hosts {
		if h.Up {
			out = append(out, h.Addr)
		}
	}
	return out
}

// mergeHosts takes the sweep's list of every address and replaces the entries for the addresses the engine looked at
// with what the engine found, so addresses that did not answer are still reported, as down.
func mergeHosts(swept, scanned []Host) []Host {
	byAddr := make(map[netip.Addr]Host, len(scanned))
	for _, h := range scanned {
		byAddr[h.Addr] = h
	}
	out := make([]Host, 0, len(swept))
	for _, h := range swept {
		if detailed, ok := byAddr[h.Addr]; ok {
			h = detailed
			h.Up = true // the sweep saw it answer
		} else if h.Up {
			h.Unscanned = true // alive, but the engine never reported on it
		}
		out = append(out, h)
	}
	return out
}

// pickEngine prefers nmap. A mode that needs nmap, or an unattended run, never falls back; an attended run that can
// use the limited engine does, and says so in the result.
func (s *Service) pickEngine(ctx context.Context, req Request) (Engine, []string, error) {
	unavailable := s.primary.Available(ctx)
	if unavailable == nil && s.primary.Supports(req.Mode) {
		return s.primary, nil, nil
	}
	if unavailable == nil {
		unavailable = fmt.Errorf("%s does not support mode %s", s.primary.Name(), req.Mode)
	}
	if req.Mode.Spec().NeedsNmap || req.Unattended || s.fallback == nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNmapRequired, unavailable)
	}
	if err := s.fallback.Available(ctx); err != nil || !s.fallback.Supports(req.Mode) {
		return nil, nil, fmt.Errorf("%w: %v", ErrNmapRequired, unavailable)
	}
	return s.fallback, []string{"limited engine used (" + unavailable.Error() + "): results are less complete than an nmap scan"}, nil
}
