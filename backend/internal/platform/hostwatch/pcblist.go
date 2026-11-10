package hostwatch

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// The socket tables of macOS (sysctl net.inet.tcp.pcblist_n and net.inet.udp.pcblist_n) are a header, then for every socket a run of records (its
// ports, who holds it, buffers and counters, and for TCP its state), then a trailer equal to the header. Each record starts with its length and its kind, is packed to 4 bytes, and is padded to 8 in the buffer. The
// offsets below were read from the kernel's answer on a real Mac and checked against lsof (docs/learnings/network-audit.md); a buffer that does not
// fit them is "could not check", never an empty answer.
const (
	pcbHeaderLen = 24 // struct xinpgen: the header and the trailer

	kindSocket = 0x001 // XSO_SOCKET: who holds it
	kindInpcb  = 0x010 // XSO_INPCB: ports and addresses; the first record of each socket
	kindTcpcb  = 0x020 // XSO_TCPCB: the TCP state

	socketLastPID  = 68 // so_last_pid, int32, within the socket record
	socketMinLen   = 72
	inpcbForeign   = 16 // inp_fport, network byte order
	inpcbLocal     = 18 // inp_lport, network byte order
	inpcbVFlag     = 44 // INP_IPV4 (1) or INP_IPV6 (2)
	inpcbLocalAddr = 64 // 16 bytes; an IPv4 address is in the last four
	inpcbMinLen    = 80
	tcpcbState     = 36 // t_state, int32
	tcpcbMinLen    = 40

	tcpsListen = 1 // TCPS_LISTEN
	vflagIPv6  = 2
)

// pcbSocket is what is read of one socket: enough to say whether it is a service others can reach and whether this program holds it.
type pcbSocket struct {
	pid       int32
	foreign   uint16
	local     uint16
	addr      netip.Addr
	tcpState  int32
	haveInpcb bool
	haveTCP   bool
	haveOwner bool
}

// errPCBLayout is the answer to a buffer that is not the layout this file reads.
func errPCBLayout(why string) error {
	return fmt.Errorf("%w: the kernel's socket list is not in a layout this program reads (%s)", ErrUnavailable, why)
}

// pcbRecord is one record of the buffer: its kind and its bytes.
type pcbRecord struct {
	kind uint32
	data []byte
}

// pcbRecords checks the framing (header, trailer, every record's length) and cuts the buffer into records. Anything off is the layout error.
func pcbRecords(data []byte) ([]pcbRecord, error) {
	if len(data) < 2*pcbHeaderLen || int(binary.LittleEndian.Uint32(data)) != pcbHeaderLen {
		return nil, errPCBLayout("no header")
	}
	end := len(data) - pcbHeaderLen // the trailer
	if int(binary.LittleEndian.Uint32(data[end:])) != pcbHeaderLen {
		return nil, errPCBLayout("no trailer")
	}
	var out []pcbRecord
	for off := pcbHeaderLen; off < end; {
		if off+8 > end {
			return nil, errPCBLayout("a record is cut short")
		}
		n := int(binary.LittleEndian.Uint32(data[off:]))
		if n < 8 || off+n > end {
			return nil, errPCBLayout("a record's length is wrong")
		}
		out = append(out, pcbRecord{kind: binary.LittleEndian.Uint32(data[off+4:]), data: data[off : off+n]})
		off += (n + 7) &^ 7
	}
	return out, nil
}

// apply reads one record into the socket it belongs to; a record too short for its fields is the layout error.
func (s *pcbSocket) apply(r pcbRecord) error {
	switch {
	case r.kind == kindSocket && len(r.data) >= socketMinLen:
		s.pid, s.haveOwner = int32(binary.LittleEndian.Uint32(r.data[socketLastPID:])), true
	case r.kind == kindTcpcb && len(r.data) >= tcpcbMinLen:
		s.tcpState, s.haveTCP = int32(binary.LittleEndian.Uint32(r.data[tcpcbState:])), true
	case r.kind == kindSocket || r.kind == kindTcpcb:
		return errPCBLayout("a record is short")
	}
	return nil
}

// newPCBSocket starts a socket from its port record, the first record of each.
func newPCBSocket(r pcbRecord) (*pcbSocket, error) {
	if len(r.data) < inpcbMinLen {
		return nil, errPCBLayout("a port record is short")
	}
	return &pcbSocket{haveInpcb: true,
		foreign: binary.BigEndian.Uint16(r.data[inpcbForeign:]),
		local:   binary.BigEndian.Uint16(r.data[inpcbLocal:]),
		addr:    pcbAddr(r.data[inpcbVFlag], r.data[inpcbLocalAddr:inpcbLocalAddr+16])}, nil
}

// complete says whether everything this protocol needs was read: ports and holder, and for TCP the state.
func (s *pcbSocket) complete(proto string) bool {
	return s.haveInpcb && s.haveOwner && (proto != "tcp" || s.haveTCP)
}

// parsePCBList reads one protocol's buffer into sockets. Nothing but numbers is kept.
func parsePCBList(data []byte, proto string) ([]pcbSocket, error) {
	recs, err := pcbRecords(data)
	if err != nil {
		return nil, err
	}
	var out []pcbSocket
	var cur *pcbSocket
	flush := func() {
		if cur != nil && cur.complete(proto) {
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, r := range recs {
		if r.kind == kindInpcb {
			flush()
			if cur, err = newPCBSocket(r); err != nil {
				return nil, err
			}
			continue
		}
		if cur == nil {
			return nil, errPCBLayout("a record comes before any port record")
		}
		if err := cur.apply(r); err != nil {
			return nil, err
		}
	}
	flush()
	return out, nil
}

// pcbAddr reads a local address: IPv6 as it is, IPv4 from the last four of the sixteen bytes.
func pcbAddr(vflag byte, raw []byte) netip.Addr {
	if vflag&vflagIPv6 != 0 {
		return netip.AddrFrom16([16]byte(raw))
	}
	return netip.AddrFrom4([4]byte(raw[12:16]))
}

// listenersFromPCB is Proc.Listeners for the tables of macOS, with the same rules: a TCP socket that is listening or a UDP socket that is bound
// and not connected, never loopback, never one this program holds (ownPID), and no UDP port in the dynamic range (ephemeralLow), which programs
// pick anew at every start. A nil buffer is not read.
func listenersFromPCB(tcp, udp []byte, ownPID, ephemeralLow int) ([]Listener, error) {
	seen := map[Listener]bool{}
	for _, t := range []struct {
		data  []byte
		proto string
	}{{tcp, "tcp"}, {udp, "udp"}} {
		if t.data == nil {
			continue
		}
		rows, err := parsePCBList(t.data, t.proto)
		if err != nil {
			return nil, err
		}
		for _, s := range rows {
			if s.isService(t.proto, ownPID, ephemeralLow) {
				seen[Listener{t.proto, int(s.local)}] = true
			}
		}
	}
	return sortedListeners(seen), nil
}

// isService says whether the socket is a service others can reach: listening (TCP) or bound and unconnected (UDP), not loopback, not held by this
// program, and for UDP below the dynamic range.
func (s pcbSocket) isService(proto string, ownPID, ephemeralLow int) bool {
	if s.foreign != 0 || s.local == 0 || s.addr.IsLoopback() || int(s.pid) == ownPID {
		return false
	}
	if proto == "tcp" {
		return s.tcpState == tcpsListen
	}
	return int(s.local) < ephemeralLow
}
