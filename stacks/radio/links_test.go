package radio

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// confirmHomeAP makes macAP one of the owner's confirmed access points, as a confirmation on the Network page does.
func confirmHomeAP(t *testing.T, s *Store, r *Recorder, guest bool) {
	t.Helper()
	ctx := context.Background()
	apTag, _ := fakeVault{}.Tag(ctx, tagAP, macAP[:])
	if err := s.ProposeOwnAP(ctx, OwnAP{Tag: apTag, Sealed: []byte{1}, Name: "home"}, t0); err != nil {
		t.Fatal(err)
	}
	if guest {
		if _, err := s.SetOwnAPGuest(ctx, apTag, true); err != nil {
			t.Fatal(err)
		}
	}
	confirmTags(t, r, apTag)
}

func rowOf(t *testing.T, s *Store, m MAC) FeatureRow {
	t.Helper()
	ctx := context.Background()
	want, _ := fakeVault{}.Tag(ctx, tagDevice, m[:])
	rows, _, err := s.FeatureRows(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if bytes.Equal(r.Tag, want) {
			return r
		}
	}
	t.Fatalf("no row for %s", m)
	return FeatureRow{}
}

func cycle(t *testing.T, r *Recorder, frames ...[]byte) {
	t.Helper()
	r.BeginCycle()
	for _, f := range frames {
		record(t, r, f, t0)
	}
	if err := r.EndCycle(context.Background(), t0); err != nil {
		t.Fatal(err)
	}
}

func TestADeviceExchangingDataWithYourAccessPointIsMarkedAfterOneCycle(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	confirmHomeAP(t, s, r, false)
	cycle(t, r, data(neighbour)) // a data frame to the AP: no association request was heard
	row := rowOf(t, s, neighbour)
	if !row.JoinedOwn || row.OwnCycles != 1 {
		t.Fatalf("a device sending data to your access point should be marked: %+v", row)
	}
	apDev, _ := fakeVault{}.Tag(context.Background(), tagDevice, macAP[:])
	if !bytes.Equal(row.Net, apDev) {
		t.Errorf("the link should point at the access point's device row")
	}
	if row.Owner {
		t.Error("evidence never makes a device the owner's by itself")
	}
}

func TestAGuestNetworkIsNoEvidenceOfOwnership(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	confirmHomeAP(t, s, r, true)
	cycle(t, r, data(neighbour))
	if row := rowOf(t, s, neighbour); row.JoinedOwn || len(row.Net) == 0 {
		t.Fatalf("a visitor on the guest network: linked but not marked, got %+v", row)
	}
}

func TestADataFrameFromTheAccessPointLinksTheReceiver(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	confirmHomeAP(t, s, r, false)
	fromAP := rtap(cat(hdr(0x88, 0x02, neighbour, macAP, macAP), []byte{0, 0}, make([]byte, 10))) // FromDS: a1 receiver, a2 BSSID
	cycle(t, r, data(neighbour), fromAP)
	if row := rowOf(t, s, neighbour); row.OwnCycles != 1 {
		t.Errorf("one cycle on your network, got %d", row.OwnCycles)
	}
}

func TestAnRTSBetweenADeviceAndAKnownAccessPointLinksThem(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	confirmHomeAP(t, s, r, false)
	rts := rtap(cat([]byte{0xb4, 0, 0, 0}, macAP[:], neighbour[:]))
	cycle(t, r, beacon("home"), rts)
	if row := rowOf(t, s, neighbour); !row.JoinedOwn {
		t.Errorf("an RTS to a known access point is evidence: %+v", row)
	}
	// Between two unknown devices nothing is linked.
	s2, _ := openTest(t, 0)
	r2 := NewRecorder(fakeVault{}, s2)
	cycle(t, r2, rtap(cat([]byte{0xb4, 0, 0, 0}, macPhone[:], neighbour[:])))
	if row := rowOf(t, s2, neighbour); len(row.Net) != 0 {
		t.Error("no access point was known, so nothing should be linked")
	}
}

