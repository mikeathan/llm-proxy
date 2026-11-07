//go:build linux

package helper

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// soPeerPIDFD is SO_PEERPIDFD (Linux 6.5 and later); golang.org/x/sys does not export it yet. It is 77 on every architecture the build
// supports.
const soPeerPIDFD = 77

// IdentifyPeer reads who is on the other end of a Unix connection. It takes a process handle (SO_PEERPIDFD) when the kernel has one and reads the pid
// from it, so the pid cannot be reused before it is compared; on an older kernel it falls back to SO_PEERCRED's pid, which can in
// principle be reused by another process after the peer exits (the design names that race). When both are available they must agree.
func IdentifyPeer(conn net.Conn) (Peer, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return Peer{}, fmt.Errorf("peer: not a Unix connection")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return Peer{}, fmt.Errorf("peer: %w", err)
	}
	var cred *unix.Ucred
	var pidfd int
	var credErr, fdErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		pidfd, fdErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, soPeerPIDFD)
	}); err != nil {
		return Peer{}, fmt.Errorf("peer: %w", err)
	}
	if credErr != nil {
		return Peer{}, fmt.Errorf("peer credentials: %w", credErr)
	}
	peer := Peer{PID: int(cred.Pid), UID: int(cred.Uid)}
	if fdErr != nil {
		return peer, nil // no process handle on this kernel: SO_PEERCRED is the whole answer
	}
	defer unix.Close(pidfd)
	pid, err := pidOfHandle(pidfd)
	if err != nil {
		return Peer{}, err
	}
	if pid != peer.PID {
		return Peer{}, fmt.Errorf("peer: the process handle (pid %d) and the credentials (pid %d) disagree", pid, peer.PID)
	}
	return peer, nil
}

// pidOfHandle reads the pid a process handle refers to from /proc/self/fdinfo; the pid is -1 once the process has gone.
func pidOfHandle(fd int) (int, error) {
	f, err := os.Open("/proc/self/fdinfo/" + strconv.Itoa(fd))
	if err != nil {
		return 0, fmt.Errorf("peer: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "Pid:"); ok {
			pid, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil || pid <= 0 {
				return 0, fmt.Errorf("peer: the process has gone")
			}
			return pid, nil
		}
	}
	return 0, fmt.Errorf("peer: no pid in the process handle")
}
