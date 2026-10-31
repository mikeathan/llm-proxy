package names

import (
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"os"
	"strings"
	"testing"
)

var target = netip.MustParseAddr("192.168.50.251")

func fixture(t *testing.T, name string) []byte {
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

func TestParseReply_ReadsThePhoneAnswerWhichNamesOnlyTheBrand(t *testing.T) {
	got, err := ParseReply(fixture(t, "mdns_ptr_response_phone.hex"), netip.MustParseAddr("192.168.50.63"), 0x1234)
	if err != nil || got != "Samsung" {
		t.Errorf("name = %q, %v", got, err)
	}
}

func TestQuery_AsksForTheReverseNameOfTheAddressOnPort5353Unicast(t *testing.T) {
	q := Query(target, 0x1234)
	if binary.BigEndian.Uint16(q[0:2]) != 0x1234 || binary.BigEndian.Uint16(q[4:6]) != 1 {
		t.Fatalf("header = % x", q[:12])
	}
	want := "\x03251\x0250\x03168\x03192\x07in-addr\x04arpa\x00\x00\x0c\x00\x01"
	if string(q[12:]) != want {
		t.Errorf("question = %q, want %q", q[12:], want)
	}
}

func TestParseReply_ReadsTheNameFromARealMacAnswer(t *testing.T) {
	got, err := ParseReply(fixture(t, "mdns_ptr_response.hex"), target, 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	if got != "example-laptop" {
		t.Errorf("name = %q, want example-laptop (.local is dropped)", got)
	}
}

func TestParseReply_RefusesAnAnswerThatIsNotForThisQuestion(t *testing.T) {
	reply := fixture(t, "mdns_ptr_response.hex")
	if _, err := ParseReply(reply, target, 0x9999); err == nil {
		t.Error("a different transaction id must be refused")
	}
	if _, err := ParseReply(reply, netip.MustParseAddr("192.168.50.7"), 0x1234); err == nil {
		t.Error("an answer about another address must be refused")
	}
	query := append([]byte{}, reply...)
	query[2] &^= 0x80 // QR bit: this is a question, not an answer
	if _, err := ParseReply(query, target, 0x1234); err == nil {
		t.Error("a question must not be taken for an answer")
	}
}

func TestParseReply_NeverTrustsTheNameOrTheBytes(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":     {},
		"truncated": fixture(t, "mdns_ptr_response.hex")[:40],
		"garbage":   []byte(strings.Repeat("\xff", 64)),
	} {
		if _, err := ParseReply(raw, target, 0x1234); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseReply_ACompressionPointerThatLoopsEndsInAnError(t *testing.T) {
	reply := fixture(t, "mdns_ptr_response.hex")
	at := len(reply) - len("\x0eexample-laptop\x05local\x00")
	loop := append(append([]byte{}, reply[:at]...), 0xc0, byte(at)) // the name is a pointer to itself
	binary.BigEndian.PutUint16(loop[at-2:], 2)
	if _, err := ParseReply(loop, target, 0x1234); err == nil {
		t.Error("a pointer to itself must be refused")
	}
}

func TestClean_KeepsOnlyPlainPrintableTextOfBoundedLength(t *testing.T) {
	cases := map[string]string{
		"Living-room-TV":              "Living-room-TV",
		"example.local":               "example",
		"  spaced  out ":              "spaced out",
		"bell\x07 and\x1b[31m escape": "bell and[31m escape",
		"line\nbreak\r\x00nul":        "linebreaknul",
		"<script>alert(1)</script>":   "<script>alert(1)</script>",
		strings.Repeat("a", 200):      strings.Repeat("a", maxNameLen),
		"‮evil-rtl":                   "evil-rtl",
		"":                            "",
		"\x00\x01":                    "",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}
