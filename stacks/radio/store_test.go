package radio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func day(n int) time.Time { return t0.AddDate(0, 0, n) }

func openTest(t *testing.T, capBytes int64) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "radio.db")
	s, err := OpenStore(p, capBytes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, p
}

func tag(i int) []byte {
	h := sha256.Sum256(binary.LittleEndian.AppendUint64(nil, uint64(i)))
	return h[:]
}

func TestStoreDeviceVisitsAndSignal(t *testing.T) {
	s, _ := openTest(t, 0)
	ctx := context.Background()
	for _, c := range []struct {
		at  time.Time
		sig int
	}{{t0, -60}, {t0.Add(time.Hour), -40}, {day(1), -75}} {
		if err := s.SeenDevice(ctx, Seen{Tag: tag(1), Signal: c.sig, SignalOK: true, At: c.at}); err != nil {
			t.Fatal(err)
		}
	}
	d, ok, err := s.Device(ctx, tag(1))
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if d.Visits != 2 || d.SignalMin != -75 || d.SignalMax != -40 || d.FirstDay != dayNumber(t0) || d.LastDay != dayNumber(day(1)) {
		t.Errorf("%+v", d)
	}
}

func TestStoreOtherPeoplesRowsCarryNoAddressOrLabel(t *testing.T) {
	s, _ := openTest(t, 0)
	err := s.SeenDevice(context.Background(), Seen{Tag: tag(1), Sealed: []byte("x"), At: t0})
	if err == nil {
		t.Error("a sealed address on a row that is not the owner's must be refused")
	}
}

func TestStoreRetention(t *testing.T) {
	s, _ := openTest(t, 0)
	ctx := context.Background()
	_ = s.SeenDevice(ctx, Seen{Tag: tag(1), At: t0})                                                        // neighbour
	_ = s.SeenDevice(ctx, Seen{Tag: tag(2), Owner: true, Sealed: []byte("sealed"), Label: "phone", At: t0}) // owner's
	_ = s.SeenBeacon(ctx, Beacon{Tag: tag(3), Security: "wpa2", Channel: 1, At: t0})
	_ = s.SeenBeacon(ctx, Beacon{Tag: tag(4), Own: true, Name: "home", Security: "wpa2", Channel: 1, At: t0})
	_ = s.AddEvent(ctx, "deauth", tag(1), false, t0)
	_ = s.AddEvent(ctx, "deauth", tag(2), true, t0)

	if n, err := s.Prune(ctx, day(13)); err != nil || n != 0 {
		t.Fatalf("day 13 removed %d %v", n, err)
	}
	if n, err := s.Prune(ctx, day(15)); err != nil || n != 3 { // neighbour device, neighbour beacon, neighbour events
		t.Fatalf("day 15 removed %d %v", n, err)
	}
	if _, ok, _ := s.Device(ctx, tag(1)); ok {
		t.Error("a neighbour is kept 14 days")
	}
	if _, ok, _ := s.Device(ctx, tag(2)); !ok {
		t.Error("the owner's device is kept until deleted")
	}
	if n, _ := s.Prune(ctx, day(31)); n != 1 { // the owner's event, 30 days
		t.Errorf("day 31 removed %d, want the owner's event only", n)
	}
	if c, _ := s.count("beacons"); c != 1 {
		t.Errorf("the owner's own access point is kept, beacons=%d", c)
	}
}

func TestStoreForgetOthersKeepsTheOwners(t *testing.T) {
	s, _ := openTest(t, 0)
	ctx := context.Background()
	_ = s.SeenDevice(ctx, Seen{Tag: tag(1), At: t0})
	_ = s.SeenDevice(ctx, Seen{Tag: tag(2), Owner: true, Sealed: []byte("s"), At: t0})
	_ = s.AddEvent(ctx, "deauth", nil, false, t0)
	if err := s.ForgetOthers(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.count("devices"); c != 1 {
		t.Errorf("devices=%d", c)
	}
	if c, _ := s.count("events"); c != 0 {
		t.Errorf("events=%d", c)
	}
}

// The store cannot fill the disk: a flood of devices stops at the cap, the lowest-value rows go first, and the owner's rows stay.
func TestStoreFloodStaysUnderTheCap(t *testing.T) {
	const capBytes = 1 << 20
	s, p := openTest(t, capBytes)
	ctx := context.Background()
	_ = s.SeenDevice(ctx, Seen{Tag: tag(-1), Owner: true, Sealed: []byte("sealed"), Label: "mine", At: t0})
	for i := 0; i < 40000; i++ {
		if err := s.SeenDevice(ctx, Seen{Tag: tag(i), Signal: -50, SignalOK: true, At: t0.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if _, ok, _ := s.Device(ctx, tag(-1)); !ok {
		t.Error("the owner's device was evicted")
	}
	if c, _ := s.count("devices"); c >= 40000 {
		t.Errorf("nothing was evicted: %d rows", c)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() > capBytes {
		t.Errorf("file %v bytes, cap %d, %v", fi.Size(), capBytes, err)
	}
}

func TestStoreOwnersAloneOverTheCapIsAnError(t *testing.T) {
	s, _ := openTest(t, 64<<10)
	var err error
	for i := 0; i < 5000 && err == nil; i++ {
		err = s.SeenDevice(context.Background(), Seen{Tag: tag(i), Owner: true, Sealed: bytes.Repeat([]byte{1}, 100), At: t0})
	}
	if err == nil {
		t.Error("owner rows are never evicted, so the cap must refuse them")
	}
}

func TestStoreStorageUse(t *testing.T) {
	s, _ := openTest(t, 1<<20)
	ctx := context.Background()
	_ = s.SeenDevice(ctx, Seen{Tag: tag(1), At: t0})
	use, err := s.StorageUse(ctx)
	if err != nil || len(use) != 1 || use[0].File != "radio.db" || use[0].Rows != 1 || use[0].CapBytes != 1<<20 || use[0].Bytes <= 0 {
		t.Fatalf("%+v %v", use, err)
	}
	if !use[0].Oldest.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) || use[0].KeepFor != OthersKeep {
		t.Errorf("%+v", use[0])
	}
}
