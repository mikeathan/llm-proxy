//go:build !linux

package helper

import (
	"errors"
	"net"
)

// IdentifyPeer: the helper is Linux only (macOS has no unit and reports the actions unavailable), so there is no peer to identify.
func IdentifyPeer(net.Conn) (Peer, error) {
	return Peer{}, errors.New("peer identity is available on Linux only")
}
