package netaudit

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrBadPorts means the port list is not a list of ports and port ranges.
var ErrBadPorts = errors.New("invalid port list")

// maxPortListLen bounds the port list the model may pass.
const maxPortListLen = 120

// maxPort is the highest TCP port number.
const maxPort = 65535

var portListRe = regexp.MustCompile(`^[0-9]{1,5}(-[0-9]{1,5})?(,[0-9]{1,5}(-[0-9]{1,5})?)*$`)

// ValidatePorts checks a port list and returns it normalised (spaces removed). Empty is allowed.
func ValidatePorts(raw string) (string, error) {
	ports := strings.ReplaceAll(raw, " ", "")
	if ports == "" {
		return "", nil
	}
	if len(ports) > maxPortListLen || !portListRe.MatchString(ports) {
		return "", fmt.Errorf("%w: use numbers and ranges such as 22,80,8000-8100", ErrBadPorts)
	}
	for _, part := range strings.Split(ports, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		first, _ := strconv.Atoi(lo)
		last := first
		if isRange {
			last, _ = strconv.Atoi(hi)
		}
		if first < 1 || last > maxPort || first > last {
			return "", fmt.Errorf("%w: %q is outside 1-%d", ErrBadPorts, part, maxPort)
		}
	}
	return ports, nil
}

// DefaultPorts are scanned when a request names none: the services a home or office network most often exposes. Both
// engines use the same list, so a quick scan means the same thing whichever one runs it.
var DefaultPorts = []int{21, 22, 23, 25, 53, 80, 110, 139, 143, 443, 445, 515, 631, 1883, 3306, 3389, 5000, 5353, 5432, 5900, 8000, 8080, 8443, 9100}

// DiscoverPorts are the ports tried to see whether an address is alive: any answer, even a refusal, means it is. Both
// engines use the same list.
var DiscoverPorts = []int{22, 80, 443, 445, 5000, 8080}

// ExpandPorts lists the ports a validated list names, in order, without duplicates. An empty list is empty.
func ExpandPorts(ports string) ([]int, error) {
	checked, err := ValidatePorts(ports)
	if err != nil || checked == "" {
		return nil, err
	}
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(checked, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		first, _ := strconv.Atoi(lo)
		last := first
		if isRange {
			last, _ = strconv.Atoi(hi)
		}
		for p := first; p <= last; p++ {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}
