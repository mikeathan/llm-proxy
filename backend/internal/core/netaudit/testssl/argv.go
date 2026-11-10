package testssl

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Target is the one service a look is run against: an address and a port, never a name. The caller has already checked the address against the
// declared scan scope.
type Target struct {
	Addr netip.Addr
	Port uint16
}

// ErrBadTarget means the target or the output file is not something Args will put on a command line.
var ErrBadTarget = errors.New("testssl: not a usable target")

// Args is the whole argument list, built in code: the protocols (-p) and the standard cipher categories (-s), quiet, in plain text, with the
// result written as flat JSON to jsonFile. It never asks for the vulnerability checks, which send crafted packets and belong to the deep check
// (ADR-0012), nor for the certificate, which webprobe already checks, nor for per-connection timeouts: those make testssl.sh need a `timeout`
// program, which macOS does not have in the jail's PATH, so the runner's own time limit bounds the look instead. Nothing here comes from a model or a request except the address and port.
func Args(t Target, jsonFile string) ([]string, error) {
	switch {
	case !t.Addr.IsValid() || t.Addr.Zone() != "":
		return nil, fmt.Errorf("%w: the address must be a plain IP address", ErrBadTarget)
	case t.Port == 0:
		return nil, fmt.Errorf("%w: the port must be 1 to 65535", ErrBadTarget)
	case jsonFile == "" || strings.HasPrefix(jsonFile, "-"):
		return nil, fmt.Errorf("%w: the output file must be a path that does not start with a dash", ErrBadTarget)
	}
	return []string{
		"--quiet", "--color", "0", "--warnings", "off",
		"--jsonfile", jsonFile, "--overwrite",
		"-p", "-s",
		hostPort(t),
	}, nil
}

// hostPort writes the target the way testssl.sh reads it: an IPv6 address in brackets, so the port is not taken as part of it.
func hostPort(t Target) string {
	port := strconv.Itoa(int(t.Port))
	if t.Addr.Is6() && !t.Addr.Is4In6() {
		return "[" + t.Addr.String() + "]:" + port
	}
	return t.Addr.Unmap().String() + ":" + port
}
