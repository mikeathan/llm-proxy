//go:build !linux

package neighbors

import "context"

// discover6 has no way to send the echo off Linux.
func discover6(context.Context, []string) error { return ErrUnavailable }
