// Package dnswire reads the parts of a DNS message that mDNS answers use. It is the one reader of DNS wire format in the
// program: everything in a message comes from a device, so every length, count and compression pointer is bounded and a
// malformed message is an error, never a panic or an unbounded allocation.
package dnswire

import (
	"encoding/binary"
	"errors"
	"strings"
)

// Sizes and record types.
const (
	HeaderLen   = 12
	TypeA       = 1
	TypePTR     = 12
	TypeTXT     = 16
	TypeSRV     = 33
	ClassIN     = 1
	MaxWireName = 255 // the DNS limit for a whole name
	MaxPointers = 8
	// MaxRecords bounds how many records one message is read for, whatever its header claims.
	MaxRecords = 128

	responseFlag = 0x8000
)

// ErrMalformed means the bytes are not a usable DNS message.
var ErrMalformed = errors.New("not a usable DNS message")

// Header is the fixed start of a message.
type Header struct {
	ID, Flags                       uint16
	Questions, Answers, Auth, Extra uint16
}

// ParseHeader reads the first twelve bytes.
func ParseHeader(raw []byte) (Header, error) {
	if len(raw) < HeaderLen {
		return Header{}, ErrMalformed
	}
	be := binary.BigEndian
	return Header{be.Uint16(raw[0:]), be.Uint16(raw[2:]), be.Uint16(raw[4:]), be.Uint16(raw[6:]), be.Uint16(raw[8:]), be.Uint16(raw[10:])}, nil
}

// IsSuccessfulResponse is true for a response whose result code is "no error".
func (h Header) IsSuccessfulResponse() bool { return h.Flags&responseFlag != 0 && h.Flags&0x000f == 0 }

// Record is one resource record's header: who it is about, what it is, and where its data starts and how long it is.
type Record struct {
	Owner       string
	Type, Class uint16
	Data        int
	Length      int
}

// SkipQuestions returns the offset of the first answer.
func SkipQuestions(raw []byte, n int) (int, error) {
	off := HeaderLen
	for range min(n, MaxRecords) {
		_, next, err := ReadName(raw, off)
		if err != nil || next+4 > len(raw) {
			return 0, ErrMalformed
		}
		off = next + 4
	}
	return off, nil
}

// ReadRecord reads the record header at off; its data is checked to lie inside the message.
func ReadRecord(raw []byte, off int) (Record, error) {
	owner, next, err := ReadName(raw, off)
	if err != nil || next+10 > len(raw) {
		return Record{}, ErrMalformed
	}
	rec := Record{
		Owner: owner, Type: binary.BigEndian.Uint16(raw[next:]), Class: binary.BigEndian.Uint16(raw[next+2:]) & 0x7fff,
		Data: next + 10, Length: int(binary.BigEndian.Uint16(raw[next+8:])),
	}
	if rec.Data+rec.Length > len(raw) {
		return Record{}, ErrMalformed
	}
	return rec, nil
}

// Records reads every answer, authority and additional record of a response, at most MaxRecords of them. A message that
// breaks off part way returns what was read before it together with the error.
func Records(raw []byte) ([]Record, error) {
	h, err := ParseHeader(raw)
	if err != nil {
		return nil, err
	}
	off, err := SkipQuestions(raw, int(h.Questions))
	if err != nil {
		return nil, err
	}
	total := int(h.Answers) + int(h.Auth) + int(h.Extra)
	var out []Record
	for range min(total, MaxRecords) {
		rec, err := ReadRecord(raw, off)
		if err != nil {
			return out, err
		}
		out = append(out, rec)
		off = rec.Data + rec.Length
	}
	return out, nil
}

// ReadName reads a possibly compressed name starting at off and returns it with the offset just after it in the message.
// A pointer may only go backwards and only a few times, so a crafted loop ends in an error.
func ReadName(msg []byte, off int) (string, int, error) {
	var labels []string
	next, hops, total := 0, 0, 0
	for {
		if off >= len(msg) {
			return "", 0, ErrMalformed
		}
		n := int(msg[off])
		switch {
		case n == 0:
			if hops == 0 {
				next = off + 1
			}
			return strings.Join(labels, "."), next, nil
		case n&0xc0 == 0xc0:
			target, ok := pointerTarget(msg, off, hops)
			if !ok {
				return "", 0, ErrMalformed
			}
			if hops == 0 {
				next = off + 2
			}
			off, hops = target, hops+1
		case n&0xc0 != 0 || off+1+n > len(msg):
			return "", 0, ErrMalformed
		default:
			if total += n + 1; total > MaxWireName {
				return "", 0, ErrMalformed
			}
			labels = append(labels, string(msg[off+1:off+1+n]))
			off += 1 + n
		}
	}
}

func pointerTarget(msg []byte, off, hops int) (int, bool) {
	if off+1 >= len(msg) || hops >= MaxPointers {
		return 0, false
	}
	target := int(msg[off]&0x3f)<<8 | int(msg[off+1])
	return target, target < off
}
