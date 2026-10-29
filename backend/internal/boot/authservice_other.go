//go:build !linux

package boot

import "errors"

// becomeServiceUser is only done on Linux, where the service runs under systemd; elsewhere the operator is told the command.
func becomeServiceUser(installedService) error {
	return errors.New("switching to the service's account is only done on Linux")
}
