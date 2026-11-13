// Package port is the core's implementation of the SDK's Privileged port (ADR-0034): the only way a stack reaches the privileged helper.
// A stack passes back the grant the core minted for its route; this adapter checks the grant against the action and the checked
// parameters, checks the target against the operator's declared scan ranges (the helper holds no range of its own), counts the action
// against its hourly cap, uses the grant up, and only then calls the helper's client. The helper decides who is calling. Only the
// composition root imports it.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/wire"
)

// Fixed sentences: what a stack is told, never the helper's own text or an echo of its input.
const (
	msgGrant       = "the grant is not valid for this request"
	msgRefused     = "the privileged helper refused the request"
	msgUnavailable = "the privileged helper is not available"
	msgUnsafe      = "the sandbox is not enforcing file confinement and Unix-socket denial"
	msgFailed      = "the privileged action failed"
	msgTimeout     = "the privileged action ran out of time"
	msgOutside     = "the target is outside the scan ranges declared in the Network settings"
	msgNoRange     = "no scan range is declared: declare one in the Network settings first"
	msgRangeError  = "the scan ranges could not be checked"
	msgHourly      = "the hourly limit for this action is reached; try again later"
	msgSession     = "no open session covers this request"
	msgOff         = "this action is switched off"
)

// Typed answers from a Ranges check; match them with errors.Is.
var (
	// ErrNoRangeDeclared means the operator has declared no scan range, so nothing may be scanned.
	ErrNoRangeDeclared = errors.New("no scan range is declared")
	// ErrOutsideRange means the target is not inside a declared range.
	ErrOutsideRange = errors.New("the target is outside the declared scan ranges")
)

// hourlyWindow is the span MaxPerHour counts over.
const hourlyWindow = time.Hour

// Ranges says whether the operator's declared scan ranges allow one target (a canonical IPv4 address). It returns nil, ErrNoRangeDeclared,
// ErrOutsideRange, or another error when the answer cannot be had.
type Ranges interface {
	Check(target string) error
}

// Grants uses up a grant. grant.Store satisfies it.
type Grants interface {
	Consume(token, action string, canonical []byte) error
}

// Helper runs an action in the helper. client.Client satisfies it.
type Helper interface {
	Call(ctx context.Context, action string, params json.RawMessage) (wire.Result, error)
}

// Options are what a Port is built from. Ranges is required in effect: without it nothing may be scanned. Now is injected for the hourly
// limit and defaults to the wall clock.
type Options struct {
	Grants Grants
	Helper Helper
	Ranges Ranges
	Now    func() time.Time
}

// Port implements stacksdk.Privileged.
type Port struct {
	grants Grants
	helper Helper
	ranges Ranges
	limits *limiter
}

// New returns a Port.
func New(o Options) *Port {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Port{grants: o.Grants, helper: o.Helper, ranges: o.Ranges, limits: newLimiter(o.Now)}
}

// limiter counts calls per action over the last hour. It keeps at most the cap's worth of timestamps per action, so it cannot grow.
type limiter struct {
	mu    sync.Mutex
	now   func() time.Time
	calls map[string][]time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, calls: map[string][]time.Time{}}
}

// take records one call of the action and reports whether it is within max for the last hour.
func (l *limiter) take(action string, max int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	recent := l.calls[action][:0]
	for _, at := range l.calls[action] {
		if now.Sub(at) < hourlyWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) >= max {
		l.calls[action] = recent
		return false
	}
	l.calls[action] = append(recent, now)
	return true
}

// Run checks the request and runs it. The parameters are checked against the catalogue first, because a grant is bound to their
// canonical form; the grant is used up before the helper is called, so one click is one attempt.
func (p *Port) Run(ctx context.Context, req stacksdk.PrivilegedRequest) (stacksdk.PrivilegedResult, error) {
	return p.run(ctx, req, func(a catalog.Action, prepared catalog.Prepared) error {
		if err := p.grants.Consume(req.Grant, a.ID, prepared.Canonical); err != nil {
			return stacksdk.Errorf(stacksdk.CodeDenied, msgGrant)
		}
		return nil
	})
}

// authorize is what lets a request through once its parameters and target are checked: a grant used up, or a session use counted. It runs
// before the hourly limit and the helper, and its error is what the caller is told.
type authorize func(a catalog.Action, prepared catalog.Prepared) error

func (p *Port) run(ctx context.Context, req stacksdk.PrivilegedRequest, allow authorize) (stacksdk.PrivilegedResult, error) {
	a, err := catalog.Lookup(req.Action)
	if err != nil {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeInvalid, "no such action")
	}
	prepared, err := a.Prepare(req.Params)
	if err != nil {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(parameterCode(err), "the parameters are refused")
	}
	if err := p.checkRanges(prepared.Target); err != nil {
		return stacksdk.PrivilegedResult{}, err
	}
	if err := allow(a, prepared); err != nil {
		return stacksdk.PrivilegedResult{}, err
	}
	if !p.limits.take(a.ID, a.Caps.MaxPerHour) {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeUnavailable, msgHourly)
	}
	res, err := p.helper.Call(ctx, a.ID, req.Params)
	if err != nil {
		return stacksdk.PrivilegedResult{}, helperError(err)
	}
	return stacksdk.PrivilegedResult{Kind: res.Kind, Data: res.Data, Truncated: res.Truncated}, nil
}

// checkRanges holds a target to the operator's declared scan ranges. It runs before the grant is used, so a refused target costs nothing.
func (p *Port) checkRanges(target string) error {
	if target == "" {
		return nil
	}
	var err error = ErrNoRangeDeclared
	if p.ranges != nil {
		err = p.ranges.Check(target)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNoRangeDeclared):
		return stacksdk.Errorf(stacksdk.CodeDenied, msgNoRange)
	case errors.Is(err, ErrOutsideRange):
		return stacksdk.Errorf(stacksdk.CodeDenied, msgOutside)
	}
	return stacksdk.Errorf(stacksdk.CodeDenied, msgRangeError)
}

func parameterCode(err error) stacksdk.Code {
	if errors.Is(err, catalog.ErrScope) {
		return stacksdk.CodeDenied
	}
	return stacksdk.CodeInvalid
}

func helperError(err error) error {
	var refusal *client.Refusal
	switch {
	case errors.As(err, &refusal):
		return stacksdk.Errorf(stacksdk.CodeDenied, msgRefused)
	case errors.Is(err, client.ErrUnsafeHost):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, msgUnsafe)
	case errors.Is(err, client.ErrUnavailable):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, msgUnavailable)
	case errors.Is(err, context.DeadlineExceeded):
		return stacksdk.Errorf(stacksdk.CodeTimeout, msgTimeout)
	case errors.Is(err, context.Canceled):
		return err
	}
	return stacksdk.Errorf(stacksdk.CodeInternal, msgFailed)
}
