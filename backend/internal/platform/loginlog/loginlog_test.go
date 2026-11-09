package loginlog

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"net/netip"
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

func TestAttempts_ReadsTheTimesOfFailedLoginsAfterATime(t *testing.T) {
	path := writeFile(t,
		record(failedLogin, base, "alice", "192.0.2.10"),
		record(failedLogin, base.Add(time.Minute), "bob", "192.0.2.11"),
		record(failedLogin, base.Add(2*time.Minute), "root", "192.0.2.12"),
	)
	got, err := Reader{Path: path}.Attempts(base.Add(30 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].At.Equal(base.Add(time.Minute)) || !got[1].At.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("times = %v, want the last two, oldest first", got)
	}
}

func TestAttempts_OnlyAFailedLoginRecordCounts(t *testing.T) {
	path := writeFile(t, record(7, base, "alice", "192.0.2.10"), record(failedLogin, base, "alice", "192.0.2.10"), record(0, base, "", ""))
	got, err := Reader{Path: path}.Attempts(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("times = %v err = %v, want one", got, err)
	}
}

func TestAttempts_AHalfWrittenRecordAtTheEndIsIgnored(t *testing.T) {
	path := writeFile(t, record(failedLogin, base, "a", "h"), record(failedLogin, base.Add(time.Minute), "a", "h")[:200])
	got, err := Reader{Path: path}.Attempts(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("times = %v err = %v, want only the whole record", got, err)
	}
}

// The result has nowhere to carry text: a time and a parsed address. The user name and the host text, which the sender controls, are never
// read, so an address written into them is not believed (Constitution I.10).
func TestAttempts_NeverReadsTheUserNameOrTheHostText(t *testing.T) {
	path := writeFile(t, record(failedLogin, base, "hostile\nname", "198.51.100.99"))
	got, err := Reader{Path: path}.Attempts(base.Add(-time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("attempts = %v err = %v", got, err)
	}
	if got[0].From.IsValid() {
		t.Errorf("the host text must not become the source: %v", got[0].From)
	}
}

func withAddr(rec []byte, raw ...byte) []byte {
	copy(rec[addrOffset:addrOffset+addrSize], make([]byte, addrSize))
	copy(rec[addrOffset:], raw)
	return rec
}

// The source is the 16 binary bytes sshd stores: an IPv4 address in the first four, an IPv6 address in all sixteen, none when all are zero.
func TestAttempts_TheSourceIsTheBinaryAddressField(t *testing.T) {
	v6 := netip.MustParseAddr("2001:db8::7").As16()
	path := writeFile(t,
		withAddr(record(failedLogin, base, "a", ""), 203, 0, 113, 9),
		withAddr(record(failedLogin, base.Add(time.Second), "a", ""), v6[:]...),
		record(failedLogin, base.Add(2*time.Second), "a", ""),
	)
	got, err := Reader{Path: path}.Attempts(base.Add(-time.Hour))
	if err != nil || len(got) != 3 {
		t.Fatalf("attempts = %v err = %v", got, err)
	}
	if got[0].From != netip.MustParseAddr("203.0.113.9") || got[1].From != netip.MustParseAddr("2001:db8::7") || got[2].From.IsValid() {
		t.Errorf("sources = %v %v %v", got[0].From, got[1].From, got[2].From)
	}
}

func TestAttempts_ReadsAtMostTheLastFewRecordsOfAHugeFile(t *testing.T) {
	var recs [][]byte
	for i := 0; i < MaxRecords+500; i++ {
		recs = append(recs, record(failedLogin, base.Add(time.Duration(i)*time.Second), "a", "h"))
	}
	got, err := Reader{Path: writeFile(t, recs...)}.Attempts(base.Add(-time.Hour))
	if err != nil || len(got) != MaxRecords {
		t.Fatalf("%d times (err %v), want the bound of %d", len(got), err, MaxRecords)
	}
	if !got[len(got)-1].At.Equal(base.Add(time.Duration(MaxRecords+499) * time.Second)) {
		t.Fatalf("the newest record was not the last returned: %v", got[len(got)-1])
	}
}

func TestAttempts_StopsAtTheFirstRecordOlderThanTheTime(t *testing.T) {
	var recs [][]byte
	for i := 0; i < 100; i++ {
		recs = append(recs, record(failedLogin, base.Add(time.Duration(i)*time.Hour), "a", "h"))
	}
	got, err := Reader{Path: writeFile(t, recs...)}.Attempts(base.Add(97 * time.Hour))
	if err != nil || len(got) != 2 {
		t.Fatalf("times = %d err = %v, want the two after hour 97", len(got), err)
	}
}

func TestAttempts_AMissingFileIsNoFailuresAndAnUnreadableOneIsATypedError(t *testing.T) {
	got, err := Reader{Path: filepath.Join(t.TempDir(), "none")}.Attempts(base)
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
	if _, err := (Reader{Path: path}).Attempts(base); !errors.Is(err, fs.ErrPermission) {
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
