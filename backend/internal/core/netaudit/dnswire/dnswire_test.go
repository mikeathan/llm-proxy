package dnswire

import (
	"encoding/binary"
	"testing"
)

func question(labels ...string) []byte {
	b := make([]byte, HeaderLen)
	b[5] = 1
	for _, l := range labels {
		b = append(append(b, byte(len(l))), l...)
	}
	return append(b, 0, 0, TypePTR, 0, ClassIN)
}

func TestReadName_Plain(t *testing.T) {
	msg := question("_services", "_dns-sd", "_udp", "local")
	name, next, err := ReadName(msg, HeaderLen)
	if err != nil || name != "_services._dns-sd._udp.local" || next != len(msg)-4 {
		t.Fatalf("got %q %d %v", name, next, err)
	}
}

func TestReadName_RejectsALoopAndAForwardPointer(t *testing.T) {
	loop := append(make([]byte, HeaderLen), 0xc0, HeaderLen) // points at itself
	if _, _, err := ReadName(loop, HeaderLen); err == nil {
		t.Fatal("a pointer to itself must be refused")
	}
	fwd := append(make([]byte, HeaderLen), 0xc0, HeaderLen+4, 0, 0, 0)
	if _, _, err := ReadName(fwd, HeaderLen); err == nil {
		t.Fatal("a forward pointer must be refused")
	}
}

func TestReadName_RejectsALongName(t *testing.T) {
	var labels []string
	for range 6 {
		labels = append(labels, string(make([]byte, 63)))
	}
	if _, _, err := ReadName(question(labels...), HeaderLen); err == nil {
		t.Fatal("a name over 255 bytes must be refused")
	}
}

func TestRecords_IsBoundedWhateverTheHeaderClaims(t *testing.T) {
	msg := make([]byte, HeaderLen)
	binary.BigEndian.PutUint16(msg[6:], 65535)
	got, err := Records(msg)
	if err == nil || len(got) != 0 {
		t.Fatalf("a header claiming records that are not there is an error: %v %v", got, err)
	}
}

func TestRecords_ReadsWhatWasThereBeforeABreak(t *testing.T) {
	msg := make([]byte, HeaderLen)
	binary.BigEndian.PutUint16(msg[6:], 2)
	rec := []byte{0, 0, TypeA, 0, ClassIN, 0, 0, 0, 10, 0, 4, 192, 0, 2, 1}
	msg = append(append(msg, rec...), 0xff)
	got, err := Records(msg)
	if err == nil || len(got) != 1 || got[0].Type != TypeA {
		t.Fatalf("got %v %v", got, err)
	}
}

func FuzzRecords(f *testing.F) {
	f.Add(question("_services", "_dns-sd", "_udp", "local"))
	f.Add(append(make([]byte, HeaderLen), 0xc0, 0x0c))
	f.Fuzz(func(t *testing.T, raw []byte) {
		recs, _ := Records(raw)
		if len(recs) > MaxRecords {
			t.Fatalf("read %d records", len(recs))
		}
		for _, r := range recs {
			if r.Data < 0 || r.Data+r.Length > len(raw) {
				t.Fatalf("record data outside the message: %+v", r)
			}
		}
	})
}
