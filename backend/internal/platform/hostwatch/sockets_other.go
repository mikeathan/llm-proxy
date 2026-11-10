//go:build !darwin

package hostwatch

// systemListeners has nothing to read off Linux (which reads /proc) and macOS: "could not check", never "nothing is listening".
func systemListeners() ([]Listener, error) { return nil, ErrUnavailable }
