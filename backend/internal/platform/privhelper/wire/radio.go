package wire

import (
	"encoding/binary"
	"errors"
)

// The framing of the radio_listen action's output (Constitution I.16: the radiotap header and the 802.11 header of the frames a USB Wi-Fi
// adapter hears, up to 512 bytes of a beacon and 64 bytes of any other frame). The helper's radio program writes variable-length records and
// one trailer and parses nothing but the two bytes that tell a beacon from another frame (RadioSnapFor). Anyone in range controls these
// bytes, so the unprivileged service reads them with its own strict parser, never the helper. This file is only the byte layout both sides share.
const (
	// RadioCommand is the first argument that makes the helper binary run the radio capture program.
	RadioCommand = "radio-listen"
	// RadioSnapBeacon is how many bytes of a beacon are kept: its tagged elements hold the network name and the security fields.
	RadioSnapBeacon = 512
	// RadioSnapOther is how many bytes of any other frame are kept: the radiotap header (38 bytes on the adapter measured) and the 802.11 header.
	RadioSnapOther = 64
	// RadioRecordHeader is the start of a record: 4 bytes of milliseconds since the window began, 2 bytes of kept length, 2 bytes reserved.
	RadioRecordHeader = 8
	// RadioTrailerSize is the trailer that ends the output.
	RadioTrailerSize = 32
	// RadioMaxFrames is the most records one window keeps.
	RadioMaxFrames = 4000
	// RadioMaxOutput is the most bytes one window can write: every record a beacon, and the trailer.
	RadioMaxOutput = RadioMaxFrames*(RadioRecordHeader+RadioSnapBeacon) + RadioTrailerSize
	// RadioMaxSeconds is the longest window on one channel.
	RadioMaxSeconds = 30
	// RadioReportMax is the most bytes of the channel read-back the result carries.
	RadioReportMax = 4096
)

// radioMagic opens the trailer.
var radioMagic = [4]byte{'R', 'W', 'T', '1'}

// RadioRecord is one kept frame: when it arrived and its first bytes.
type RadioRecord struct {
	OffsetMs uint32
	Frame    []byte
}

// RadioTrailer is what the window says about itself.
type RadioTrailer struct {
	Stop CaptureStop
	// Frames is how many records were written.
	Frames uint32
	// KernelSeen and KernelDropped are the packet socket's own counters: frames it queued and frames it dropped because the program did not
	// read fast enough.
	KernelSeen, KernelDropped uint32
	// ElapsedMs is how long the window ran.
	ElapsedMs uint32
}

// ErrRadioFormat means the bytes are not a radio_listen output.
var ErrRadioFormat = errors.New("not a radio_listen output")

// RadioSnapFor is how many bytes of a frame are kept: RadioSnapBeacon for a beacon, RadioSnapOther for anything else, including a frame too
// short to say. It reads only the radiotap length (bytes 2 and 3, little endian) and the first byte of the 802.11 frame control it points
// at: type management (0) and subtype beacon (8) is 0x80 once the version bits are masked.
func RadioSnapFor(frame []byte) int {
	if len(frame) < 4 || frame[0] != 0 {
		return min(len(frame), RadioSnapOther)
	}
	hdr := int(binary.LittleEndian.Uint16(frame[2:4]))
	if hdr >= 8 && hdr < len(frame) && frame[hdr]&0xFC == 0x80 {
		return min(len(frame), RadioSnapBeacon)
	}
	return min(len(frame), RadioSnapOther)
}

// AppendRadioRecord appends one record, the frame cut to RadioSnapFor.
func AppendRadioRecord(dst []byte, offsetMs uint32, frame []byte) []byte {
	frame = frame[:RadioSnapFor(frame)]
	var head [RadioRecordHeader]byte
	binary.BigEndian.PutUint32(head[0:4], offsetMs)
	binary.BigEndian.PutUint16(head[4:6], uint16(len(frame)))
	return append(append(dst, head[:]...), frame...)
}

// AppendRadioTrailer appends the trailer.
func AppendRadioTrailer(dst []byte, t RadioTrailer) []byte {
	var tr [RadioTrailerSize]byte
	copy(tr[0:4], radioMagic[:])
	tr[4] = byte(t.Stop)
	binary.BigEndian.PutUint32(tr[8:12], t.Frames)
	binary.BigEndian.PutUint32(tr[12:16], t.KernelSeen)
	binary.BigEndian.PutUint32(tr[16:20], t.KernelDropped)
	binary.BigEndian.PutUint32(tr[20:24], t.ElapsedMs)
	return append(dst, tr[:]...)
}

// DecodeRadio reads an output into its records and trailer. An output with no trailer (the run was cut) gives the whole records it holds and
// StopCut. Records that disagree with the trailer, a record longer than the limit or more records than a window may keep is an error.
func DecodeRadio(data []byte) ([]RadioRecord, RadioTrailer, error) {
	body, tr, trailed, err := splitRadioTrailer(data)
	if err != nil {
		return nil, RadioTrailer{}, err
	}
	records := make([]RadioRecord, 0, min(len(body)/(RadioRecordHeader+RadioSnapOther), RadioMaxFrames))
	for off := 0; off+RadioRecordHeader <= len(body); {
		kept := int(binary.BigEndian.Uint16(body[off+4 : off+6]))
		end := off + RadioRecordHeader + kept
		switch {
		case kept > RadioSnapBeacon || len(records) >= RadioMaxFrames:
			return nil, RadioTrailer{}, ErrRadioFormat
		case end > len(body): // a cut run: the last record is whole or it is not kept
			if trailed {
				return nil, RadioTrailer{}, ErrRadioFormat
			}
			return records, RadioTrailer{Stop: StopCut, Frames: uint32(len(records))}, nil
		}
		records = append(records, RadioRecord{OffsetMs: binary.BigEndian.Uint32(body[off : off+4]), Frame: append([]byte(nil), body[off+RadioRecordHeader:end]...)})
		off = end
	}
	if !trailed {
		return records, RadioTrailer{Stop: StopCut, Frames: uint32(len(records))}, nil
	}
	if uint32(len(records)) != tr.Frames {
		return nil, RadioTrailer{}, ErrRadioFormat
	}
	return records, tr, nil
}

func splitRadioTrailer(data []byte) (body []byte, tr RadioTrailer, trailed bool, err error) {
	n := len(data)
	if n < RadioTrailerSize || [4]byte(data[n-RadioTrailerSize:n-RadioTrailerSize+4]) != radioMagic {
		return data, RadioTrailer{}, false, nil
	}
	t := data[n-RadioTrailerSize:]
	tr = RadioTrailer{
		Stop: CaptureStop(t[4]), Frames: binary.BigEndian.Uint32(t[8:12]), KernelSeen: binary.BigEndian.Uint32(t[12:16]),
		KernelDropped: binary.BigEndian.Uint32(t[16:20]), ElapsedMs: binary.BigEndian.Uint32(t[20:24]),
	}
	if tr.Stop > StopUnsupported {
		return nil, RadioTrailer{}, false, ErrRadioFormat
	}
	return data[:n-RadioTrailerSize], tr, true, nil
}
