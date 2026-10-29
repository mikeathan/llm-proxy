//go:build linux

package boot

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// becomeServiceUser makes this process the service's account, so the sign-in file it writes is owned by the account that
// reads it. It changes this process's identity (nothing is started), and only root may.
func becomeServiceUser(svc installedService) error {
	u, err := user.Lookup(svc.User)
	if err != nil {
		return fmt.Errorf("the service's account %q was not found: %w", svc.User, err)
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("the account %q has no numeric id", svc.User)
	}
	// Groups first, then the group, then the user: after the user changes there is no right left to change the others.
	if err := syscall.Setgroups([]int{}); err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	if err := syscall.Setuid(uid); err != nil {
		return err
	}
	if os.Geteuid() != uid {
		return fmt.Errorf("could not become %q", svc.User)
	}
	return nil
}
