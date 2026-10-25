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
