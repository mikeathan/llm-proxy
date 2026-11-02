package hostwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var patchClock = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// patchDirs builds a look-alike of /var/run and /var/lib/update-notifier. A nil body leaves that file out.
func patchDirs(t *testing.T, reboot, updates *string, rebootAge time.Duration) Patcher {
	t.Helper()
	root := t.TempDir()
	p := Patcher{RebootFile: filepath.Join(root, "reboot-required"), UpdatesFile: filepath.Join(root, "updates-available"), Now: func() time.Time { return patchClock }}
	if reboot != nil {
		write(t, p.RebootFile, *reboot)
		when := patchClock.Add(-rebootAge)
		if err := os.Chtimes(p.RebootFile, when, when); err != nil {
			t.Fatal(err)
		}
	}
	if updates != nil {
		write(t, p.UpdatesFile, *updates)
	}
	return p
}

func realUpdates(t *testing.T) *string {
	t.Helper()
	b, err := os.ReadFile("testdata/updates_available_ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	return &s
}

func TestPatches_RealVertexCapture(t *testing.T) {
	// 28 Oct 16:19 pending, looked at on 2 Nov: five days. The text is the server's own file of 1 Nov.
	reboot := "*** System restart required ***\n"
	got := patchDirs(t, &reboot, realUpdates(t), 5*24*time.Hour).Check()
	if !got.RebootRequired || got.RebootDays != 5 {
		t.Fatalf("reboot = %v for %d days, want pending for 5", got.RebootRequired, got.RebootDays)
	}
	if !got.UpdatesRead || got.Updates != 12 || got.SecurityUpdates != 0 || got.ESMUpdates != 29 {
		t.Fatalf("updates = %+v, want 12 / 0 security / 29 ESM", got)
	}
}

func TestPatches_NothingPendingIsQuiet(t *testing.T) {
	none := "\n0 updates can be applied immediately.\n"
	got := patchDirs(t, nil, &none, 0).Check()
	if got.RebootRequired || got.Updates != 0 || !got.UpdatesRead {
		t.Fatalf("got %+v, want nothing pending and the file read", got)
	}
}

func TestPatches_MissingFilesAreNotReadAndNotClean(t *testing.T) {
	got := patchDirs(t, nil, nil, 0).Check()
	if got.UpdatesRead {
		t.Fatal("a host without the file must not be reported as up to date: UpdatesRead has to stay false")
	}
}

func TestPatches_SecurityLineIsRead(t *testing.T) {
	body := "14 updates can be applied immediately.\n3 of these updates are standard security updates.\n"
	if got := patchDirs(t, nil, &body, 0).Check(); got.Updates != 14 || got.SecurityUpdates != 3 {
		t.Fatalf("got %+v, want 14 updates, 3 security", got)
	}
}

func TestPatches_OneUpdateSingular(t *testing.T) {
	body := "1 update can be applied immediately.\n"
	if got := patchDirs(t, nil, &body, 0).Check(); got.Updates != 1 {
		t.Fatalf("got %+v, want 1", got)
	}
}

func TestPatches_HugeOrGarbageNumbersAreIgnored(t *testing.T) {
	body := "99999999999999999999 updates can be applied immediately.\n" + string(make([]byte, 2<<20))
	if got := patchDirs(t, nil, &body, 0).Check(); got.Updates != 0 {
		t.Fatalf("got %+v, want the unreadable number ignored", got)
	}
}

func autoRun(t *testing.T, file string, now time.Time) Patches {
	t.Helper()
	return Patcher{AutoUpgradeLog: file, Zone: time.UTC, Now: func() time.Time { return now }}.Check()
}

func TestAutoRun_LastStartOfTheRealLogIsRead(t *testing.T) {
	// The capture is vertex's own log (2026-11-02); its last run started at 15:13 on 2 November.
	same := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	if got := autoRun(t, "testdata/unattended_upgrades_log_ubuntu", same); !got.AutoRunRead || got.AutoRunDays != 0 {
		t.Errorf("same day: %+v", got)
	}
	later := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	if got := autoRun(t, "testdata/unattended_upgrades_log_ubuntu", later); !got.AutoRunRead || got.AutoRunDays != 10 {
		t.Errorf("ten days on: %+v", got)
	}
}

func TestAutoRun_NothingReadIsNothingKnown(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "empty.log"), "")
	write(t, filepath.Join(dir, "other.log"), "2026-11-01 06:02:54,972 INFO No packages found\n")
	for name, file := range map[string]string{"missing": filepath.Join(dir, "none.log"), "empty": filepath.Join(dir, "empty.log"), "no start line": filepath.Join(dir, "other.log")} {
		got := autoRun(t, file, now)
		if got.AutoRunRead || got.Unreadable != 0 {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestAutoRun_OnlyTheEndOfALongLogIsRead(t *testing.T) {
	dir := t.TempDir()
	long := "2026-01-01 06:00:00,000 INFO Starting unattended upgrades script\n" + strings.Repeat("2026-10-01 06:00:00,000 INFO filler line of some length to fill the file up\n", 8000) +
		"2026-11-05 06:00:00,000 INFO Starting unattended upgrades script\n"
	write(t, filepath.Join(dir, "big.log"), long)
	if got := autoRun(t, filepath.Join(dir, "big.log"), time.Date(2026, 11, 7, 6, 0, 0, 0, time.UTC)); !got.AutoRunRead || got.AutoRunDays != 2 {
		t.Errorf("got %+v", got)
	}
}
