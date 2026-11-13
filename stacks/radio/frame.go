package radio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is the parser for what a monitor-mode adapter hears: a radiotap header and an 802.11 frame header, from bytes, into typed facts
// (Constitution I.16). Anyone in radio range controls these bytes, so every read goes through a bounds check, a lying length is clamped or
// refused, nothing is allocated from a length the frame states, and the fuzz test must never see a panic. It reads headers only: the helper
// returns at most 512 bytes of a beacon and 64 of anything else, and a frame body is never kept.

// MAC is a hardware address as heard on the air. It is the other people's data of ADR-0053: the store keeps a keyed tag of it, never this.
type MAC [6]byte

// String is the usual colon form.
func (m MAC) String() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}

// Errors Parse returns. A frame that fails is counted and dropped by the caller.
var (
	ErrTruncated   = errors.New("radio: frame is shorter than its own headers")
	ErrNotRadiotap = errors.New("radio: not a radiotap header")
	ErrNotWiFi     = errors.New("radio: not an 802.11 frame")
)

// FrameKind names the frames the stack tells apart. Every other management frame is KindManagement, a control frame KindControl, a data frame
// KindData.
type FrameKind string

// The kinds of frame.
const (
	KindBeacon             FrameKind = "beacon"
	KindProbeRequest       FrameKind = "probe_request"
	KindProbeResponse      FrameKind = "probe_response"
	KindAssociationRequest FrameKind = "association_request"
	KindAssociationReply   FrameKind = "association_response"
	KindAuthentication     FrameKind = "authentication"
	KindDeauthentication   FrameKind = "deauthentication"
	KindDisassociation     FrameKind = "disassociation"
	KindManagement         FrameKind = "management"
	KindControl            FrameKind = "control"
	KindData               FrameKind = "data"
)

// SecMode is what a network says about its protection.
type SecMode string

// The modes a beacon can announce.
const (
	SecOpen SecMode = "open"
	SecWEP  SecMode = "wep"
	SecWPA  SecMode = "wpa"
	SecWPA2 SecMode = "wpa2"
	// SecWPA3 is SAE only; SecWPA23 is a network that offers both SAE and PSK.
	SecWPA3  SecMode = "wpa3"
	SecWPA23 SecMode = "wpa2_wpa3"
	SecOWE   SecMode = "owe"
)

// Security is the protection a beacon or probe response announces.
type Security struct {
	Mode SecMode
	// Enterprise: the network signs devices in with 802.1X, not a shared password.
	Enterprise  bool
	PMFCapable  bool
	PMFRequired bool
}

// BeaconInfo is what only a beacon or probe response carries.
type BeaconInfo struct {
	IntervalTU uint16
	// Generation is the highest Wi-Fi generation announced: 4 (HT), 5 (VHT), 6 (HE), 0 when none was.
	Generation int
}

// AuthInfo is the fixed part of an authentication frame.
type AuthInfo struct {
	Algorithm, Sequence, Status uint16
}

// Frame is one frame's typed facts. Only the fields its Kind defines are set.
type Frame struct {
	Kind     FrameKind
	Subtype  uint8
	Retry    bool
	Signal   int // dBm, the strongest antenna; valid when SignalOK
	SignalOK bool
	FreqMHz  uint16
	RateKbps uint32
	Channel  int

	Receiver, Transmitter, BSSID MAC
	HasTransmitter, HasBSSID     bool

	SSID       string // sanitised, at most 32 bytes of the name
	HiddenSSID bool
	Security   Security
	Beacon     BeaconInfo
	Auth       AuthInfo
	Reason     uint16 // deauthentication and disassociation
	// PayloadLen is the length of a data frame's body as captured (headers-only capture makes it a lower bound).
	PayloadLen int
}

// Parse reads one frame that starts with a radiotap header (link type IEEE802_11_RADIO).
func Parse(b []byte) (Frame, error) {
	var f Frame
	rt, err := parseRadiotap(b)
	if err != nil {
		return f, err
	}
	f.Signal, f.SignalOK, f.FreqMHz, f.RateKbps = rt.signal, rt.signalOK, rt.freq, rt.rate
	body := b[rt.length:]
	if rt.fcsAtEnd {
		if len(body) < 4 {
			return f, ErrTruncated
		}
		body = body[:len(body)-4]
	}
	if err := parseDot11(body, &f); err != nil {
		return f, err
	}
	if f.Channel == 0 {
		f.Channel = channelOf(int(f.FreqMHz))
	}
	return f, nil
}

