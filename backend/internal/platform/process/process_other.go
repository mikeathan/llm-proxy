//go:build !linux && !darwin

package process

import "fmt"

func listByBinary(binaryName string, activePID int) ([]Info, error) {
	return nil, fmt.Errorf("process listing not supported on this platform")
}
