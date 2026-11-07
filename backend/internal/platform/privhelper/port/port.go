// Package port is the core's implementation of the SDK's Privileged port (ADR-0034): the only way a stack reaches the privileged helper.
// A stack passes back the grant the core minted for its route; this adapter checks the grant against the action and the checked
// parameters, uses it up, and only then calls the helper's client. It owns no authority beyond that check: the helper decides who is
// calling. Only the composition root imports it.
package port

import (
	"context"
	"encoding/json"
	"errors"

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
)

// Grants uses up a grant. grant.Store satisfies it.
type Grants interface {
	Consume(token, action string, canonical []byte) error
}

// Helper runs an action in the helper. client.Client satisfies it.
type Helper interface {
	Call(ctx context.Context, action string, params json.RawMessage) (wire.Result, error)
}

// Port implements stacksdk.Privileged.
type Port struct {
	grants Grants
	helper Helper
}

// New returns a Port that checks grants with g and calls h.
func New(g Grants, h Helper) *Port {
	return &Port{grants: g, helper: h}
}

// Run checks the request and runs it. The parameters are checked against the catalogue first, because a grant is bound to their
// canonical form; the grant is used up before the helper is called, so one click is one attempt.
func (p *Port) Run(ctx context.Context, req stacksdk.PrivilegedRequest) (stacksdk.PrivilegedResult, error) {
	a, err := catalog.Lookup(req.Action)
	if err != nil {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeInvalid, "no such action")
	}
	prepared, err := a.Prepare(req.Params)
	if err != nil {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(parameterCode(err), "the parameters are refused")
	}
	if err := p.grants.Consume(req.Grant, a.ID, prepared.Canonical); err != nil {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeDenied, msgGrant)
	}
	res, err := p.helper.Call(ctx, a.ID, req.Params)
	if err != nil {
		return stacksdk.PrivilegedResult{}, helperError(err)
	}
	return stacksdk.PrivilegedResult{Kind: res.Kind, Data: res.Data, Truncated: res.Truncated}, nil
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
