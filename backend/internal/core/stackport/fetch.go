// Package stackport holds the core's implementations of the SDK ports that wrap a core service. A stack calls the port with plain data
// and never sees the service behind it. Only the composition root imports this package.
package stackport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/tools"
)

// Fixed sentences a stack is told: never the guard's own wording, which can name a host or an address.
const (
	msgOff      = "network or internet access is switched off"
	msgDenied   = "the guard refused this address"
	msgTimeout  = "the request ran out of time"
	msgInvalid  = "the address is not an http or https URL"
	msgFailed   = "the request failed"
	msgNot200   = "the server did not answer with HTTP 200"
	maxBodyCeil = 1 << 20
)

// Opener makes the guarded GET: *tools.NetworkTools.
type Opener interface {
	OpenAccepting(ctx context.Context, target, accept string) (*http.Response, error)
}

// Fetch is the SDK's Fetch port over the guarded GET. It reads the body once, as the callers did before the port existed.
type Fetch struct {
	Opener Opener
}

// Get makes one guarded GET and returns at most the smaller of the request's MaxBytes and 1 MiB of the body.
func (f Fetch) Get(ctx context.Context, req stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return stacksdk.FetchResponse{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgInvalid)
	}
	resp, err := f.Opener.OpenAccepting(ctx, req.URL, req.Accept)
	if err != nil {
		return stacksdk.FetchResponse{}, refusal(ctx, err)
	}
	defer resp.Body.Close()
	limit := maxBodyCeil
	if req.MaxBytes > 0 && req.MaxBytes < limit {
		limit = req.MaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return stacksdk.FetchResponse{}, refusal(ctx, err)
	}
	if len(body) > limit {
		return stacksdk.FetchResponse{Body: body[:limit], Truncated: true}, nil
	}
	return stacksdk.FetchResponse{Body: body}, nil
}

// refusal turns the guard's and the transport's failures into the SDK's codes, with a fixed sentence.
func refusal(ctx context.Context, err error) error {
	var status *tools.StatusError
	switch {
	case errors.Is(err, tools.ErrNetworkDisabled), errors.Is(err, tools.ErrInternetBlocked):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, msgOff)
	case errors.Is(err, tools.ErrGuardRefused):
		return stacksdk.Errorf(stacksdk.CodeDenied, msgDenied)
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return stacksdk.Errorf(stacksdk.CodeTimeout, msgTimeout)
	case errors.As(err, &status):
		return stacksdk.Errorf(stacksdk.CodeInternal, msgNot200)
	}
	return stacksdk.Errorf(stacksdk.CodeInternal, msgFailed)
}
