package hostwatch

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

// The fixtures are the real answers of macOS's sysctl net.inet.tcp.pcblist_n and net.inet.udp.pcblist_n, captured on 2026-11-10 from a test process
// (pid 28884) that held: TCP 127.0.0.1:52694, [::]:52697 and 0.0.0.0:52698; UDP 0.0.0.0:56953, 127.0.0.1:57943, [::]:56753 and a socket connected
// to 192.0.2.1:9 (local port 49756). Nothing of any other program is in them.
const pcbFixturePID = 28884

func pcbFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestListenersFromPCB_TCPKeepsListeningSocketsOthersCanReach(t *testing.T) {
	got, err := listenersFromPCB(pcbFixture(t, "macos_tcp_pcblist.hex"), nil, -1, 49152)
	if err != nil {
		t.Fatal(err)
	}
	want := []Listener{{"tcp", 52697}, {"tcp", 52698}} // the loopback one is left out; the IPv6 and IPv4 wildcards are kept
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListenersFromPCB_LeavesOutThisProgramsOwnSockets(t *testing.T) {
	got, err := listenersFromPCB(pcbFixture(t, "macos_tcp_pcblist.hex"), pcbFixture(t, "macos_udp_pcblist.hex"), pcbFixturePID, 1024)
	if err != nil || len(got) != 0 {
		t.Errorf("a port this program opened is never news: %+v %v", got, err)
	}
}

func TestListenersFromPCB_UDPKeepsUnconnectedSocketsBelowTheDynamicRange(t *testing.T) {
	udp := pcbFixture(t, "macos_udp_pcblist.hex")
	got, err := listenersFromPCB(nil, udp, -1, 70000)
	if err != nil {
		t.Fatal(err)
	}
	// 56753 and 56953 are bound and unconnected; 57943 is loopback; 49756 is connected, which is somebody else's conversation, not a service.
	if len(got) != 2 || got[0] != (Listener{"udp", 56753}) || got[1] != (Listener{"udp", 56953}) {
		t.Errorf("got %+v", got)
	}
	if got, _ := listenersFromPCB(nil, udp, -1, 49152); len(got) != 0 {
		t.Errorf("ports in the dynamic range are programs that pick a new one at every start: %+v", got)
	}
}

func TestListenersFromPCB_ATableWithNoSocketsIsAnAnswerNotAnError(t *testing.T) {
	b := pcbFixture(t, "macos_tcp_pcblist.hex")
	empty := append(append([]byte{}, b[:pcbHeaderLen]...), b[len(b)-pcbHeaderLen:]...)
	if got, err := listenersFromPCB(empty, empty, -1, 49152); err != nil || len(got) != 0 {
		t.Errorf("got %+v %v", got, err)
	}
}

// A layout this program does not recognise is "could not check", never a quiet computer: a wrong offset would otherwise report nothing listening.
func TestListenersFromPCB_ALayoutItDoesNotKnowIsUnavailableNotEmpty(t *testing.T) {
	good := pcbFixture(t, "macos_tcp_pcblist.hex")
	cut := func(b []byte, n int) []byte { return append([]byte{}, b[:n]...) }
	with := func(off int, v uint32) []byte {
		b := append([]byte{}, good...)
		binary.LittleEndian.PutUint32(b[off:], v)
		return b
	}
	cases := map[string][]byte{
		"nothing":                          {},
		"shorter than the header":          good[:10],
		"a record longer than the buffer":  with(pcbHeaderLen, uint32(len(good))),
		"a record shorter than its header": with(pcbHeaderLen, 4),
		"cut inside a record":              cut(good, pcbHeaderLen+60),
		"no trailer":                       cut(good, len(good)-pcbHeaderLen),
	}
	for name, b := range cases {
		if _, err := listenersFromPCB(b, nil, -1, 49152); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: err = %v, want ErrUnavailable", name, err)
		}
	}
}

// The address family decides which bytes are the address: an IPv6 address is all sixteen, an IPv4 one is the last four (the first twelve are padding).
func TestPcbAddr_ReadsIPv6WholeAndIPv4FromTheLastFourBytes(t *testing.T) {
	v6loop := [16]byte{15: 1} // ::1
	if a := pcbAddr(vflagIPv6, v6loop[:]); !a.IsLoopback() || !a.Is6() {
		t.Errorf("::1 must read as IPv6 loopback, got %v", a)
	}
	v4 := [16]byte{12: 192, 13: 168, 14: 1, 15: 20}
	if a := pcbAddr(1, v4[:]); a.String() != "192.168.1.20" {
		t.Errorf("IPv4 is read from the last four bytes, got %v", a)
	}
	if a := pcbAddr(3, v6loop[:]); !a.Is6() { // a dual-stack socket carries both flags and an IPv6 address
		t.Errorf("both flags set is read as IPv6, got %v", a)
	}
}

// A real record with its address rewritten to loopback must stop being reported, in both families: this ties the family byte and the address
// offsets to the kernel's layout and not only to the helper that decodes them.
func TestListenersFromPCB_ARealRecordRewrittenToLoopbackIsLeftOut(t *testing.T) {
	data := append([]byte{}, pcbFixture(t, "macos_tcp_pcblist.hex")...)
	recs, err := pcbRecords(data)
	if err != nil {
		t.Fatal(err)
	}
	patched := 0
	for _, r := range recs {
		if r.kind != kindInpcb {
			continue
		}
		switch binary.BigEndian.Uint16(r.data[inpcbLocal:]) {
		case 52697: // [::]
			r.data[inpcbLocalAddr+15] = 1
			patched++
		case 52698: // 0.0.0.0
			copy(r.data[inpcbLocalAddr+12:], []byte{127, 0, 0, 1})
			patched++
		}
	}
	if patched != 2 {
		t.Fatalf("the fixture changed: patched %d records", patched)
	}
	if got, err := listenersFromPCB(data, nil, -1, 49152); err != nil || len(got) != 0 {
		t.Errorf("loopback in either family is not reachable from outside: %+v %v", got, err)
	}
}
