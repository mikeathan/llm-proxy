//go:build !linux

package routeprobe

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestOutsideLinuxAProbeSaysItIsUnsupported(t *testing.T) {
	if _, err := Probe(context.Background(), Request{Dst: netip.MustParseAddr("127.0.0.1"), TTL: 1}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
