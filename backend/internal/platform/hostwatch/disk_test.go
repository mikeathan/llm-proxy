package hostwatch

import (
	"errors"
	"testing"
)

const gib = 1 << 30

func checkDisk(free, total uint64, err error) Hardening {
	return Hardener{DiskPaths: []string{"/"}, Space: func(string) (uint64, uint64, error) { return free, total, err }}.Check()
}

func TestDisk_PlentyOfRoomIsNotAFinding(t *testing.T) {
	if got := checkDisk(400*gib, 1000*gib, nil); got.DiskLow != 0 {
		t.Errorf("40%% free: %+v", got)
	}
	// A small volume with little in absolute terms but plenty in proportion is fine too.
	if got := checkDisk(30*gib, 100*gib, nil); got.DiskLow != 0 {
		t.Errorf("30%% free: %+v", got)
	}
}

func TestDisk_UnderTenPercentAndUnderFiftyGiBIsLow(t *testing.T) {
	if got := checkDisk(40*gib, 500*gib, nil); got.DiskLow != DiskLow {
		t.Errorf("8%% and 40 GiB free: %+v", got)
	}
	// A huge volume at 9% free still has 360 GiB: no finding.
	if got := checkDisk(360*gib, 4000*gib, nil); got.DiskLow != 0 {
		t.Errorf("9%% of 4 TB: %+v", got)
	}
}

func TestDisk_AlmostFullIsCritical(t *testing.T) {
	if got := checkDisk(1*gib, 500*gib, nil); got.DiskLow != DiskCritical {
		t.Errorf("1 GiB free: %+v", got)
	}
	if got := checkDisk(10*gib, 1000*gib, nil); got.DiskLow != DiskCritical {
		t.Errorf("1%% free: %+v", got)
	}
}

func TestDisk_TheWorstVolumeDecidesAndAnErrorIsCountedNotReported(t *testing.T) {
	h := Hardener{DiskPaths: []string{"/", "/home", "/gone"}, Space: func(p string) (uint64, uint64, error) {
		switch p {
		case "/home":
			return 1 * gib, 500 * gib, nil
		case "/gone":
			return 0, 0, errors.New("no such volume")
		}
		return 400 * gib, 500 * gib, nil
	}}
	got := h.Check()
	if got.DiskLow != DiskCritical || got.Unreadable != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestDisk_ARealVolumeIsReadable(t *testing.T) {
	free, total, err := volumeSpace(t.TempDir())
	if err != nil || total == 0 || free > total {
		t.Errorf("free %d total %d err %v", free, total, err)
	}
}
