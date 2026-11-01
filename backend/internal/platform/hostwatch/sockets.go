// Package hostwatch reads what this computer exposes and what survives a reboot, from files it can read as the service's own
// user: the kernel's socket tables under /proc, and a short closed list of configuration files. It starts no program,
// opens no socket and sends no packet. Nothing it reads is passed on as text: sockets become a protocol and a port number,
// files become a hash.
package hostwatch

import (
	"bufio"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
)

// Socket states in /proc/net: TCP's listening state and UDP's unconnected one.
const (
	stateTCPListen = "0A"
	stateUDPBound  = "07"
)

// Socket is one row of a /proc/net socket table that is waiting for traffic.
type Socket struct {
	Proto string // "tcp" or "udp"
	Addr  netip.Addr
	Port  int
	Inode string
}

// ParseSockets reads the layout of /proc/net/tcp, tcp6, udp and udp6 and keeps the rows that are listening (TCP state 0A) or
// bound but not connected (UDP state 07). A connected socket is somebody else's connection, not a service. A row it cannot
// read is skipped: the file is the kernel's, but a bad row must not hide the rest.
func ParseSockets(r io.Reader, proto string) ([]Socket, error) {
	want := stateTCPListen
	if proto == "udp" {
		want = stateUDPBound
	}
	var out []Socket
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for first := true; sc.Scan(); {
		if first { // the header
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 10 || f[3] != want {
			continue
		}
		addr, port, ok := parseEndpoint(f[1])
		if !ok {
			continue
		}
		out = append(out, Socket{Proto: proto, Addr: addr, Port: port, Inode: f[9]})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the socket table: %w", err)
	}
	return out, nil
}

// parseEndpoint reads "0100007F:0277": an address as 32-bit words in the kernel's byte order (little-endian on the hosts
// this runs on, which is also the only order the capture in testdata proves), and the port as plain hexadecimal.
func parseEndpoint(s string) (netip.Addr, int, bool) {
	host, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return netip.Addr{}, 0, false
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil || (len(host) != 8 && len(host) != 32) {
		return netip.Addr{}, 0, false
	}
	var raw [16]byte
	for i := 0; i < len(host); i += 8 {
		word, err := strconv.ParseUint(host[i:i+8], 16, 32)
		if err != nil {
			return netip.Addr{}, 0, false
		}
		j := i / 2
		raw[j], raw[j+1], raw[j+2], raw[j+3] = byte(word), byte(word>>8), byte(word>>16), byte(word>>24)
	}
	if len(host) == 8 {
		return netip.AddrFrom4([4]byte(raw[:4])), int(port), true
	}
	return netip.AddrFrom16(raw).Unmap(), int(port), true
}
