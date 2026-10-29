//go:build !unix

package boot

import "errors"

func ownerOfFolder(string) (int, int, bool) { return 0, 0, false }

// becomeUserID is only done on Unix, where the service runs under systemd.
func becomeUserID(int, int) error {
	return errors.New("switching account is only done on Unix")
}
