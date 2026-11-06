package loginlog

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var base = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

// record builds one 384-byte utmp record the way glibc writes a failed login to btmp (utmp(5)): a type, a pid, the terminal, the user
// and the host as text, and the time as two 32-bit numbers. The user and host here are invented.
func record(kind int32, at time.Time, user, host string) []byte {
	b := make([]byte, recordSize)
	binary.LittleEndian.PutUint32(b[0:], uint32(kind))
	binary.LittleEndian.PutUint32(b[4:], 4242)
	copy(b[8:], "ssh:notty")
	copy(b[44:], user)
	copy(b[76:], host)
	binary.LittleEndian.PutUint32(b[timeOffset:], uint32(at.Unix()))
	return b
}

func writeFile(t *testing.T, records ...[]byte) string {
	t.Helper()
	var all []byte
	for _, r := range records {
		all = append(all, r...)
	}
	path := filepath.Join(t.TempDir(), "btmp")
	if err := os.WriteFile(path, all, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFailures_ReadsTheTimesOfFailedLoginsAfterATime(t *testing.T) {
	path := writeFile(t,
		record(failedLogin, base, "alice", "192.0.2.10"),
		record(failedLogin, base.Add(time.Minute), "bob", "192.0.2.11"),
		record(failedLogin, base.Add(2*time.Minute), "root", "192.0.2.12"),
	)
	got, err := Reader{Path: path}.Failures(base.Add(30 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Equal(base.Add(time.Minute)) || !got[1].Equal(base.Add(2*time.Minute)) {
		t.Fatalf("times = %v, want the last two, oldest first", got)
	}
}

func TestFailures_OnlyAFailedLoginRecordCounts(t *testing.T) {
	path := writeFile(t, record(7, base, "alice", "192.0.2.10"), record(failedLogin, base, "alice", "192.0.2.10"), record(0, base, "", ""))
	got, err := Reader{Path: path}.Failures(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("times = %v err = %v, want one", got, err)
	}
}

func TestFailures_AHalfWrittenRecordAtTheEndIsIgnored(t *testing.T) {
	path := writeFile(t, record(failedLogin, base, "a", "h"), record(failedLogin, base.Add(time.Minute), "a", "h")[:200])
	got, err := Reader{Path: path}.Failures(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("times = %v err = %v, want only the whole record", got, err)
	}
}

func TestFailures_NeverReturnsAUserNameOrAnAddress(t *testing.T) {
	// The reader's result is only times: the type has nowhere to carry text, which is the point (Constitution I.10, proposed).
	path := writeFile(t, record(failedLogin, base, "hostile\nname", "192.0.2.99"))
	got, err := Reader{Path: path}.Failures(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("times = %v err = %v", got, err)
	}
}

func TestFailures_ReadsAtMostTheLastFewRecordsOfAHugeFile(t *testing.T) {
	var recs [][]byte
	for i := 0; i < MaxRecords+500; i++ {
		recs = append(recs, record(failedLogin, base.Add(time.Duration(i)*time.Second), "a", "h"))
	}
	got, err := Reader{Path: writeFile(t, recs...)}.Failures(base.Add(-time.Hour))
	if err != nil || len(got) != MaxRecords {
		t.Fatalf("%d times (err %v), want the bound of %d", len(got), err, MaxRecords)
	}
	if !got[len(got)-1].Equal(base.Add(time.Duration(MaxRecords+499) * time.Second)) {
		t.Fatalf("the newest record was not the last returned: %v", got[len(got)-1])
	}
}

func TestFailures_StopsAtTheFirstRecordOlderThanTheTime(t *testing.T) {
	var recs [][]byte
	for i := 0; i < 100; i++ {
		recs = append(recs, record(failedLogin, base.Add(time.Duration(i)*time.Hour), "a", "h"))
	}
	got, err := Reader{Path: writeFile(t, recs...)}.Failures(base.Add(97 * time.Hour))
	if err != nil || len(got) != 2 {
		t.Fatalf("times = %d err = %v, want the two after hour 97", len(got), err)
	}
}

func TestFailures_AMissingFileIsNoFailuresAndAnUnreadableOneIsATypedError(t *testing.T) {
	got, err := Reader{Path: filepath.Join(t.TempDir(), "none")}.Failures(base)
	if err != nil || len(got) != 0 {
		t.Fatalf("a missing file: %v %v, want nothing and no error (no failure was ever recorded)", got, err)
	}
	path := writeFile(t, record(failedLogin, base, "a", "h"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	if _, err := (Reader{Path: path}).Failures(base); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("err = %v, want a permission error the caller can explain", err)
	}
}

func TestReadable_SaysWhetherTheFileCanBeOpened(t *testing.T) {
	if err := (Reader{Path: writeFile(t)}).Readable(); err != nil {
		t.Fatalf("an empty readable file: %v", err)
	}
	if err := (Reader{Path: filepath.Join(t.TempDir(), "none")}).Readable(); err != nil {
		t.Fatalf("a missing file is no failures: %v", err)
	}
}
