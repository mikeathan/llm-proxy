package port

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/wire"
)

const params = `{"target":"192.168.50.10","ports":[161,53]}`

type recordingHelper struct {
	calls int
	err   error
}

func (h *recordingHelper) Call(context.Context, string, json.RawMessage) (wire.Result, error) {
	h.calls++
	return wire.Result{Kind: "bytes", Data: []byte("out"), Truncated: true}, h.err
}

// minted returns a store holding a grant for scan_udp with the canonical form of params, the way the route handler mints it.
func minted(t *testing.T) (*grant.Store, string) {
	t.Helper()
	s := grant.NewStore(grant.Options{})
	token, err := s.Mint("scan_udp", []byte(`{"target":"192.168.50.10","ports":[53,161]}`))
	if err != nil {
		t.Fatal(err)
	}
	return s, token
}

func TestAGrantedRequestRunsOnceAndReturnsTheBytes(t *testing.T) {
	s, token := minted(t)
	h := &recordingHelper{}
	p := New(s, h)
	req := stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)}
	res, err := p.Run(t.Context(), req)
	if err != nil || string(res.Data) != "out" || !res.Truncated || res.Kind != "bytes" {
		t.Fatalf("got %+v, %v", res, err)
	}
	if _, err := p.Run(t.Context(), req); stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 1 {
		t.Errorf("a second use must be denied without a call: %v, %d calls", err, h.calls)
	}
}

func TestARequestThatDoesNotMatchItsGrantNeverReachesTheHelper(t *testing.T) {
	cases := map[string]stacksdk.PrivilegedRequest{
		"no grant":        {Action: "scan_udp", Params: json.RawMessage(params)},
		"a made-up grant": {Grant: "00000000000000000000000000000000", Action: "scan_udp", Params: json.RawMessage(params)},
		"another action":  {Action: "os_detect", Params: json.RawMessage(`{"target":"192.168.50.10"}`)},
		"another target":  {Action: "scan_udp", Params: json.RawMessage(`{"target":"192.168.50.11","ports":[53,161]}`)},
		"other ports":     {Action: "scan_udp", Params: json.RawMessage(`{"target":"192.168.50.10","ports":[53]}`)},
	}
	for name, req := range cases {
		s, token := minted(t)
		if req.Grant == "" && name != "no grant" {
			req.Grant = token
		}
		h := &recordingHelper{}
		_, err := New(s, h).Run(t.Context(), req)
		if stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
			t.Errorf("%s: %v, %d calls", name, err, h.calls)
		}
	}
}

func TestParametersTheCatalogueRefusesAreRefusedBeforeTheGrantIsUsed(t *testing.T) {
	s, token := minted(t)
	h := &recordingHelper{}
	p := New(s, h)
	for name, c := range map[string]struct {
		action, params string
		want           stacksdk.Code
	}{
		"unknown action": {"rm_rf", `{}`, stacksdk.CodeInvalid},
		"out of scope":   {"scan_udp", `{"target":"8.8.8.8","ports":[53]}`, stacksdk.CodeDenied},
		"malformed":      {"scan_udp", `{"target":"192.168.50.10","ports":[53],"x":1}`, stacksdk.CodeInvalid},
	} {
		_, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: c.action, Params: json.RawMessage(c.params)})
		if stacksdk.CodeOf(err) != c.want || h.calls != 0 {
			t.Errorf("%s: %v, %d calls", name, err, h.calls)
		}
	}
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)}); err != nil {
		t.Errorf("the grant must still work after refused attempts that did not match it: %v", err)
	}
}

func TestHelperFailuresBecomeCodesWithFixedSentences(t *testing.T) {
	cases := map[string]struct {
		err  error
		want stacksdk.Code
	}{
		"refused":           {&client.Refusal{Code: wire.CodeCaller, Message: "secret detail"}, stacksdk.CodeDenied},
		"not installed":     {client.ErrUnavailable, stacksdk.CodeUnavailable},
		"unsafe host":       {client.ErrUnsafeHost, stacksdk.CodeUnavailable},
		"ran out of time":   {context.DeadlineExceeded, stacksdk.CodeTimeout},
		"the action failed": {client.ErrFailed, stacksdk.CodeInternal},
		"an answer too big": {client.ErrTooLarge, stacksdk.CodeInternal},
		"something else":    {errors.New("path /var/x leaked"), stacksdk.CodeInternal},
	}
	for name, c := range cases {
		s, token := minted(t)
		_, err := New(s, &recordingHelper{err: c.err}).Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)})
		var e *stacksdk.Error
		if !errors.As(err, &e) || e.Code != c.want {
			t.Errorf("%s: got %v", name, err)
			continue
		}
		if e.Message == "secret detail" || e.Message == "path /var/x leaked" {
			t.Errorf("%s: the helper's text reached the stack: %q", name, e.Message)
		}
	}
	s, token := minted(t)
	if _, err := New(s, &recordingHelper{err: context.Canceled}).Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled call must stay a cancellation, got %v", err)
	}
}

// The port satisfies the SDK contract.
var _ stacksdk.Privileged = (*Port)(nil)
