//go:build !linux

package providers

import "syscall"

func setPdeathsig(attr *syscall.SysProcAttr) {
}