type radiotap struct {
	length   int
	signal   int
	signalOK bool
	freq     uint16
	rate     uint32
	fcsAtEnd bool
}

type rtSize struct{ size, align int }

// rtSizes gives the size and alignment of a radiotap field by presence bit. A bit not in the table has an unknown size, after which the
// rest of the fields cannot be placed and parsing of them stops (the header length still locates the frame).
var rtSizes = map[int]rtSize{
	0: {8, 8}, 1: {1, 1}, 2: {1, 1}, 3: {4, 2}, 5: {1, 1}, 6: {1, 1}, 7: {2, 2}, 8: {2, 2}, 9: {2, 2}, 10: {1, 1}, 11: {1, 1}, 12: {1, 1},
	13: {1, 1}, 14: {2, 2}, 15: {2, 2}, 16: {1, 1}, 17: {1, 1}, 18: {8, 4}, 19: {3, 1}, 20: {8, 4}, 21: {12, 2}, 22: {12, 8}, 23: {12, 2},
	24: {12, 2}, 25: {6, 2}, 26: {1, 1}, 27: {4, 2},
}

func parseRadiotap(b []byte) (radiotap, error) {
	var rt radiotap
	if len(b) < 8 {
		return rt, ErrTruncated
	}
	if b[0] != 0 {
		return rt, ErrNotRadiotap
	}
	rt.length = int(binary.LittleEndian.Uint16(b[2:]))
	if rt.length < 8 || rt.length > len(b) {
		return rt, ErrTruncated
	}
	hdr := b[:rt.length]
	words, off := presentWords(hdr)
	if off < 0 {
		return rt, ErrTruncated
	}
	readFields(hdr, words, off, &rt)
	return rt, nil
}

// presentWords collects the presence bitmaps (bit 31 chains another) and returns the offset of the first field.
func presentWords(hdr []byte) ([]uint32, int) {
	var words []uint32
	off := 4
	for {
		if off+4 > len(hdr) || len(words) >= 8 {
			return nil, -1
		}
		w := binary.LittleEndian.Uint32(hdr[off:])
		words = append(words, w)
		off += 4
		if w&(1<<31) == 0 {
			return words, off
		}
	}
}

// readFields walks the fields of every radiotap-namespace word. A later word in the radiotap namespace repeats the fields for another
// antenna, so the strongest signal of all of them is kept. A vendor namespace, or a bit of unknown size, ends the walk.
func readFields(hdr []byte, words []uint32, off int, rt *radiotap) {
	radiotapNS := true
	for _, w := range words {
		if radiotapNS {
			var ok bool
			if off, ok = readWord(hdr, w, off, rt); !ok {
				return
			}
		} else {
			return // a vendor namespace: its field sizes are unknown here
		}
		switch {
		case w&(1<<30) != 0:
			radiotapNS = false
		case w&(1<<29) != 0:
			radiotapNS = true
		}
	}
}

func readWord(hdr []byte, w uint32, off int, rt *radiotap) (int, bool) {
	for bit := 0; bit < 29; bit++ {
		if w&(1<<bit) == 0 {
			continue
		}
		sz, known := rtSizes[bit]
		if !known {
			return off, false
		}
		size, align := sz.size, sz.align
		off = (off + align - 1) / align * align
		if off+size > len(hdr) {
			return off, false
		}
		takeField(bit, hdr[off:off+size], rt)
		off += size
	}
	return off, true
}

func takeField(bit int, v []byte, rt *radiotap) {
	switch bit {
	case 1:
		rt.fcsAtEnd = rt.fcsAtEnd || v[0]&0x10 != 0
	case 2:
		rt.rate = uint32(v[0]) * 500
	case 3:
		if rt.freq == 0 {
			rt.freq = binary.LittleEndian.Uint16(v)
		}
	case 5:
		s := int(int8(v[0]))
		if !rt.signalOK || s > rt.signal {
			rt.signal, rt.signalOK = s, true
		}
	}
}

func channelOf(mhz int) int {
	switch {
	case mhz == 2484:
		return 14
	case mhz >= 2412 && mhz < 2484:
		return (mhz - 2407) / 5
	case mhz >= 5000 && mhz <= 5895:
		return (mhz - 5000) / 5
	case mhz >= 5955 && mhz <= 7115:
		return (mhz - 5950) / 5
	}
	return 0
}

const (
	typeMgmt = 0
	typeCtrl = 1
	typeData = 2
)