func TestRadiosOfOneRouterShareABoxAndDifferentMakersNever(t *testing.T) {
	base := MAC{0x04, 0xd9, 0xf5, 0x10, 0x20, 0x30}
	same := []MAC{base, {0x04, 0xd9, 0xf5, 0x10, 0x20, 0x34}, {0x06, 0xd9, 0xf5, 0x10, 0x20, 0x31}}
	other := []MAC{{0x04, 0xd9, 0xf5, 0x10, 0x20, 0x38}, {0x5c, 0xd9, 0xf5, 0x10, 0x20, 0x30}, {0x04, 0xd9, 0xf6, 0x10, 0x20, 0x30}}
	for _, m := range same {
		if boxKey(m) != boxKey(base) {
			t.Errorf("%s should share a box with %s", m, base)
		}
	}
	for _, m := range other {
		if boxKey(m) == boxKey(base) {
			t.Errorf("%s should not share a box with %s", m, base)
		}
	}
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	frames := [][]byte{}
	for _, m := range same {
		frames = append(frames, data(m)) // a device row for each radio
		frames = append(frames, rtap(cat(hdr(0x80, 0, macBroad, m, m), make([]byte, 12), ie(0, 'x'), ie(3, 6), ie(255, 35))))
	}
	cycle(t, r, frames...)
	box := rowOf(t, s, same[0]).Box
	if len(box) == 0 {
		t.Fatal("an access point should get a box")
	}
	for _, m := range same[1:] {
		if !bytes.Equal(rowOf(t, s, m).Box, box) {
			t.Errorf("%s should be in the same box", m)
		}
	}
}

func TestWPSModelIsKeptAndTheDeviceNameNeverReachesTheFile(t *testing.T) {
	s, p := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	el := wpsElement(wpsAttr(wpsManufacturer, []byte("Sky UK")), wpsAttr(wpsModelName, []byte("SR203")), wpsAttr(0x1011, []byte("AnnasHub")),
		wpsAttr(wpsPrimaryType, []byte{0, wpsNetwork}))
	ap := rtap(cat(hdr(0x80, 0, macBroad, neighbourAP, neighbourAP), make([]byte, 12), ie(0, []byte("Smiths")...), ie(3, 6), el))
	probe := rtap(cat(hdr(0x40, 0, macBroad, macPhone, macBroad), ie(0), ie(45, 0xef, 0x01), ie(221, 0x00, 0x17, 0xf2, 0x0a)))
	cycle(t, r, data(neighbourAP), ap, probe)
	row := rowOf(t, s, neighbourAP)
	if row.Model != "SR203" || row.WPSMaker != "Sky UK" || row.Category != wpsNetwork {
		t.Errorf("the access point's product: %+v", row)
	}
	phone := rowOf(t, s, macPhone)
	if phone.Hint != "Apple" || phone.Sig == 0 {
		t.Errorf("the phone's hint and signature: %+v", phone)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{p, p + "-wal"} {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, bad := range [][]byte{[]byte("AnnasHub"), []byte("Smiths"), neighbourAP[:], macPhone[:]} {
			if bytes.Contains(raw, bad) {
				t.Errorf("%s holds %q", filepath.Base(f), bad)
			}
		}
	}
}

func TestForgetNeighboursClearsLinksAndBoxes(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	if err := r.AddOwnerDevice(ctx, mineMAC, "mine", t0); err != nil {
		t.Fatal(err)
	}
	cycle(t, r, data(mineMAC), beacon("x"))
	if row := rowOf(t, s, mineMAC); len(row.Net) == 0 {
		t.Fatal("the owner's device should be linked before the forget")
	}
	if err := r.ForgetNeighbours(ctx); err != nil {
		t.Fatal(err)
	}
	var refs int
	if err := s.db.QueryRow(`SELECT count(*) FROM devices WHERE net IS NOT NULL OR box IS NOT NULL`).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 0 {
		t.Errorf("%d rows still point at a forgotten device", refs)
	}
}
