package radio

import "testing"

func TestMakerOf(t *testing.T) {
	cases := []struct {
		name string
		mac  MAC
		want bool // a maker is found
	}{
		{"a registered block", MAC{0x28, 0x6f, 0xb9, 0x01, 0x02, 0x03}, true},
		{"a random address has no maker", MAC{0x2a, 0x6f, 0xb9, 0x01, 0x02, 0x03}, false},
		{"a group address has no maker", MAC{0x01, 0x00, 0x5e, 0x01, 0x02, 0x03}, false},
		{"a block nobody was assigned", MAC{0xfc, 0xff, 0xff, 0x01, 0x02, 0x03}, false},
	}
	for _, c := range cases {
		got := MakerOf(c.mac)
		if (got != "") != c.want {
			t.Errorf("%s: MakerOf(%v) = %q", c.name, c.mac, got)
		}
	}
	if got := MakerOf(MAC{0x28, 0x6f, 0xb9, 0x01, 0x02, 0x03}); got != "Nokia Shanghai Bell" {
		t.Errorf("the legal suffix should be trimmed: %q", got)
	}
}

func TestMakerTableSize(t *testing.T) {
	if n := len(makers()); n < 40000 {
		t.Fatalf("the embedded table holds %d makers; the registries list over 50,000", n)
	}
}