func parseDot11(b []byte, f *Frame) error {
	if len(b) < 10 {
		return ErrTruncated
	}
	fc0, fc1 := b[0], b[1]
	if fc0&0x03 != 0 {
		return ErrNotWiFi
	}
	typ, sub := int(fc0>>2)&3, fc0>>4
	f.Subtype = sub
	f.Retry = fc1&0x08 != 0
	copy(f.Receiver[:], b[4:10])
	switch typ {
	case typeCtrl:
		f.Kind = KindControl
		if len(b) >= 16 && sub != 12 && sub != 13 { // CTS and ACK carry only a receiver
			copy(f.Transmitter[:], b[10:16])
			f.HasTransmitter = true
		}
		return nil
	case typeMgmt:
		return parseMgmt(b, sub, f)
	case typeData:
		return parseData(b, fc1, sub, f)
	}
	return ErrNotWiFi
}

func parseData(b []byte, fc1, sub uint8, f *Frame) error {
	f.Kind = KindData
	if len(b) < 24 {
		return ErrTruncated
	}
	var a2, a3 MAC
	copy(a2[:], b[10:16])
	copy(a3[:], b[16:22])
	f.Transmitter, f.HasTransmitter = a2, true
	f.BSSID, f.HasBSSID = dataBSSID(fc1, f.Receiver, a2, a3)
	if hl := dataHeaderLen(fc1, sub); len(b) > hl {
		f.PayloadLen = len(b) - hl
	}
	return nil
}

// dataBSSID picks the BSSID out of the three addresses by the To DS and From DS flags; a frame between access points has none here.
func dataBSSID(fc1 uint8, a1, a2, a3 MAC) (MAC, bool) {
	toDS, fromDS := fc1&1 != 0, fc1&2 != 0
	switch {
	case toDS && !fromDS:
		return a1, true
	case !toDS && fromDS:
		return a2, true
	case !toDS && !fromDS:
		return a3, true
	}
	return MAC{}, false
}

func dataHeaderLen(fc1, sub uint8) int {
	hl := 24
	if fc1&3 == 3 {
		hl += 6 // a fourth address
	}
	if sub&0x08 != 0 { // a QoS subtype
		hl += 2
		if fc1&0x80 != 0 {
			hl += 4 // HT control
		}
	}
	return hl
}

func parseMgmt(b []byte, sub uint8, f *Frame) error {
	if len(b) < 24 {
		return ErrTruncated
	}
	copy(f.Transmitter[:], b[10:16])
	copy(f.BSSID[:], b[16:22])
	f.HasTransmitter, f.HasBSSID = true, true
	body := b[24:]
	switch sub {
	case 8, 5: // beacon, probe response
		f.Kind = KindBeacon
		if sub == 5 {
			f.Kind = KindProbeResponse
		}
		return parseBeaconBody(body, f)
	case 4:
		f.Kind = KindProbeRequest
		parseIEs(body, false, f)
	case 0, 2:
		return parseAssocRequest(body, sub, f)
	case 1, 3:
		f.Kind = KindAssociationReply
	case 11:
		return parseAuth(body, f)
	case 12, 10:
		return parseReason(body, sub, f)
	default:
		f.Kind = KindManagement
	}
	return nil
}

func parseAssocRequest(body []byte, sub uint8, f *Frame) error {
	f.Kind = KindAssociationRequest
	skip := 4
	if sub == 2 {
		skip = 10 // reassociation carries the current AP's address too
	}
	if len(body) < skip {
		return ErrTruncated
	}
	parseIEs(body[skip:], false, f)
	return nil
}

func parseAuth(body []byte, f *Frame) error {
	f.Kind = KindAuthentication
	if len(body) < 6 {
		return ErrTruncated
	}
	f.Auth = AuthInfo{binary.LittleEndian.Uint16(body), binary.LittleEndian.Uint16(body[2:]), binary.LittleEndian.Uint16(body[4:])}
	return nil
}

func parseBeaconBody(body []byte, f *Frame) error {
	if len(body) < 12 {
		return ErrTruncated
	}
	f.Beacon.IntervalTU = binary.LittleEndian.Uint16(body[8:])
	capab := binary.LittleEndian.Uint16(body[10:])
	parseIEs(body[12:], capab&0x0010 != 0, f)
	return nil
}

func parseReason(body []byte, sub uint8, f *Frame) error {
	f.Kind = KindDeauthentication
	if sub == 10 {
		f.Kind = KindDisassociation
	}
	if len(body) < 2 {
		return ErrTruncated
	}
	f.Reason = binary.LittleEndian.Uint16(body)
	return nil
}

