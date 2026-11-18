package radio

import (
	"encoding/binary"
	"strings"
	"testing"
)

// wpsAttr is one WPS attribute: 2-byte type, 2-byte length, the value (big-endian, as the specification has it).
func wpsAttr(typ uint16, v []byte) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b, typ)
	binary.BigEndian.PutUint16(b[2:], uint16(len(v)))
	return append(b, v...)
}

func wpsElement(attrs ...[]byte) []byte {
	return ie(221, cat(append([][]byte{{0x00, 0x50, 0xf2, 0x04}}, attrs...)...)...)
}

func TestWPSGivesManufacturerModelAndCategoryNeverTheDeviceName(t *testing.T) {
	el := wpsElement(
		wpsAttr(wpsManufacturer, []byte("Sky UK Ltd")),
		wpsAttr(wpsModelName, []byte("SR203")),
		wpsAttr(wpsModelNumber, []byte("1.2.3")), // a firmware version: never joined to the model
		wpsAttr(0x1011, []byte("Anna's Hub")),    // the device name: never read
		wpsAttr(wpsPrimaryType, []byte{0x00, wpsNetwork, 0x00, 0x50, 0xf2, 0x04, 0x00, 0x01}),
	)
	f, err := Parse(beacon("x", el))
	if err != nil {
		t.Fatal(err)
	}
	id := f.Ident
	if id.Manufacturer != "Sky UK Ltd" || id.Model != "SR203" || id.Category != wpsNetwork {
		t.Fatalf("%+v", id)
	}
	if strings.Contains(id.Manufacturer+id.Model+id.Hint, "Anna") {
		t.Fatalf("the device name reached a field: %+v", id)
	}
}

func TestWPSModelNumberJoinsOnlyWhenItIsAModelNumber(t *testing.T) {
	for _, c := range []struct{ name, number, want string }{
		{"RT-AX88U", "RT-AX88U", "RT-AX88U"},
		{"Archer", "AX55", "Archer AX55"},
		{"Fire TV", "123456", "Fire TV"},
		{"", "EX6120", "EX6120"},
		{"Hub", "4.0.1", "Hub"},
	} {
		if got := modelWith(c.name, c.number); got != c.want {
			t.Errorf("%q + %q = %q, want %q", c.name, c.number, got, c.want)
		}
	}
}

func TestIdentityTextIsCleanedAndBounded(t *testing.T) {
	el := wpsElement(wpsAttr(wpsModelName, []byte("Bad\x00\x1b[31mModel"+strings.Repeat("y", 80))), wpsAttr(wpsManufacturer, []byte("WPS")))
	f, _ := Parse(beacon("x", el))
	if strings.ContainsAny(f.Ident.Model, "\x00\x1b") || len(f.Ident.Model) > maxIdentityText {
		t.Errorf("model not cleaned: %q", f.Ident.Model)
	}
	if f.Ident.Manufacturer != "" {
		t.Errorf("a placeholder is not a manufacturer: %q", f.Ident.Manufacturer)
	}
}

func TestATruncatedWPSElementKeepsWhatCameBefore(t *testing.T) {
	el := wpsElement(wpsAttr(wpsManufacturer, []byte("Netgear")), []byte{0x10, 0x23, 0x00, 0x40, 'A'})
	f, _ := Parse(beacon("x", el))
	if f.Ident.Manufacturer != "Netgear" || f.Ident.Model != "" {
		t.Errorf("%+v", f.Ident)
	}
}

func probeWith(src MAC, ies ...[]byte) []byte {
	return rtap(cat(hdr(0x40, 0, macBroad, src, macBroad), cat(ies...)))
}

func TestVendorElementIsAHint(t *testing.T) {
	f, _ := Parse(probeWith(macPhone, ie(0), ie(221, 0x00, 0x17, 0xf2, 0x0a, 0x00)))
	if f.Ident.Hint != "Apple" {
		t.Errorf("hint %q", f.Ident.Hint)
	}
	f, _ = Parse(probeWith(macPhone, ie(0), ie(221, 0x00, 0x50, 0xf2, 0x08, 0x00))) // Microsoft: almost every device sends one
	if f.Ident.Hint != "" {
		t.Errorf("an uninformative vendor element gave %q", f.Ident.Hint)
	}
}

func TestProbeSignatureIgnoresTheNetworkAskedForAndFollowsTheLayout(t *testing.T) {
	ht := ie(45, 0xef, 0x01, 0x17)
	a, _ := Parse(probeWith(macPhone, ie(0, []byte("home")...), ie(1, 0x82, 0x84), ht))
	b, _ := Parse(probeWith(MAC{0x02, 9, 9, 9, 9, 9}, ie(0, []byte("cafe")...), ie(1, 0x82, 0x84), ht))
	c, _ := Parse(probeWith(macPhone, ie(0), ie(1, 0x82, 0x84), ie(45, 0x2c, 0x01, 0x17)))
	if a.Ident.Signature == 0 || a.Ident.Signature != b.Ident.Signature {
		t.Errorf("same layout, different networks: %x %x", a.Ident.Signature, b.Ident.Signature)
	}
	if a.Ident.Signature == c.Ident.Signature {
		t.Error("different HT capabilities gave the same signature")
	}
	bc, _ := Parse(beacon("x", ht))
	if bc.Ident.Signature != 0 {
		t.Error("a beacon has no request signature")
	}
}
