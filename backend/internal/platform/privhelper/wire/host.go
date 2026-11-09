package wire

import (
	"encoding/binary"
	"errors"
)

// The framing of the capture_host action's output (Constitution I.13: a look at this computer's own traffic). It is a different layout from
// capture_window's on purpose: that one promises 64 bytes of a few kinds of frame and its tests pin it, this one keeps a little more of a
// few kinds of frame (a DNS message holds names) and so has its own limits. The helper's capture program writes variable-length records and
// one trailer; it parses nothing, so a frame is never read in the privileged process. The service decodes the records and reads the
// headers in its own, unprivileged process. This file is only the byte layout both sides share.
const (
	// HostCommand is the first argument that makes the helper binary run the host capture program.
	HostCommand = "capture-host"
	// HostSnapHeaders is the most bytes an ordinary frame keeps: the Ethernet header, the IP header (at most 60 bytes with every IPv4 option)
	// and the first four bytes after it, which are the ports. That is the addresses, the ports and the size, and no payload byte; a typical
	// IPv4 frame keeps 38 bytes and an IPv6 frame 58.
	HostSnapHeaders = 14 + 60 + 4
	// HostSnapMax is the most bytes of any frame that are kept (a DNS message). It bounds a record.
	HostSnapMax = 512
	// HostRecordHeader is the start of a record: 4 bytes of milliseconds since the window began, 2 bytes of kept length, 2 bytes of the
	// sampling rate in force (1 in N of the ordinary frames was kept; 1 means all of them).
	HostRecordHeader = 8
	// HostTrailerSize is the trailer that ends the output.
	HostTrailerSize = 32
	// HostMaxFrames is the most records one window keeps.
	HostMaxFrames = 60000
	// HostMaxOutput is the most bytes one window can write: 8 MiB, the records and the trailer.
	HostMaxOutput = 8 << 20
	// HostMaxSeconds is the longest window.
	HostMaxSeconds = 60
	// HostMaxSample is the coarsest sampling the program may reach: 1 in 1,024 ordinary frames.
	HostMaxSample = 1024
)

// hostMagic opens the trailer.
var hostMagic = [4]byte{'H', 'T', 'T', '1'}

// HostRecord is one kept frame: when it arrived, the sampling rate when it did, and its first bytes.
type HostRecord struct {
	OffsetMs uint32
	Sample   uint16
	Frame    []byte
}

// HostTrailer is what the window says about itself.
type HostTrailer struct {
	Stop CaptureStop
	// Frames is how many records were written.
	Frames uint32
	// KernelSeen and KernelDropped are the packet socket's own counters: frames that passed the filter, and frames the kernel dropped
	// because the program did not read fast enough.
	KernelSeen, KernelDropped uint32
	// ElapsedMs is how long the window ran.
	ElapsedMs uint32
	// Sample is the sampling rate at the end of the window.
	Sample uint16
}

// ErrHostCaptureFormat means the bytes are not a capture_host output.
var ErrHostCaptureFormat = errors.New("not a capture_host output")

// AppendHostRecord appends one record. A frame longer than HostSnapMax is cut to it.
func AppendHostRecord(dst []byte, offsetMs uint32, sample uint16, frame []byte) []byte {
	frame = frame[:min(len(frame), HostSnapMax)]
	var head [HostRecordHeader]byte
	binary.BigEndian.PutUint32(head[0:4], offsetMs)
	binary.BigEndian.PutUint16(head[4:6], uint16(len(frame)))
	binary.BigEndian.PutUint16(head[6:8], sample)
	return append(append(dst, head[:]...), frame...)
}

// AppendHostTrailer appends the trailer.
func AppendHostTrailer(dst []byte, t HostTrailer) []byte {
	var tr [HostTrailerSize]byte
	copy(tr[0:4], hostMagic[:])
	tr[4] = byte(t.Stop)
	binary.BigEndian.PutUint32(tr[8:12], t.Frames)
	binary.BigEndian.PutUint32(tr[12:16], t.KernelSeen)
	binary.BigEndian.PutUint32(tr[16:20], t.KernelDropped)
	binary.BigEndian.PutUint32(tr[20:24], t.ElapsedMs)
	binary.BigEndian.PutUint16(tr[24:26], t.Sample)
	return append(dst, tr[:]...)
}

// DecodeHostCapture reads an output into its records and trailer. An output with no trailer (the run was cut) gives the whole records it
// holds and StopCut. Records that disagree with the trailer, a record longer than the limit, a sampling rate outside 1 to HostMaxSample or
// more records than a window may keep is an error.
func DecodeHostCapture(data []byte) ([]HostRecord, HostTrailer, error) {
	body, tr, trailed, err := splitHostTrailer(data)
	if err != nil {
		return nil, HostTrailer{}, err
	}
	records := make([]HostRecord, 0, min(len(body)/(HostRecordHeader+HostSnapHeaders), HostMaxFrames))
	for off := 0; off+HostRecordHeader <= len(body); {
		rec, next, err := readHostRecord(body, off)
		if err != nil {
			return nil, HostTrailer{}, err
		}
		if next < 0 { // a cut run: the last record is whole or it is not kept
			if trailed {
				return nil, HostTrailer{}, ErrHostCaptureFormat
			}
			break
		}
		if len(records) >= HostMaxFrames {
			return nil, HostTrailer{}, ErrHostCaptureFormat
		}
		records = append(records, rec)
		off = next
	}
	if !trailed {
		return records, HostTrailer{Stop: StopCut, Frames: uint32(len(records))}, nil
	}
	if uint32(len(records)) != tr.Frames {
		return nil, HostTrailer{}, ErrHostCaptureFormat
	}
	return records, tr, nil
}

// splitHostTrailer separates the trailer from the records. trailed is false when the output ends without one (the run was cut).
func splitHostTrailer(data []byte) (body []byte, tr HostTrailer, trailed bool, err error) {
	n := len(data)
	if n < HostTrailerSize || [4]byte(data[n-HostTrailerSize:n-HostTrailerSize+4]) != hostMagic {
		return data, HostTrailer{}, false, nil
	}
	t := data[n-HostTrailerSize:]
	tr = HostTrailer{
		Stop: CaptureStop(t[4]), Frames: binary.BigEndian.Uint32(t[8:12]), KernelSeen: binary.BigEndian.Uint32(t[12:16]),
		KernelDropped: binary.BigEndian.Uint32(t[16:20]), ElapsedMs: binary.BigEndian.Uint32(t[20:24]), Sample: binary.BigEndian.Uint16(t[24:26]),
	}
	if tr.Stop > StopUnsupported {
		return nil, HostTrailer{}, false, ErrHostCaptureFormat
	}
	return data[:n-HostTrailerSize], tr, true, nil
}

// readHostRecord reads the record at off. next is the offset after it, or -1 when the record runs past the end of body.
func readHostRecord(body []byte, off int) (rec HostRecord, next int, err error) {
	kept := int(binary.BigEndian.Uint16(body[off+4 : off+6]))
	sample := binary.BigEndian.Uint16(body[off+6 : off+8])
	if kept > HostSnapMax || sample < 1 || sample > HostMaxSample {
		return HostRecord{}, 0, ErrHostCaptureFormat
	}
	end := off + HostRecordHeader + kept
	if end > len(body) {
		return HostRecord{}, -1, nil
	}
	return HostRecord{OffsetMs: binary.BigEndian.Uint32(body[off : off+4]), Sample: sample, Frame: append([]byte(nil), body[off+HostRecordHeader:end]...)}, end, nil
}
