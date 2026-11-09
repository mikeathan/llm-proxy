// Package loginlog reads the failed logins to this computer from /var/log/btmp, the file glibc and sshd append a record to each time a
// password login fails. Each record is a fixed 384 bytes (utmp(5)); only the type, the time and the 16-byte binary source address are
// read. The user name, the terminal and the host text the record also holds are never read: they are written by whoever tried, so they can
// hold anything, including a password typed into the name field (Constitution I.10). The address is a number, not text, and what leaves
// this package is a time and a parsed address.
package loginlog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
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
	// addrOffset and addrSize are where ut_addr_v6, the source address in network byte order, sits: an IPv4 address fills the first four
	// bytes and leaves the rest zero; an IPv6 address fills all sixteen; all zero means no address was recorded.
	addrOffset = 348
	addrSize   = 16
	// MaxRecords is the most records one read looks at, newest first: about 1.5 MB, however large the file has grown.
	MaxRecords = 4096
)

// Attempt is one failed login: when, and from where when the record says. From is the zero address when none was recorded.
type Attempt struct {
	At   time.Time
	From netip.Addr
}

// Reader reads the failed logins from one file.
type Reader struct{ Path string }

// Attempts returns the failed logins after the given time, oldest first, looking back from the end of the file and
// stopping at the first record that is not newer, or after MaxRecords. A missing file is no failures (nothing was ever recorded); a file
// this program may not open is an error that satisfies errors.Is(err, fs.ErrPermission).
func (r Reader) Attempts(after time.Time) ([]Attempt, error) {
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
	var out []Attempt
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
			out = append(out, Attempt{At: at, From: source(buf[addrOffset : addrOffset+addrSize])})
		}
	}
	slices.Reverse(out)
	return out, nil
}

// source parses the binary address field. Anything that is not a plain address (all zero) is the zero address.
func source(raw []byte) netip.Addr {
	var b [addrSize]byte
	copy(b[:], raw)
	if b == [addrSize]byte{} {
		return netip.Addr{}
	}
	if [addrSize - 4]byte(b[4:]) == [addrSize - 4]byte{} {
		return netip.AddrFrom4([4]byte(b[:4]))
	}
	return netip.AddrFrom16(b)
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
