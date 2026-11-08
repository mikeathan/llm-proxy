package port

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/wire"
)

const params = `{"target":"192.168.50.10","ports":[161,53]}`

// allRanges stands in for the Network settings: it allows every target unless err is set.
type fakeRanges struct {
	err   error
	asked []string
}

func (r *fakeRanges) Check(target string) error {
	r.asked = append(r.asked, target)
	return r.err
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newPort(g Grants, h Helper) *Port {
	return New(Options{Grants: g, Helper: h, Ranges: &fakeRanges{}})
}

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
	p := newPort(s, h)
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
		_, err := newPort(s, h).Run(t.Context(), req)
		if stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
			t.Errorf("%s: %v, %d calls", name, err, h.calls)
		}
	}
}

func TestParametersTheCatalogueRefusesAreRefusedBeforeTheGrantIsUsed(t *testing.T) {
	s, token := minted(t)
	h := &recordingHelper{}
	p := newPort(s, h)
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
		_, err := newPort(s, &recordingHelper{err: c.err}).Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)})
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
	if _, err := newPort(s, &recordingHelper{err: context.Canceled}).Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled call must stay a cancellation, got %v", err)
	}
}

// The port satisfies the SDK contract.
var _ stacksdk.Privileged = (*Port)(nil)

// The operator's declared scan ranges are enforced here, before the helper is asked: the helper holds no range of its own.
func TestATargetOutsideTheDeclaredRangesNeverReachesTheHelperAndKeepsTheGrant(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"outside":       {ErrOutsideRange, "outside the scan ranges"},
		"none declared": {ErrNoRangeDeclared, "no scan range is declared"},
		"unreadable":    {errors.New("settings unreadable"), "could not be checked"},
	} {
		s, token := minted(t)
		h := &recordingHelper{}
		ranges := &fakeRanges{err: c.err}
		p := New(Options{Grants: s, Helper: h, Ranges: ranges})
		_, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)})
		var e *stacksdk.Error
		if !errors.As(err, &e) || e.Code != stacksdk.CodeDenied || !strings.Contains(e.Message, c.want) || h.calls != 0 {
			t.Errorf("%s: %v, %d calls", name, err, h.calls)
		}
		if len(ranges.asked) != 1 || ranges.asked[0] != "192.168.50.10" {
			t.Errorf("%s: the canonical target must be what is checked: %v", name, ranges.asked)
		}
		ranges.err = nil
		if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)}); err != nil {
			t.Errorf("%s: a refused target must not spend the grant: %v", name, err)
		}
	}
}

func TestWithNoRangesCheckerNothingIsScanned(t *testing.T) {
	s, token := minted(t)
	h := &recordingHelper{}
	_, err := New(Options{Grants: s, Helper: h}).Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)})
	if stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
		t.Errorf("%v, %d calls", err, h.calls)
	}
}

// MaxPerHour is the catalogue's cap on one action; the helper starts a process per connection and shares no state, so it is counted here.
func TestAnActionIsLimitedPerHour(t *testing.T) {
	a, _ := catalog.Lookup("scan_udp")
	c := &clock{now: time.Unix(1_000_000, 0)}
	store := grant.NewStore(grant.Options{Now: c.Now})
	h := &recordingHelper{}
	p := New(Options{Grants: store, Helper: h, Ranges: &fakeRanges{}, Now: c.Now})
	run := func() error {
		token, err := store.Mint("scan_udp", []byte(`{"target":"192.168.50.10","ports":[53,161]}`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: json.RawMessage(params)})
		return err
	}
	for i := range a.Caps.MaxPerHour {
		if err := run(); err != nil {
			t.Fatalf("call %d of %d: %v", i+1, a.Caps.MaxPerHour, err)
		}
		c.now = c.now.Add(time.Second)
	}
	err := run()
	var e *stacksdk.Error
	if !errors.As(err, &e) || e.Code != stacksdk.CodeUnavailable || !strings.Contains(e.Message, "hourly limit") || h.calls != a.Caps.MaxPerHour {
		t.Fatalf("over the limit: %v, %d calls", err, h.calls)
	}
	c.now = c.now.Add(time.Hour)
	if err := run(); err != nil {
		t.Errorf("an hour later the earlier calls no longer count: %v", err)
	}
}

func TestTheHourlyLimitIsPerAction(t *testing.T) {
	a, _ := catalog.Lookup("scan_udp")
	l := newLimiter(time.Now)
	for range a.Caps.MaxPerHour {
		if !l.take("scan_udp", a.Caps.MaxPerHour) {
			t.Fatal("under the limit")
		}
	}
	if l.take("scan_udp", a.Caps.MaxPerHour) || !l.take("os_detect", a.Caps.MaxPerHour) {
		t.Error("one action's calls must not use another's allowance")
	}
}
