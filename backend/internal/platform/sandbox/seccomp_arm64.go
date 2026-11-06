//go:build linux && arm64

package sandbox

import "golang.org/x/sys/unix"

// nativeAuditArch is the architecture the filter accepts; a call made through any other (32-bit) is killed.
const nativeAuditArch = unix.AUDIT_ARCH_AARCH64
