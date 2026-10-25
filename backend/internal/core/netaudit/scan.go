package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"

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
	// Unattended marks a scheduled run, where no person can read a warning: it never falls back to the limited engine.
	Unattended bool
}

// ScanRequest is a scan after authorisation: addresses only, and a vetted port list.
type ScanRequest struct {
	Targets []netip.Addr
	Mode    Mode
	Ports   string
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

	running sync.Mutex
}

// NewService builds a Service. fallback may be nil, which means a host without nmap cannot scan.
func NewService(authorizer Authorizer, primary, fallback Engine) *Service {
	return &Service{authorizer: authorizer, primary: primary, fallback: fallback}
}

// Scan authorises and runs one scan. It refuses before touching the network when the mode or ports are invalid,
// scanning is off, a target is out of scope, another scan is running, or no suitable engine is available.
func (s *Service) Scan(ctx context.Context, req Request) (Result, error) {
	if !req.Mode.Valid() {
		return Result{}, fmt.Errorf("%w: %q (use one of %v)", ErrBadMode, req.Mode, Modes())
	}
	ports, err := ValidatePorts(req.Ports)
	if err != nil {
		return Result{}, err
	}
	addrs, err := s.authorizer.Authorize(ctx, req.Targets)
	if err != nil {
		return Result{}, err
	}
	if !s.running.TryLock() {
		return Result{}, ErrScanBusy
	}
	defer s.running.Unlock()

	engine, warnings, err := s.pickEngine(ctx, req)
	if err != nil {
		return Result{}, err
	}
	res, err := engine.Scan(ctx, ScanRequest{Targets: addrs, Mode: req.Mode, Ports: ports})
	if err != nil {
		return Result{}, err
	}
	res.Engine = engine.Name()
	res.Warnings = append(warnings, res.Warnings...)
	if res.Findings == nil {
		res.Findings = FindingsOf(res.Hosts)
	}
	return res, nil
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
