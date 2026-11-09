package hosttraffic

import (
	"net/netip"

	"llm-proxy/internal/core/netaudit/dnswire"
	"llm-proxy/internal/core/netaudit/routerlog"
)

const (
	portDNS    = 53
	typeAAAA   = 28
	tcpLenPref = 2
	// maxAnswers bounds how many address records one message is read for, whatever its header claims.
	maxAnswers = 16
	// maxNames bounds how many addresses are named in one window.
	maxNames = 256

	flagResponse = 0x8000
	rcodeMask    = 0x000f
)

// isDNS says whether a frame is a DNS message: either port is 53.
func (f frame) isDNS() bool { return f.sport == portDNS || f.dport == portDNS }

// dnsMessage returns the DNS message in a frame's payload: the payload itself over UDP, and past the two-byte length over TCP. A message
// split over several TCP segments is not read.
func (f frame) dnsMessage() []byte {
	if f.proto == protoTCP {
		if len(f.payload) <= tcpLenPref {
			return nil
		}
		return f.payload[tcpLenPref:]
	}
	return f.payload
}

// dnsReading is what one DNS message told: a question asked, or the addresses an answer gave for the name that was asked.
type dnsReading struct {
	question bool
	name     string
	addrs    []netip.Addr
}

// readDNS reads one DNS message with the program's one bounded reader. A malformed or cut message gives what was read before it. The name is
// the one in the question, vetted as a plain DNS name (Constitution I.13): a name that is not one is dropped, and the addresses go unnamed.
func readDNS(msg []byte) dnsReading {
	h, err := dnswire.ParseHeader(msg)
	if err != nil {
		return dnsReading{}
	}
	if h.Flags&flagResponse == 0 {
		return dnsReading{question: h.Questions > 0}
	}
	if h.Flags&rcodeMask != 0 || h.Questions != 1 {
		return dnsReading{}
	}
	name, _, err := dnswire.ReadName(msg, dnswire.HeaderLen)
	if err != nil {
		return dnsReading{}
	}
	out := dnsReading{}
	if vetted, ok := routerlog.VetName(name); ok {
		out.name = vetted
	}
	out.addrs = answerAddresses(msg, h)
	return out
}

// answerAddresses lists the A and AAAA addresses in the answer section, at most maxAnswers.
func answerAddresses(msg []byte, h dnswire.Header) []netip.Addr {
	off, err := dnswire.SkipQuestions(msg, int(h.Questions))
	if err != nil {
		return nil
	}
	var addrs []netip.Addr
	for range min(int(h.Answers), dnswire.MaxRecords) {
		rec, err := dnswire.ReadRecord(msg, off)
		if err != nil {
			break
		}
		off = rec.Data + rec.Length
		if a, ok := recordAddress(msg, rec); ok {
			if addrs = append(addrs, a); len(addrs) >= maxAnswers {
				break
			}
		}
	}
	return addrs
}

func recordAddress(msg []byte, rec dnswire.Record) (netip.Addr, bool) {
	data := msg[rec.Data : rec.Data+rec.Length]
	switch {
	case rec.Class != dnswire.ClassIN:
	case rec.Type == dnswire.TypeA && len(data) == 4:
		return netip.AddrFrom4([4]byte(data)), true
	case rec.Type == typeAAAA && len(data) == 16:
		return netip.AddrFrom16([16]byte(data)), true
	}
	return netip.Addr{}, false
}
