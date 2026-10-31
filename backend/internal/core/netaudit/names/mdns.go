// Package names learns what a device calls itself, for suggesting a name to the operator. It asks one device one
// question (a unicast mDNS reverse lookup) and reads the one answer. Everything a device sends is untrusted: a name is
// cleaned and bounded, shown on screen only, and never put in a finding, an alert or a prompt.
package names

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MDNSPort is where devices listen for mDNS questions.
const MDNSPort = 5353

const (
	typePTR      = 12
	classIN      = 1
	headerLen    = 12
	maxNameLen   = 40  // runes kept of a name
	maxWireName  = 255 // the DNS limit for a whole name
	maxPointers  = 8
	responseFlag = 0x8000
)

var errBadReply = errors.New("not a usable mDNS answer")

// Query builds the question "what is the name of this address" (a PTR for its reverse name). It is sent straight to the
// device on port 5353, not to the multicast group, so nothing else on the network is asked.
func Query(addr netip.Addr, id uint16) []byte {
	a := addr.As4()
	q := make([]byte, headerLen, 64)
	binary.BigEndian.PutUint16(q[0:], id)
	binary.BigEndian.PutUint16(q[4:], 1) // one question
	for i := 3; i >= 0; i-- {
		q = appendLabel(q, fmt.Sprint(a[i]))
	}
	q = appendLabel(q, "in-addr")
	q = appendLabel(q, "arpa")
	q = append(q, 0)
	return binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(q, typePTR), classIN)
}

func appendLabel(b []byte, label string) []byte { return append(append(b, byte(len(label))), label...) }

// ParseReply reads the device's answer and returns its cleaned name. It accepts only a response with the right
// transaction id whose answer is a PTR for the address that was asked about; anything else is an error.
func ParseReply(raw []byte, addr netip.Addr, id uint16) (string, error) {
	if !isAnswerTo(raw, id) {
		return "", errBadReply
	}
	off, err := skipQuestions(raw, int(binary.BigEndian.Uint16(raw[4:])))
	if err != nil {
		return "", err
	}
	want := reverseName(addr)
	for range int(binary.BigEndian.Uint16(raw[6:])) {
		rec, err := readRecord(raw, off)
		if err != nil {
			return "", err
		}
		if rec.kind == typePTR && rec.class == classIN && strings.EqualFold(rec.owner, want) {
			name, _, err := readName(raw, rec.data)
			if cleaned := Clean(name); err == nil && cleaned != "" {
				return cleaned, nil
			}
			return "", errBadReply
		}
		off = rec.data + rec.length
	}
	return "", errBadReply
}

// isAnswerTo is true for a successful response carrying the transaction id that was sent.
func isAnswerTo(raw []byte, id uint16) bool {
	if len(raw) < headerLen || binary.BigEndian.Uint16(raw[0:]) != id {
		return false
	}
	flags := binary.BigEndian.Uint16(raw[2:])
	return flags&responseFlag != 0 && flags&0x000f == 0
}

// skipQuestions returns the offset of the first answer.
func skipQuestions(raw []byte, n int) (int, error) {
	off := headerLen
	for range n {
		_, next, err := readName(raw, off)
		if err != nil || next+4 > len(raw) {
			return 0, errBadReply
		}
		off = next + 4
	}
	return off, nil
}

// record is one answer's header: who it is about, what it is, and where its data starts and how long it is.
type record struct {
	owner       string
	kind, class uint16
	data        int
	length      int
}

func readRecord(raw []byte, off int) (record, error) {
	owner, next, err := readName(raw, off)
	if err != nil || next+10 > len(raw) {
		return record{}, errBadReply
	}
	rec := record{
		owner: owner, kind: binary.BigEndian.Uint16(raw[next:]), class: binary.BigEndian.Uint16(raw[next+2:]) & 0x7fff,
		data: next + 10, length: int(binary.BigEndian.Uint16(raw[next+8:])),
	}
	if rec.data+rec.length > len(raw) {
		return record{}, errBadReply
	}
	return rec, nil
}

func reverseName(addr netip.Addr) string {
	a := addr.As4()
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", a[3], a[2], a[1], a[0])
}

// readName reads a possibly compressed name starting at off and returns it with the offset just after it in the
// message. A pointer may only go backwards and only a few times, so a crafted loop ends in an error.
func readName(msg []byte, off int) (string, int, error) {
	var labels []string
	next, hops, total := 0, 0, 0
	for {
		if off >= len(msg) {
			return "", 0, errBadReply
		}
		n := int(msg[off])
		switch {
		case n == 0:
			if hops == 0 {
				next = off + 1
			}
			return strings.Join(labels, "."), next, nil
		case n&0xc0 == 0xc0:
			target, ok := pointerTarget(msg, off, hops)
			if !ok {
				return "", 0, errBadReply
			}
			if hops == 0 {
				next = off + 2
			}
			off, hops = target, hops+1
		case n&0xc0 != 0 || off+1+n > len(msg):
			return "", 0, errBadReply
		default:
			if total += n + 1; total > maxWireName {
				return "", 0, errBadReply
			}
			labels = append(labels, string(msg[off+1:off+1+n]))
			off += 1 + n
		}
	}
}

// pointerTarget is where a compression pointer at off leads; it must lead backwards, within the hop limit.
func pointerTarget(msg []byte, off, hops int) (int, bool) {
	if off+1 >= len(msg) || hops >= maxPointers {
		return 0, false
	}
	target := int(msg[off]&0x3f)<<8 | int(msg[off+1])
	return target, target < off
}

// Clean makes a device's name safe to show: it drops the ".local" ending, every control and invisible character
// (including the ones that reverse text direction), collapses spaces and keeps at most maxNameLen characters. It does
// not escape anything; the screen escapes text, and the name is never used anywhere else.
func Clean(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".local")
	var b strings.Builder
	space := false
	for _, r := range name {
		switch {
		case r == utf8.RuneError || unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if out := []rune(b.String()); len(out) > maxNameLen {
		return string(out[:maxNameLen])
	}
	return b.String()
}
