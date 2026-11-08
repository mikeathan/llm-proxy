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
