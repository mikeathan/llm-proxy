package hosttraffic

import (
	"encoding/binary"
	"strings"
	"testing"
)

// clientHello builds the start of a TLS 1.3-shaped ClientHello: record header, handshake header, version, random, a session id, cipher suites,
// compression and the extensions given, each as type and body.
func clientHello(exts ...[]byte) []byte {
	body := []byte{0x03, 0x03}
	body = append(body, make([]byte, 32)...)
	body = append(body, 32)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0, 6, 0x13, 0x01, 0x13, 0x02, 0x13, 0x03)
	body = append(body, 1, 0)
	var all []byte
	for _, e := range exts {
		all = append(all, e...)
	}
	body = binary.BigEndian.AppendUint16(body, uint16(len(all)))
	body = append(body, all...)
	hs := append([]byte{0x01, 0, byte(len(body) >> 8), byte(len(body))}, body...)
	return append([]byte{0x16, 0x03, 0x01, byte(len(hs) >> 8), byte(len(hs))}, hs...)
}

func ext(kind uint16, body []byte) []byte {
	out := binary.BigEndian.AppendUint16(nil, kind)
	out = binary.BigEndian.AppendUint16(out, uint16(len(body)))
	return append(out, body...)
}

func sniExt(name string) []byte {
	entry := append([]byte{0, byte(len(name) >> 8), byte(len(name))}, name...)
	list := append([]byte{byte(len(entry) >> 8), byte(len(entry))}, entry...)
	return ext(0, list)
}

func TestTheServerNameIsReadFromAClientHello(t *testing.T) {
	got, ok := serverName(clientHello(ext(43, []byte{2, 3, 4}), sniExt("Example.COM"), ext(16, []byte{0, 3, 2, 'h', '2'})))
	if !ok || got != "example.com" {
		t.Fatalf("serverName = %q, %v; want the name in lower case", got, ok)
	}
}

func TestAHelloCutBeforeTheNameGivesNoName(t *testing.T) {
	full := clientHello(ext(43, make([]byte, 600)), sniExt("late.example.com"))
	for _, cut := range []int{0, 3, 5, 9, 43, 80, 120, len(full) - 5, len(full) - 1} {
		if got, ok := serverName(full[:cut]); ok {
			t.Errorf("cut at %d of %d: read %q from a hello that ends before the name does", cut, len(full), got)
		}
	}
	if got, ok := serverName(clientHello(ext(43, make([]byte, 300)), sniExt("seen.example.com"))); !ok || got != "seen.example.com" {
		t.Errorf("a name after other extensions was not found: %q %v", got, ok)
	}
}

func TestOnlyAHostNameFromAClientHelloIsAName(t *testing.T) {
	notHello := [][]byte{
		nil, {0x16}, []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		append([]byte{0x17, 3, 3}, clientHello(sniExt("example.com"))[3:]...),
		append([]byte{0x16, 3, 3, 0, 0, 2}, clientHello(sniExt("example.com"))[6:]...), // a ServerHello
		append([]byte{0x16, 2, 1}, clientHello(sniExt("example.com"))[3:]...),
	}
	for i, p := range notHello {
		if got, ok := serverName(p); ok {
			t.Errorf("payload %d: read %q from something that is not a ClientHello", i, got)
		}
	}
	// A name type other than host_name (0) is not read.
	other := ext(0, append([]byte{0, 8, 1, 0, 5}, "x.com"...))
	if got, ok := serverName(clientHello(other)); ok {
		t.Errorf("read %q from a name type that is not host_name", got)
	}
}

func TestAServerNameThatIsNotAPlainDNSNameIsDropped(t *testing.T) {
	for _, bad := range []string{"", "bad name.example.com", "ex\x01ample.com", "x.in-addr.arpa", strings.Repeat("a", 64) + ".com", "192.168.1.1\n", "-x.com", "a..com"} {
		if got, ok := serverName(clientHello(sniExt(bad))); ok {
			t.Errorf("%q: kept the name %q", bad, got)
		}
	}
}

func TestAHelloThatLiesAboutItsLengthsIsReadSafely(t *testing.T) {
	good := clientHello(sniExt("example.com"))
	for i := range good {
		for _, v := range []byte{0, 0xff, 0x80} {
			bad := append([]byte(nil), good...)
			bad[i] = v
			serverName(bad) // must not panic or hang whatever byte is changed
		}
	}
	// An extension block that claims more than the frame has, and a list that claims more than its extension holds.
	huge := clientHello(ext(0, []byte{0xff, 0xff, 0, 0xff, 0xff}))
	if got, ok := serverName(huge); ok {
		t.Errorf("read %q from a name that claims 65 KB", got)
	}
}
