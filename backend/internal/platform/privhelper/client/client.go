// Package client is the service's side of the privileged helper (design network-privileged-helper-design.md section 6): it connects to the
// helper's Unix socket, sends one catalogue action and returns the bytes. It is a plain request channel with no authority of its own: the
// helper decides who is calling (ADR-0034), and the grant that lets a request reach this client is checked before it does. Only the
// step-up route handler and the composition root import it (an architecture test pins that).
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/internal/platform/sandbox"
)

// callGrace is the time allowed beyond an action's own timeout for the helper to start, answer and clean up.
const callGrace = 10 * time.Second

// Typed failures; match them with errors.Is or errors.As.
var (
	// ErrUnavailable means the helper's socket is absent or not accepting: the unit is not installed or not running.
	ErrUnavailable = errors.New("the privileged helper is not available")
	// ErrUnsafeHost means the sandbox does not report file confinement and Unix-socket denial, so a child of the service could reach
	// the helper's socket itself; the service makes no call (design section 2, a safety interlock).
	ErrUnsafeHost = errors.New("the sandbox is not enforcing file confinement and Unix-socket denial")
	// ErrFailed means the action was allowed and failed.
	ErrFailed = errors.New("the privileged action failed")
	// ErrTooLarge means the helper answered with more than the action's output cap.
	ErrTooLarge = errors.New("the helper's answer is over the action's output cap")
)

// Refusal is a rule's "no" from the helper: a code and the helper's one fixed sentence, never a Go error from the privileged side.
type Refusal struct {
	Code    wire.Code
	Message string
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("the privileged helper refused (%s): %s", r.Code, r.Message)
}

// Sandbox is the part of the sandbox provider the interlock reads.
type Sandbox interface {
	Effective() sandbox.EffectiveState
}

// Client calls the helper at one socket path.
type Client struct {
	socket  string
	sandbox Sandbox
}

// New returns a Client for the helper's socket. The sandbox is read on every call, because it can change at runtime.
func New(socket string, sb Sandbox) *Client {
	return &Client{socket: socket, sandbox: sb}
}

// safeHost reports whether this service's own children are kept from Unix sockets, which is what makes the helper's socket
// unreachable by anything but the service.
func (c *Client) safeHost() error {
	if c.sandbox == nil {
		return ErrUnsafeHost
	}
	e := c.sandbox.Effective()
	if e.Filesystem.Mechanism == sandbox.EnforcementNone || e.UnixSockets.Mechanism == sandbox.EnforcementNone {
		return ErrUnsafeHost
	}
	return nil
}

// Ready reports whether a call would be allowed to start: nil when the sandbox enforces file confinement and Unix-socket denial,
// ErrUnsafeHost otherwise. It makes no connection.
func (c *Client) Ready() error { return c.safeHost() }

// Call runs one catalogue action. The parameters are checked locally against the same catalogue the helper uses, so a malformed request
// fails here without a connection; the helper checks them again from its own copy.
func (c *Client) Call(ctx context.Context, action string, params json.RawMessage) (wire.Result, error) {
	a, err := catalog.Lookup(action)
	if err != nil {
		return wire.Result{}, err
	}
	if _, err := a.Prepare(params); err != nil {
		return wire.Result{}, err
	}
	if err := c.safeHost(); err != nil {
		return wire.Result{}, err
	}
	id, err := wire.NewID()
	if err != nil {
		return wire.Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.Caps.Timeout+callGrace)
	defer cancel()
	resp, err := c.exchange(ctx, wire.Request{V: wire.Version, ID: id, Action: a.ID, Params: params})
	if err != nil {
		return wire.Result{}, err
	}
	return resultOf(a, resp)
}

func resultOf(a catalog.Action, resp wire.Response) (wire.Result, error) {
	switch resp.Status {
	case wire.StatusRefused:
		return wire.Result{}, &Refusal{Code: resp.Code, Message: resp.Message}
	case wire.StatusError:
		return wire.Result{}, fmt.Errorf("%w: %s", ErrFailed, resp.Message)
	}
	if resp.Result == nil || resp.Result.Kind != string(a.Result) {
		return wire.Result{}, fmt.Errorf("%w: the answer has no %s result", wire.ErrMalformed, a.Result)
	}
	if len(resp.Result.Data) > a.Caps.MaxOutputBytes {
		return wire.Result{}, ErrTooLarge
	}
	return *resp.Result, nil
}

// exchange sends one request on a new connection and reads its one response. The connection ends with the call; when ctx ends first the
// connection is closed, which is how the helper learns to cancel the run.
func (c *Client) exchange(ctx context.Context, req wire.Request) (wire.Response, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socket)
	if err != nil {
		return wire.Response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := wire.WriteFrame(conn, req, wire.MaxRequestBytes); err != nil {
		return wire.Response{}, c.callError(ctx, err)
	}
	resp, err := wire.ReadResponse(conn, req.ID)
	if err != nil {
		return wire.Response{}, c.callError(ctx, err)
	}
	return resp, nil
}

// callError reports the context's own error when the call ended because the context did, and the I/O error otherwise.
func (c *Client) callError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
