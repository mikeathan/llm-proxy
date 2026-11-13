//go:build linux

package helper

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// ThreadAmbient is Ambient for the calling thread. A capability the unit grants as ambient reaches the helper's threads; this takes it away
// from the thread that asks a question of systemd and puts it back, on that one thread, only for a verified action.
type ThreadAmbient struct{}

// Clear removes every ambient capability from the calling thread, so a program started from it has none.
func (ThreadAmbient) Clear() error {
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return fmt.Errorf("clear ambient capabilities: %w", err)
	}
	return nil
}

// Raise makes capability ambient on the calling thread. The kernel allows it only when the capability is already permitted and
// inheritable, so it adds it to the inheritable set first (permitted is what the unit gave the process).
func (ThreadAmbient) Raise(capability int) error {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return fmt.Errorf("read capabilities: %w", err)
	}
	word, bit := capability/32, uint(capability%32)
	if data[word].Permitted&(1<<bit) == 0 {
		return fmt.Errorf("capability %d is not permitted to this process", capability)
	}
	data[word].Inheritable |= 1 << bit
	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("set inheritable capability: %w", err)
	}
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_RAISE, uintptr(capability), 0, 0); err != nil {
		return fmt.Errorf("raise ambient capability: %w", err)
	}
	return nil
}

// CapNetRaw is the capability the net class needs, and the radio class too (to read frames).
const CapNetRaw = unix.CAP_NET_RAW

// CapNetAdmin is the capability the radio class needs to change an adapter's mode and channel.
const CapNetAdmin = unix.CAP_NET_ADMIN
