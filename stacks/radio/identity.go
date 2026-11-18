package radio

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
)

// What a frame says about the device that sent it, beyond its address (ADR-0058). Three things, each read only from elements the device itself
// broadcasts to everyone:
//
//   - The WPS element (vendor 00-50-f2 type 4) of a beacon, probe response, probe request or association request carries the manufacturer,
//     the model name and number and a primary device type. Those name a product, not a person, and are kept. The same element's Device Name
//     is often a person's name ("Anna's phone"): it is never read into any field.
//   - A vendor element's company number (OUI) is a hint of who built the radio or its software (Apple, Broadcom, ...): a hint, never proof.
//   - The set and order of elements in a probe or association request is a signature of a device model and its software: phones of one
//     model and system version share it. It is kept as a 64-bit sum and only ever said to mean "the same kind of device", never "the same device".

// Identity is the product facts a frame carries. Empty fields were not announced.
type Identity struct {
	Manufacturer string
	Model        string
	// Category is the WPS primary device type category (1 computer, 3 printer, 4 camera, 6 network infrastructure, 7 display, 8 multimedia,
	// 9 gaming, 10 telephone, 11 audio ...); 0 when none was announced.
	Category int
	// Hint is the company a vendor element names, from vendorHints; empty when none is known.
	Hint string
	// Signature is the sum of the request's element layout; 0 for a frame that is not a probe or association request.
	Signature uint64
}

// The WPS attributes read; every other attribute is skipped without being looked at (0x1011 Device Name is one of them on purpose).
const (
	wpsManufacturer = 0x1021
	wpsModelName    = 0x1023
	wpsModelNumber  = 0x1024
	wpsPrimaryType  = 0x1054
	maxIdentityText = 40
)

// vendorHints are the companies whose vendor elements say something about the device. The common, uninformative ones (Microsoft's WMM and
// WPS 00-50-f2 and the Wi-Fi Alliance's 50-6f-9a, sent by almost every device) are not here.
var vendorHints = map[[3]byte]string{
	{0x00, 0x17, 0xf2}: "Apple",
	{0x00, 0x10, 0x18}: "Broadcom",
	{0x00, 0x90, 0x4c}: "Broadcom",
	{0x00, 0x03, 0x7f}: "Qualcomm",
	{0x8c, 0xfd, 0xf0}: "Qualcomm",
	{0x00, 0x0c, 0xe7}: "MediaTek",
	{0x00, 0x0c, 0x43}: "MediaTek",
	{0x00, 0xe0, 0x4c}: "Realtek",
	{0x00, 0x40, 0x96}: "Cisco",
	{0x00, 0x13, 0x92}: "Ruckus",
	{0x00, 0x0b, 0x86}: "Aruba",
	{0x00, 0x26, 0x86}: "Quantenna",
	{0xf4, 0xf5, 0xe8}: "Google",
	{0x00, 0x1d, 0x0f}: "TP-Link",
	{0x00, 0x18, 0x0a}: "Meraki",
	{0x00, 0x15, 0x6d}: "Ubiquiti",
	{0x00, 0x0c, 0xc3}: "AVM",
}

// readVendor reads a vendor element: the WPS attributes when it is WPS, otherwise the company hint.
func (id *Identity) readVendor(b []byte) {
	if len(b) < 4 {
		return
	}
	if b[0] == 0x00 && b[1] == 0x50 && b[2] == 0xf2 && b[3] == 4 {
		id.readWPS(b[4:])
		return
	}
	if name, ok := vendorHints[[3]byte{b[0], b[1], b[2]}]; ok && id.Hint == "" {
		id.Hint = name
	}
}

// readWPS walks the attributes (2-byte type, 2-byte length, big-endian). An attribute that claims more bytes than remain ends the walk.
func (id *Identity) readWPS(b []byte) {
	var number string
	for off := 0; off+4 <= len(b); {
		typ, n := binary.BigEndian.Uint16(b[off:]), int(binary.BigEndian.Uint16(b[off+2:]))
		off += 4
		if off+n > len(b) {
			return
		}
		v := b[off : off+n]
		off += n
		switch typ {
		case wpsManufacturer:
			id.Manufacturer = cleanText(v)
		case wpsModelName:
			id.Model = cleanText(v)
		case wpsModelNumber:
			number = cleanText(v)
		case wpsPrimaryType:
			if n >= 2 {
				id.Category = int(binary.BigEndian.Uint16(v))
			}
		}
	}
	id.Model = modelWith(id.Model, number)
}

// modelWith joins a model name and number when the number looks like a model number (letters and digits, no dots: some makers put the firmware
// version there) and does not repeat the name.
func modelWith(name, number string) string {
	if number == "" || strings.Contains(number, ".") || strings.Contains(strings.ToLower(name), strings.ToLower(number)) || isPlaceholder(number) {
		return name
	}
	if name == "" {
		return number
	}
	return limitText(name + " " + number)
}

// isPlaceholder is a model text that says nothing: digits only, or a word routers ship as a default.
func isPlaceholder(s string) bool {
	l := strings.ToLower(s)
	switch l {
	case "", "123456", "0", "1", "wps", "model", "none", "n/a", "na", "default", "unknown", "router", "ap":
		return true
	}
	return strings.Trim(l, "0123456789") == ""
}

// cleanText keeps printable ASCII, folds runs of spaces and caps the length: a model text is shown on screen and must never carry a control
// character or an endless string.
func cleanText(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteByte(' ')
		}
	}
	s := strings.Join(strings.Fields(sb.String()), " ")
	if isPlaceholder(s) {
		return ""
	}
	return limitText(s)
}

func limitText(s string) string {
	if len(s) > maxIdentityText {
		s = strings.TrimSpace(s[:maxIdentityText])
	}
	return s
}

// signer builds a request's layout signature: each element's id in order, with the few bytes that differ between models and not between
// devices of one model (HT capabilities, extended capabilities, a vendor element's company and type). The SSID's content and the channel are left
// out, so the same device asking for different networks keeps one signature.
type signer struct {
	h   uint64
	set bool
}

func (s *signer) add(id byte, body []byte) {
	h := fnv.New64a()
	var prev [8]byte
	binary.LittleEndian.PutUint64(prev[:], s.h)
	_, _ = h.Write(prev[:])
	_, _ = h.Write([]byte{id})
	switch {
	case id == 45 && len(body) >= 2: // HT capabilities: the capability bits
		_, _ = h.Write(body[:2])
	case id == 127: // extended capabilities
		_, _ = h.Write(body[:min(len(body), 8)])
	case id == 221 && len(body) >= 4: // vendor element: who and which type
		_, _ = h.Write(body[:4])
	}
	s.h, s.set = h.Sum64(), true
}

func (s *signer) sum() uint64 {
	if !s.set {
		return 0
	}
	if s.h == 0 {
		return 1 // 0 means "none"
	}
	return s.h
}

// WPS device categories, from the WPS specification's primary device type table.
const (
	wpsComputer = 1
	wpsInput    = 2
	wpsPrinter  = 3
	wpsCamera   = 4
	wpsStorage  = 5
	wpsNetwork  = 6
	wpsDisplay  = 7
	wpsMedia    = 8
	wpsGaming   = 9
	wpsPhone    = 10
	wpsAudio    = 11
)
