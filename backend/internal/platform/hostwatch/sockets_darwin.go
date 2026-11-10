//go:build darwin

package hostwatch

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// defaultPortrangeFirst is macOS's documented start of the dynamic port range (net.inet.ip.portrange.first), used when it cannot be read.
const defaultPortrangeFirst = 49152

// systemListeners reads the kernel's own socket tables with sysctl, in process: no program is started, no socket is opened and nothing is sent.
// That is what netstat does too, so it needs no confinement opt-in. A refusal or a layout it does not know is ErrUnavailable.
func systemListeners() ([]Listener, error) {
	tcp, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	udp, err := unix.SysctlRaw("net.inet.udp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	low := defaultPortrangeFirst
	if n, err := unix.SysctlUint32("net.inet.ip.portrange.first"); err == nil && n >= 1 && n <= 65535 {
		low = int(n)
	}
	return listenersFromPCB(tcp, udp, os.Getpid(), low)
}
