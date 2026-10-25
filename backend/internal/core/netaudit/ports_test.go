package netaudit

import (
	"errors"
	"slices"
	"testing"
)

func TestValidatePorts(t *testing.T) {
	good := map[string]string{"": "", "22": "22", " 22 , 80 ": "22,80", "8000-8100": "8000-8100", "1-65535": "1-65535"}
	for in, want := range good {
		if got, err := ValidatePorts(in); err != nil || got != want {
			t.Errorf("ValidatePorts(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "65536", "90-80", "22,", ",22", "a", "22;ls", "-1", "1-2-3", "99999999"} {
		if _, err := ValidatePorts(bad); !errors.Is(err, ErrBadPorts) {
			t.Errorf("ValidatePorts(%q) err = %v, want ErrBadPorts", bad, err)
		}
	}
}

func TestExpandPorts(t *testing.T) {
	got, err := ExpandPorts("22, 80,78-81")
	if err != nil || !slices.Equal(got, []int{22, 80, 78, 79, 81}) {
		t.Errorf("ExpandPorts = %v, %v (want in order, no duplicates)", got, err)
	}
	if got, err := ExpandPorts(""); err != nil || got != nil {
		t.Errorf("empty list = %v, %v", got, err)
	}
	if _, err := ExpandPorts("nope"); !errors.Is(err, ErrBadPorts) {
		t.Errorf("err = %v", err)
	}
}
