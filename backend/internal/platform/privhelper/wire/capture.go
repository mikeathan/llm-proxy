package wire

import (
	"encoding/binary"
	"errors"
)

// The framing of the capture_window action's output (Constitution I.11: a capture keeps at most 64 bytes of each frame). The helper's
// capture program writes fixed-size records and one trailer; it parses nothing, so a frame is never read in the privileged process. The
// service decodes the records and reads the headers in its own, unprivileged process. This file is only the byte layout both sides share.
const (
	// CaptureCommand is the first argument that makes the helper binary run the capture program instead of serving a connection.
	CaptureCommand = "capture"
	// CaptureSnap is how many bytes of a frame are kept.
	CaptureSnap = 64
	// CaptureRecordSize is one record: 4 bytes of milliseconds since the window began, 2 bytes of kept length, 2 reserved, then the frame bytes
	// zero-padded to CaptureSnap.
	CaptureRecordSize = 8 + CaptureSnap
	// CaptureTrailerSize is the trailer that ends the output.
	CaptureTrailerSize = 32
	// CaptureMaxFrames is the most records one window keeps.
	CaptureMaxFrames = 4096
	// CaptureMaxSeconds is the longest window.
	CaptureMaxSeconds = 60
	// CaptureMaxOutput is the largest output a window can produce: the records and the trailer.
	CaptureMaxOutput = CaptureMaxFrames*CaptureRecordSize + CaptureTrailerSize
)

// captureMagic opens the trailer.
var captureMagic = [4]byte{'C', 'W', 'T', '1'}

// CaptureStop is why a window ended.
type CaptureStop uint8

// The reasons. Everything but StopWindow and StopFrameCap means no frame was read.
const (
	// StopWindow: the time was up.
	StopWindow CaptureStop = iota
	// StopFrameCap: the frame limit was reached before the time.
	StopFrameCap
	// StopNoInterface: the interface does not exist.
	StopNoInterface
	// StopInterfaceDown: the interface exists and is not up.
	StopInterfaceDown
	// StopSocketError: the packet socket could not be opened, filtered or bound.
	StopSocketError
	// StopUnsupported: this host has no packet socket (not Linux).
	StopUnsupported
	// StopCut is not written by the helper: the decoder reports it when the output ended without a trailer (the run was cut).
	StopCut
)

// CaptureRecord is one kept frame: when it arrived, and its first bytes.
type CaptureRecord struct {
	OffsetMs uint32
	Frame    []byte
}

// CaptureTrailer is what the window says about itself.
type CaptureTrailer struct {
	Stop CaptureStop
	// Frames is how many records were written.
	Frames uint32
	// KernelSeen and KernelDropped are the packet socket's own counters: frames that passed the filter, and frames the kernel dropped because
	// the program did not read fast enough.
	KernelSeen, KernelDropped uint32
	// ElapsedMs is how long the window ran.
	ElapsedMs uint32
}

// ErrCaptureFormat means the bytes are not a capture output.
var ErrCaptureFormat = errors.New("not a capture_window output")

// AppendCaptureRecord appends one record. A frame longer than CaptureSnap is cut to it.
func AppendCaptureRecord(dst []byte, offsetMs uint32, frame []byte) []byte {
	frame = frame[:min(len(frame), CaptureSnap)]
	var rec [CaptureRecordSize]byte
	binary.BigEndian.PutUint32(rec[0:4], offsetMs)
	binary.BigEndian.PutUint16(rec[4:6], uint16(len(frame)))
	copy(rec[8:], frame)
	return append(dst, rec[:]...)
}

// AppendCaptureTrailer appends the trailer.
func AppendCaptureTrailer(dst []byte, t CaptureTrailer) []byte {
	var tr [CaptureTrailerSize]byte
	copy(tr[0:4], captureMagic[:])
	tr[4] = byte(t.Stop)
	binary.BigEndian.PutUint32(tr[8:12], t.Frames)
	binary.BigEndian.PutUint32(tr[12:16], t.KernelSeen)
	binary.BigEndian.PutUint32(tr[16:20], t.KernelDropped)
	binary.BigEndian.PutUint32(tr[20:24], t.ElapsedMs)
	return append(dst, tr[:]...)
}

// DecodeCapture reads an output into its records and trailer. An output with no trailer (the run was cut) gives the whole records it holds
// and StopCut. Output that is not a multiple of the record size before the trailer, or whose records disagree with the trailer, is an error.
func DecodeCapture(data []byte) ([]CaptureRecord, CaptureTrailer, error) {
	var tr CaptureTrailer
	body := data
	if n := len(data); n >= CaptureTrailerSize && [4]byte(data[n-CaptureTrailerSize:n-CaptureTrailerSize+4]) == captureMagic {
		t := data[n-CaptureTrailerSize:]
		tr = CaptureTrailer{
			Stop: CaptureStop(t[4]), Frames: binary.BigEndian.Uint32(t[8:12]), KernelSeen: binary.BigEndian.Uint32(t[12:16]),
			KernelDropped: binary.BigEndian.Uint32(t[16:20]), ElapsedMs: binary.BigEndian.Uint32(t[20:24]),
		}
		body = data[:n-CaptureTrailerSize]
		if len(body)%CaptureRecordSize != 0 || uint32(len(body)/CaptureRecordSize) != tr.Frames || tr.Stop > StopUnsupported {
			return nil, CaptureTrailer{}, ErrCaptureFormat
		}
	} else {
		body = data[:len(data)/CaptureRecordSize*CaptureRecordSize]
		tr = CaptureTrailer{Stop: StopCut, Frames: uint32(len(body) / CaptureRecordSize)}
	}
	if len(body)/CaptureRecordSize > CaptureMaxFrames {
		return nil, CaptureTrailer{}, ErrCaptureFormat
	}
	records := make([]CaptureRecord, 0, len(body)/CaptureRecordSize)
	for off := 0; off < len(body); off += CaptureRecordSize {
		rec := body[off : off+CaptureRecordSize]
		n := int(binary.BigEndian.Uint16(rec[4:6]))
		if n > CaptureSnap {
			return nil, CaptureTrailer{}, ErrCaptureFormat
		}
		records = append(records, CaptureRecord{OffsetMs: binary.BigEndian.Uint32(rec[0:4]), Frame: append([]byte(nil), rec[8:8+n]...)})
	}
	return records, tr, nil
}
