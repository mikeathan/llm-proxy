package units

import "testing"

func TestBinaryConversions(t *testing.T) {
	tests := []struct {
		name      string
		got, want int64
	}{
		{"KiB(1)", KiB(1), 1024},
		{"KiB(0)", KiB(0), 0},
		{"KiB(-5)", KiB(-5), 0},
		{"MiB(1)", MiB(1), 1024 * 1024},
		{"MiB(2048)", MiB(2048), 2 << 30},
		{"MiB(-1)", MiB(-1), 0},
		{"GiB(2)", GiB(2), 2 << 30},
		{"GiB(0)", GiB(0), 0},
		{"GiB(-3)", GiB(-3), 0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}
