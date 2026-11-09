package udpchecks

import "net/netip"

// snmpDiscovery is the standard SNMPv3 engine-discovery request (RFC 3414 section 4): version 3, msgFlags reportable with no authentication and
// no privacy, an empty user name and an empty engine id. It carries no community string and no credential; an agent that speaks v3 answers
// with a "report" naming its engine. It is the same fixed message every time, built from constants.
func snmpDiscovery(netip.Addr) []byte {
	return []byte{
		0x30, 0x3a, // message
		0x02, 0x01, 0x03, // version 3
		0x30, 0x0f, 0x02, 0x02, 0x4a, 0x69, 0x02, 0x03, 0x00, 0xff, 0xe3, 0x04, 0x01, 0x04, 0x02, 0x01, 0x03, // msgID, max size, flags (reportable), USM
		0x04, 0x10, 0x30, 0x0e, 0x04, 0x00, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x04, 0x00, 0x04, 0x00, 0x04, 0x00, // empty security parameters
		0x30, 0x12, 0x04, 0x00, 0x04, 0x00, 0xa0, 0x0c, 0x02, 0x02, 0x37, 0xf0, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x30, 0x00, // empty GetRequest
	}
}

const (
	tagSequence  = 0x30
	tagInteger   = 0x02
	tagOctets    = 0x04
	tagReportPDU = 0xa8
	snmpV3       = 3
)

// ber reads BER elements from a byte slice, refusing anything that does not fit.
type ber struct{ b []byte }

// next returns the tag and content of the next element, and false when the element is malformed or runs past the data.
func (r *ber) next() (tag byte, content []byte, ok bool) {
	if len(r.b) < 2 {
		return 0, nil, false
	}
	tag = r.b[0]
	length, header := int(r.b[1]), 2
	if length&0x80 != 0 {
		n := length & 0x7f
		if n == 0 || n > 2 || len(r.b) < 2+n {
			return 0, nil, false
		}
		length = 0
		for _, c := range r.b[2 : 2+n] {
			length = length<<8 | int(c)
		}
		header = 2 + n
	}
	if length > len(r.b)-header {
		return 0, nil, false
	}
	content = r.b[header : header+length]
	r.b = r.b[header+length:]
	return tag, content, true
}

// expect reads the next element and says whether it has the tag wanted.
func (r *ber) expect(tag byte) ([]byte, bool) {
	got, content, ok := r.next()
	return content, ok && got == tag
}

// skip reads the next elements, each of which must have the tag given.
func (r *ber) skip(tags ...byte) bool {
	for _, t := range tags {
		if _, ok := r.expect(t); !ok {
			return false
		}
	}
	return true
}

// validSNMPv3Report is true when data is a well-formed SNMPv3 message whose scoped PDU is a report: the agent answered the discovery. It
// reads shape only (tags, lengths, the version number) and keeps nothing.
func validSNMPv3Report(data []byte) bool {
	msg := ber{data}
	body, ok := msg.expect(tagSequence)
	if !ok {
		return false
	}
	in := ber{body}
	if v, ok := in.expect(tagInteger); !ok || len(v) != 1 || v[0] != snmpV3 {
		return false
	}
	// msgGlobalData, then msgSecurityParameters; an encrypted scoped PDU is an octet string, not a sequence, and is not a report.
	if !in.skip(tagSequence, tagOctets) {
		return false
	}
	scoped, ok := in.expect(tagSequence)
	if !ok {
		return false
	}
	pdu := ber{scoped}
	return pdu.skip(tagOctets, tagOctets) && pdu.skip(tagReportPDU)
}
