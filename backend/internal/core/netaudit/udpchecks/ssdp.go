package udpchecks

import (
	"bytes"
	"net/netip"
)

// ssdpSearch is the fixed UPnP M-SEARCH of Constitution I.9, addressed to one device's own unicast address instead of the group. It asks the
// device to describe where its UPnP page is.
func ssdpSearch(addr netip.Addr) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\nHOST: " + addr.String() + ":1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: upnp:rootdevice\r\n\r\n")
}

// validSSDPReply is true when the answer's first line is an HTTP/1.1 200. Nothing else in the reply is read or kept.
func validSSDPReply(data []byte) bool {
	return bytes.HasPrefix(data, []byte("HTTP/1.1 200"))
}
