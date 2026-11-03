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

	"llm-proxy/internal/core/netaudit/dnswire"
)

// MDNSPort is where devices listen for mDNS questions.
const MDNSPort = 5353

const (
	typePTR    = 12
	classIN    = 1
	headerLen  = 12
	maxNameLen = 40 // runes kept of a name
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
	h, _ := dnswire.ParseHeader(raw)
	off, err := dnswire.SkipQuestions(raw, int(h.Questions))
	if err != nil {
		return "", errBadReply
	}
	want := reverseName(addr)
	for range min(int(h.Answers), dnswire.MaxRecords) {
		rec, err := dnswire.ReadRecord(raw, off)
		if err != nil {
			return "", errBadReply
		}
		if rec.Type == typePTR && rec.Class == classIN && strings.EqualFold(rec.Owner, want) {
			name, _, err := dnswire.ReadName(raw, rec.Data)
			if cleaned := Clean(name); err == nil && cleaned != "" {
				return cleaned, nil
			}
			return "", errBadReply
		}
		off = rec.Data + rec.Length
	}
	return "", errBadReply
}

// isAnswerTo is true for a successful response carrying the transaction id that was sent, or id 0: an mDNS responder
// that answers as it would to the multicast group (an Android phone did, 2026-10-31) leaves the id at 0 instead of
// echoing it. The socket is connected to the device and the answer must name the address asked about, so id 0 admits
// nothing from another host or about another address.
func isAnswerTo(raw []byte, id uint16) bool {
	h, err := dnswire.ParseHeader(raw)
	if err != nil || (h.ID != id && h.ID != 0) {
		return false
	}
	return h.IsSuccessfulResponse()
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

func reverseName(addr netip.Addr) string {
	a := addr.As4()
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", a[3], a[2], a[1], a[0])
}
