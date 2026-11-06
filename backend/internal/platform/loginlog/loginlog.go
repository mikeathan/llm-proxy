// Package loginlog counts the failed logins to this computer from /var/log/btmp, the file glibc and sshd append a record to each time a
// password login fails. Each record is a fixed 384 bytes (utmp(5)); only the type and the time are read. The user name and the host
// the record also holds are never read, so nothing about who tried can leave this package (Constitution I.10, proposed).
package loginlog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"time"
)

const (
	// DefaultPath is where the system keeps failed logins.
	DefaultPath = "/var/log/btmp"
	// recordSize is the size of one utmp record on 64-bit Linux, which the owner's real file (vertex, 2026-11-06) confirms.
	recordSize = 384
	// failedLogin is ut_type LOGIN_PROCESS, the type btmp records have.
	failedLogin = 6
	// timeOffset is where ut_tv.tv_sec, a little-endian int32, sits in a record.
	timeOffset = 340
	// MaxRecords is the most records one read looks at, newest first: about 1.5 MB, however large the file has grown.
	MaxRecords = 4096
)

// Reader reads the times of failed logins from one file.
type Reader struct{ Path string }

// Failures returns the times of the failed logins after the given time, oldest first, looking back from the end of the file and
// stopping at the first record that is not newer, or after MaxRecords. A missing file is no failures (nothing was ever recorded); a file
// this program may not open is an error that satisfies errors.Is(err, fs.ErrPermission).
func (r Reader) Failures(after time.Time) ([]time.Time, error) {
	f, err := os.Open(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open the record of failed logins: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to read the size of the record of failed logins: %w", err)
	}
	whole := info.Size() / recordSize // a half-written record at the end is left for the next read
	var out []time.Time
	buf := make([]byte, recordSize)
	for i, n := whole-1, 0; i >= 0 && n < MaxRecords; i, n = i-1, n+1 {
		if _, err := f.ReadAt(buf, i*recordSize); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("failed to read the record of failed logins: %w", err)
		}
		at := time.Unix(int64(int32(binary.LittleEndian.Uint32(buf[timeOffset:]))), 0).UTC()
		if !at.After(after) {
			break
		}
		if binary.LittleEndian.Uint32(buf[0:]) == failedLogin {
			out = append(out, at)
		}
	}
	slices.Reverse(out)
	return out, nil
}

// Readable says whether the file can be opened: nil when it can, or when it does not exist, and the error otherwise.
func (r Reader) Readable() error {
	f, err := os.Open(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to open the record of failed logins: %w", err)
	}
	return f.Close()
}