// parseIEs walks the information elements. An element that claims more bytes than remain ends the walk; what was read before it stays.
func parseIEs(b []byte, privacy bool, f *Frame) {
	var sec secScan
	sawSSID := false
	for i := 0; i+2 <= len(b); {
		id, n := b[i], int(b[i+1])
		if i+2+n > len(b) {
			break
		}
		body := b[i+2 : i+2+n]
		i += 2 + n
		if id == 0 {
			if !sawSSID {
				sawSSID = true
				f.SSID, f.HiddenSSID = cleanSSID(body)
			}
			continue
		}
		readIE(id, body, f, &sec)
	}
	if f.Kind == KindBeacon || f.Kind == KindProbeResponse {
		f.Security = sec.result(privacy)
	}
}

func readIE(id byte, body []byte, f *Frame, sec *secScan) {
	n := len(body)
	switch {
	case id == 3 && n == 1: // DS parameter set: the channel
		f.Channel = int(body[0])
	case id == 61 && n >= 1 && f.Channel == 0: // HT operation: the primary channel, when no DS element gave it
		f.Channel = int(body[0])
	case id == 48:
		sec.rsn(body)
	case id == 221 && isWPAElement(body):
		sec.wpa = true
	default:
		f.Beacon.Generation = max(f.Beacon.Generation, generationOf(id, body))
	}
}

// isWPAElement: the vendor element 00-50-f2 type 1 announces the older WPA.
func isWPAElement(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x00 && b[1] == 0x50 && b[2] == 0xf2 && b[3] == 1
}

// generationOf is the Wi-Fi generation an element announces: HT 4, VHT 5, and the HE capabilities (element extension 35) 6.
func generationOf(id byte, body []byte) int {
	switch {
	case id == 45:
		return 4
	case id == 191:
		return 5
	case id == 255 && len(body) >= 1 && body[0] == 35:
		return 6
	}
	return 0
}

// cleanSSID keeps at most 32 bytes, drops control characters and invalid UTF-8, and says when the name is empty or all zero bytes.
func cleanSSID(b []byte) (string, bool) {
	if len(b) > 32 {
		b = b[:32]
	}
	zero := true
	for _, c := range b {
		if c != 0 {
			zero = false
			break
		}
	}
	if len(b) == 0 || zero {
		return "", true
	}
	s := strings.ToValidUTF8(string(b), "�")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
	if !utf8.ValidString(s) {
		return "", false
	}
	return s, false
}

// secScan gathers the security elements of one beacon.
type secScan struct {
	rsnSeen, wpa           bool
	psk, sae, owe, enterpr bool
	pmfCap, pmfReq         bool
}

// rsn reads an RSN element: version 2, group cipher 4, pairwise count 2 + 4 each, AKM count 2 + 4 each, capabilities 2.
func (s *secScan) rsn(b []byte) {
	s.rsnSeen = true
	if len(b) < 8 {
		return
	}
	off := 6
	pc := int(binary.LittleEndian.Uint16(b[off:]))
	off += 2 + 4*pc
	if off+2 > len(b) {
		return
	}
	ac := int(binary.LittleEndian.Uint16(b[off:]))
	off += 2
	for i := 0; i < ac && off+4 <= len(b); i++ {
		s.akm(b[off : off+4])
		off += 4
	}
	if off+2 <= len(b) {
		c := binary.LittleEndian.Uint16(b[off:])
		s.pmfReq = c&0x40 != 0
		s.pmfCap = c&0x80 != 0 || s.pmfReq
	}
}

// akm records one authentication and key management suite (selector 00-0f-ac:N).
func (s *secScan) akm(v []byte) {
	if v[0] != 0x00 || v[1] != 0x0f || v[2] != 0xac {
		return
	}
	switch v[3] {
	case 1, 3, 5, 11, 12, 13:
		s.enterpr = true
	case 2, 4, 6:
		s.psk = true
	case 8, 9, 24:
		s.sae = true
	case 18:
		s.owe = true
	}
}

func (s *secScan) result(privacy bool) Security {
	r := Security{Enterprise: s.enterpr, PMFCapable: s.pmfCap, PMFRequired: s.pmfReq}
	switch {
	case s.owe:
		r.Mode = SecOWE
	case s.sae && s.psk:
		r.Mode = SecWPA23
	case s.sae:
		r.Mode = SecWPA3
	case s.rsnSeen:
		r.Mode = SecWPA2
	case s.wpa:
		r.Mode = SecWPA
	case privacy:
		r.Mode = SecWEP
	default:
		r.Mode = SecOpen
	}
	return r
}
