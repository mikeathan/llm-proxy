//go:build !linux

package helper

import "errors"

// ThreadAmbient is unavailable off Linux.
type ThreadAmbient struct{}

var errNoCaps = errors.New("capabilities are available on Linux only")

// Clear is unavailable off Linux.
func (ThreadAmbient) Clear() error { return errNoCaps }

// Raise is unavailable off Linux.
func (ThreadAmbient) Raise(int) error { return errNoCaps }

// CapNetRaw is CAP_NET_RAW's number; off Linux nothing uses it.
const CapNetRaw = 13
