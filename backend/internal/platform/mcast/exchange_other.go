//go:build !unix

package mcast

import (
	"context"
	"net/netip"
)

// Exchange is not available on this system.
func Exchange(context.Context, Interface, []Question, Limits, func(netip.Addr) bool) ([]Datagram, error) {
	return nil, ErrUnavailable
}
