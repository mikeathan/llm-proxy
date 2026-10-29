//go:build unix

package boot

import (
	"fmt"
	"os"
	"syscall"
)

// ownerOfFolder returns the account that owns a folder, and false when it does not exist or cannot be read.
func ownerOfFolder(path string) (uid, gid int, ok bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	st, isUnix := fi.Sys().(*syscall.Stat_t)
	if !isUnix {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}

// becomeUserID makes this process another account, so a file it writes is owned by the account that will read it. It
// changes this process's identity (nothing is started) and only root may.
func becomeUserID(uid, gid int) error {
	// Groups first, then the group, then the user: once the user changes there is no right left to change the others.
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
		return fmt.Errorf("could not become user %d", uid)
	}
	return nil
}
