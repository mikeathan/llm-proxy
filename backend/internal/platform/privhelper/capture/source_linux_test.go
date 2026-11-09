//go:build linux

package capture

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// The kernel checks a classic BPF program when it is attached, on any socket. Attaching the filter to an ordinary UDP socket proves the
// program is well-formed for the kernel without needing the privilege a packet socket asks for.
func TestTheKernelAcceptsTheFilter(t *testing.T) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Skip("no UDP socket:", err)
	}
	defer unix.Close(fd)
	if err := attachFilter(fd, captureFilter()); err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("filters may not be attached here:", err)
		}
		t.Fatalf("the kernel refused the filter: %v", err)
	}
}

func TestAMissingInterfaceIsAnExpectedStop(t *testing.T) {
	if _, stop, err := Open("nope-does-not-exist"); err == nil || stop == 0 {
		t.Fatalf("stop=%d err=%v", stop, err)
	}
}

// The kernel checks the host filter too, at every sampling rate the program can reach.
func TestTheKernelAcceptsTheHostFilterAtEveryRate(t *testing.T) {
	for n := uint16(1); n <= hostMaxSample; n *= 2 {
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
		if err != nil {
			t.Skip("no UDP socket:", err)
		}
		prog, err := hostFilter(n)
		if err != nil {
			unix.Close(fd)
			t.Fatal(err)
		}
		err = attachFilter(fd, prog)
		unix.Close(fd)
		if err != nil {
			if errors.Is(err, unix.EPERM) {
				t.Skip("filters may not be attached here:", err)
			}
			t.Fatalf("1 in %d: the kernel refused the filter: %v", n, err)
		}
	}
}

func TestAMissingInterfaceIsAnExpectedStopForTheHostLook(t *testing.T) {
	if _, stop, err := OpenHost("nope-does-not-exist"); err == nil || stop == 0 {
		t.Fatalf("stop=%d err=%v", stop, err)
	}
}
