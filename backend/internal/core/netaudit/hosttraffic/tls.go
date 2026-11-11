package hosttraffic

import (
	"encoding/binary"

	"llm-proxy/internal/core/netaudit/routerlog"
)

// The server name of a TLS ClientHello (Constitution I.13). The helper keeps the first bytes of the first segment of a ClientHello, and this
// reads the one field the traffic view wants from it: the server_name extension's host name. It reads fixed offsets with a bound before every
// read, gives up at the first length that does not fit the bytes kept, and keeps nothing but one vetted name. A hello whose name lies past the
// kept bytes, one that is encrypted (Encrypted Client Hello shows only the public name of the front server), and QUIC (which carries its hello
// inside an encrypted packet) give no name or a front name, never a guess.
const (
	tlsRecordHeader = 5 // type, version (2) and length (2)
	tlsHandshake    = 0x16
	tlsClientHello  = 0x01
	tlsMajorVersion = 3
	// helloFixed is the handshake header (4), the version (2) and the random (32).
	helloFixed = 4 + 2 + 32
	// extServerName is the extension type of server_name; nameTypeHost is the entry type of a host name.
	extServerName = 0
	nameTypeHost  = 0
	extHeader     = 4
	// maxExtensions bounds how many extensions are walked, whatever the lengths say.
	maxExtensions = 64
)

// isClientHello says whether a frame is the first segment of a TLS ClientHello, which the helper's filter keeps long and never samples away: a TCP
// payload that starts a handshake record of version 3.x whose handshake type is 1. It is the filter's own test, so a frame that passes one passes
// the other.
func (f frame) isClientHello() bool {
	return f.proto == protoTCP && len(f.payload) > tlsRecordHeader && f.payload[0] == tlsHandshake && f.payload[1] == tlsMajorVersion && f.payload[tlsRecordHeader] == tlsClientHello
}

// serverName returns the vetted, lower-case host name in the server_name extension of the ClientHello that starts a TCP payload.
func serverName(p []byte) (string, bool) {
	if len(p) < tlsRecordHeader+1 || p[0] != tlsHandshake || p[1] != tlsMajorVersion || p[tlsRecordHeader] != tlsClientHello {
		return "", false
	}
	rest := p[tlsRecordHeader:]
	if len(rest) < helloFixed {
		return "", false
	}
	rest = rest[helloFixed:]
	for _, width := range []int{1, 2, 1} { // the session id, the cipher suites and the compression methods, each with its length in front
		var ok bool
		if rest, ok = skipVector(rest, width); !ok {
			return "", false
		}
	}
	if len(rest) < 2 {
		return "", false
	}
	extensions := rest[2:]
	if n := int(binary.BigEndian.Uint16(rest)); n < len(extensions) {
		extensions = extensions[:n]
	}
	return nameIn(extensions)
}

// skipVector drops a length-prefixed vector (the length is width bytes) from the front of b.
func skipVector(b []byte, width int) ([]byte, bool) {
	if len(b) < width {
		return nil, false
	}
	n := int(b[0])
	if width == 2 {
		n = int(binary.BigEndian.Uint16(b))
	}
	if len(b) < width+n {
		return nil, false
	}
	return b[width+n:], true
}

// nameIn walks the extensions for server_name. An extension that runs past the kept bytes ends the walk.
func nameIn(b []byte) (string, bool) {
	for range maxExtensions {
		if len(b) < extHeader {
			return "", false
		}
		kind, n := binary.BigEndian.Uint16(b), int(binary.BigEndian.Uint16(b[2:]))
		if len(b) < extHeader+n {
			return "", false
		}
		if kind == extServerName {
			return hostName(b[extHeader : extHeader+n])
		}
		b = b[extHeader+n:]
	}
	return "", false
}

// hostName reads the first host_name entry of a server_name list and vets it as a plain DNS name.
func hostName(list []byte) (string, bool) {
	if len(list) < 2 || int(binary.BigEndian.Uint16(list)) > len(list)-2 {
		return "", false
	}
	list = list[2 : 2+int(binary.BigEndian.Uint16(list))]
	if len(list) < 3 || list[0] != nameTypeHost {
		return "", false
	}
	n := int(binary.BigEndian.Uint16(list[1:]))
	if len(list) < 3+n {
		return "", false
	}
	return routerlog.VetName(string(list[3 : 3+n]))
}
