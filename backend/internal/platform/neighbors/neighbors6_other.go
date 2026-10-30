//go:build !linux

package neighbors

// read6 has no source off Linux.
func read6() ([]Entry, error) { return nil, ErrUnavailable }
