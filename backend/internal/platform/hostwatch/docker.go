package hostwatch

import (
	"os"
	"regexp"
)

// dockerBridgeName matches the two kinds of bridge Docker creates: the default one and a user-defined network's, which is
// "br-" and twelve hex digits. Another interface called something like "bridge0" is not Docker's.
var dockerBridgeName = regexp.MustCompile(`^(docker0|br-[0-9a-f]{12})$`)

// Docker tells whether Docker has a network on this computer. NetDir is /sys/class/net on a real computer.
//
// It cannot tell which listening port belongs to a container: the program that holds a published port runs as root, so its
// /proc/<pid>/fd cannot be read, and Docker's rules and socket need root or the docker group. A bridge is only a reason to say
// that a published port would not be covered by the firewall, never a claim about one port.
type Docker struct {
	NetDir string
}

// Bridge reports whether a Docker bridge interface exists. A directory that cannot be read, or none configured, says nothing
// about Docker, so it reports false.
func (d Docker) Bridge() bool {
	if d.NetDir == "" {
		return false
	}
	entries, err := os.ReadDir(d.NetDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if dockerBridgeName.MatchString(e.Name()) {
			return true
		}
	}
	return false
}
