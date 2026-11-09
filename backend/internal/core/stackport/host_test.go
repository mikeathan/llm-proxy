package stackport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/tools"
)

func TestGuard_ARefusalIsDeniedAndAFailureIsPassedOn(t *testing.T) {
	refused := Guard{Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, tools.ValidateDomainBoundary("a.bad.example", []string{"bad.example"})
	}}
	if _, err := refused.DialContext(context.Background(), "tcp", "x:1"); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("code = %q", stacksdk.CodeOf(err))
	}
	boom := errors.New("connection refused")
	failing := Guard{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, fmt.Errorf("dial: %w", boom) }}
	if _, err := failing.DialContext(context.Background(), "tcp", "x:1"); !errors.Is(err, boom) {
		t.Errorf("a failure that is not a refusal must keep its error: %v", err)
	}
}

func TestGuard_BoundaryUsesTheGuardsOwnClassifier(t *testing.T) {
	for addr, want := range map[string]stacksdk.Boundary{
		"192.168.1.1": stacksdk.BoundaryLAN, "10.0.0.5": stacksdk.BoundaryLAN, "1.1.1.1": stacksdk.BoundaryInternet,
		"127.0.0.1": stacksdk.BoundaryBlocked, "169.254.169.254": stacksdk.BoundaryBlocked, "0.0.0.0": stacksdk.BoundaryBlocked,
	} {
		if got := (Guard{}).Boundary(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s = %q, want %q", addr, got, want)
		}
	}
}

func TestGateway_ReturnsTheAddressOrNotFound(t *testing.T) {
	g := Gateway{Read: func() netip.Addr { return netip.MustParseAddr("192.168.50.1") }}
	if got, err := g.Default(context.Background()); err != nil || got != "192.168.50.1" {
		t.Errorf("got %q, %v", got, err)
	}
	none := Gateway{Read: func() netip.Addr { return netip.Addr{} }}
	if _, err := none.Default(context.Background()); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("code = %q", stacksdk.CodeOf(err))
	}
}

// The scoped dial applies the operator's declared scan scope first: an address outside it is refused without a dial, so a stack can never
// reach an address the core's own probes would not (ADR-0038 rule 2). The guard's own refusals still apply inside the scope.
func TestScopedGuard_RefusesWhatTheScopeRefusesAndNeverDialsIt(t *testing.T) {
	inScope := netip.MustParseAddr("192.168.1.30")
	dialed := 0
	g := ScopedGuard{
		Check: func(a netip.Addr) error {
			if a != inScope {
				return errors.New("outside the declared scan scope")
			}
			return nil
		},
		Dial: func(_ context.Context, _, addr string) (net.Conn, error) {
			dialed++
			return nil, errors.New("dialed " + addr)
		},
	}
	for _, addr := range []string{"192.168.1.31:161", "10.0.0.5:1900", "not an address", "192.168.1.30", ":161"} {
		if _, err := g.DialContext(context.Background(), "udp", addr); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
			t.Errorf("%q: code = %q, want denied", addr, stacksdk.CodeOf(err))
		}
	}
	if dialed != 0 {
		t.Fatalf("the guarded dial was reached %d times for an address outside the scope", dialed)
	}
	if _, err := g.DialContext(context.Background(), "udp", "192.168.1.30:161"); dialed != 1 || err == nil || stacksdk.CodeOf(err) == stacksdk.CodeDenied {
		t.Errorf("an address inside the scope must reach the guarded dial and keep its error: dialed=%d err=%v", dialed, err)
	}
}

func TestScopedGuard_AGuardRefusalInsideTheScopeIsStillDenied(t *testing.T) {
	g := ScopedGuard{
		Check: func(netip.Addr) error { return nil },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, tools.ValidateDomainBoundary("a.bad.example", []string{"bad.example"})
		},
	}
	if _, err := g.DialContext(context.Background(), "udp", "192.168.1.30:161"); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("code = %q", stacksdk.CodeOf(err))
	}
	if got := g.Boundary(netip.MustParseAddr("192.168.1.30")); got != stacksdk.BoundaryLAN {
		t.Errorf("boundary = %q", got)
	}
}
