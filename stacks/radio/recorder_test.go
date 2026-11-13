package radio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// fakeVault stands in for the core's: a tag is a hash, a sealed value is the bytes XOR 0x5a. Neither contains the value.
type fakeVault struct{}

func (fakeVault) Tag(_ context.Context, purpose string, v []byte) ([]byte, error) {
	h := sha256.Sum256(append([]byte(purpose+"|"), v...))
	return h[:], nil
}

func (fakeVault) Seal(_ context.Context, _ string, v []byte) ([]byte, error) {
	out := make([]byte, len(v))
	for i, b := range v {
		out[i] = b ^ 0x5a
	}
	return out, nil
}

func (fakeVault) Open(ctx context.Context, p string, v []byte) ([]byte, error) {
	return fakeVault{}.Seal(ctx, p, v)
}

var _ stacksdk.Vault = fakeVault{}

var (
	mineMAC     = MAC{0x3a, 0x91, 0x5c, 0xd2, 0x7e, 0x44}
	neighbour   = MAC{0xb8, 0x27, 0xeb, 0x19, 0x6d, 0xa3}
	neighbourAP = MAC{0xdc, 0xa6, 0x32, 0x8f, 0x10, 0x5b}
)

func data(from MAC) []byte {
	return rtap(cat(hdr(0x88, 0x01, macAP, from, macAP), []byte{0, 0}, make([]byte, 10)))
}

func record(t *testing.T, r *Recorder, raw []byte, at time.Time) {
	t.Helper()
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Record(context.Background(), f, at); err != nil {
		t.Fatal(err)
	}
}

func TestRecorderSeparatesTheOwnersDevicesFromOthers(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	if err := r.AddOwnerDevice(ctx, mineMAC, "my phone", t0); err != nil {
		t.Fatal(err)
	}
	record(t, r, data(mineMAC), t0)
	record(t, r, data(neighbour), t0)
	record(t, r, data(neighbour), day(1))
	mine, _ := fakeVault{}.Tag(ctx, tagDevice, mineMAC[:])
	other, _ := fakeVault{}.Tag(ctx, tagDevice, neighbour[:])
	d, ok, _ := s.Device(ctx, mine)
	if !ok || !d.Owner || d.Label != "my phone" || d.Visits != 1 || !d.HasSignal {
		t.Errorf("owner: %+v", d)
	}
	got, _ := fakeVault{}.Open(ctx, sealOwner, d.Sealed)
	if !bytes.Equal(got, mineMAC[:]) {
		t.Errorf("the owner's sealed address must open to the address, got %x", got)
	}
	d, ok, _ = s.Device(ctx, other)
	if !ok || d.Owner || d.Visits != 2 || d.Sealed != nil || d.Label != "" {
		t.Errorf("neighbour: %+v", d)
	}
}

func TestRecorderCountsEventsAndKeepsBeaconNamesOfOthersOut(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	record(t, r, rtap(cat(hdr(0xc0, 0, macAP, neighbour, macAP), []byte{3, 0})), t0) // deauthentication
	if n, _ := s.count("events"); n != 1 {
		t.Errorf("events=%d", n)
	}
	record(t, r, beacon("PrivateName", rsnWPA2), t0)
	if n, _ := s.count("beacons"); n != 1 {
		t.Errorf("beacons=%d", n)
	}
	var name string
	var nl int
	if err := s.db.QueryRow("SELECT name, name_len FROM beacons").Scan(&name, &nl); err != nil || name != "" || nl != len("PrivateName") {
		t.Errorf("another network's name is kept only as a length: %q %d %v", name, nl, err)
	}
}

// The database holds no address of another person and no name a device asked for, whatever was heard (ADR-0053).
func TestRecorderLeavesNoAddressOrProbeNameInTheFile(t *testing.T) {
	s, p := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	_ = r.AddOwnerDevice(context.Background(), mineMAC, "phone", t0)
	record(t, r, data(neighbour), t0)
	record(t, r, data(mineMAC), t0)
	record(t, r, rtap(cat(hdr(0x40, 0, macBroad, neighbour, macBroad), ie(0, []byte("GranddadsWiFi")...))), t0)
	record(t, r, rtap(cat(hdr(0xb0, 0, neighbourAP, neighbour, neighbourAP), []byte{0, 0, 1, 0, 0, 0})), t0)
	b := hdr(0x80, 0, macBroad, neighbourAP, neighbourAP)
	record(t, r, rtap(cat(b, make([]byte, 12), ie(0, []byte("NeighbourNet")...), ie(3, 6))), t0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, f := range []string{p, p + "-wal"} {
		if b, err := os.ReadFile(f); err == nil {
			all = append(all, b...)
		}
	}
	if len(all) == 0 {
		t.Fatal("no database bytes read")
	}
	for name, m := range map[string]MAC{"mine": mineMAC, "neighbour": neighbour, "neighbour AP": neighbourAP} {
		for _, form := range [][]byte{m[:], []byte(m.String()), []byte(hex.EncodeToString(m[:])), []byte(strings.ToUpper(m.String()))} {
			if bytes.Contains(all, form) {
				t.Errorf("%s address appears in the file as %q", name, form)
			}
		}
	}
	for _, w := range []string{"GranddadsWiFi", "NeighbourNet"} {
		if bytes.Contains(all, []byte(w)) {
			t.Errorf("%q appears in the file", w)
		}
	}
	_ = filepath.Base(p)
}
