package netaudit

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
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
var DefaultPorts = []int{21, 22, 23, 25, 53, 80, 110, 139, 143, 443, 445, 515, 631, 1883, 3306, 3389, 5000, 5353, 5432, 5900, 8000, 8080, 8443, 9100, 2375, 11211, 27017, 873, 1080, 2323, 2379, 3128, 5555, 6000, 9200, 10250}

// ThoroughPorts are scanned by a thorough request that names none: nmap 7.99's 300 most common TCP ports, plus 2323, 2375,
// 2379, 9200, 10250, 11211 and 27017 (an alternate Telnet port, an unprotected Docker API, etcd, Elasticsearch, a Kubernetes node,
// memcached and MongoDB), which the checks look for and which are not all among them. It is a list of our own rather
// than nmap's --top-ports, because nmap ignores --top-ports when -p is also given, so the extra ports could not be added
// to it. Measured at about a minute per device that drops probes.
var ThoroughPorts = []int{
	1, 3, 7, 9, 13, 17, 19, 20, 21, 22, 23, 24, 25, 26, 33, 37,
	42, 53, 79, 80, 81, 82, 88, 100, 106, 110, 111, 113, 119, 135, 139, 143,
	144, 161, 179, 199, 222, 254, 255, 264, 280, 311, 366, 389, 407, 427, 443, 444,
	445, 464, 465, 497, 500, 512, 513, 514, 515, 543, 544, 548, 554, 563, 587, 593,
	625, 631, 636, 646, 787, 808, 873, 888, 902, 990, 992, 993, 995, 999, 1000, 1022,
	1023, 1024, 1025, 1026, 1027, 1028, 1029, 1030, 1031, 1032, 1033, 1034, 1035, 1036, 1037, 1038,
	1039, 1040, 1041, 1042, 1043, 1044, 1047, 1048, 1049, 1050, 1051, 1052, 1053, 1054, 1055, 1056,
	1058, 1059, 1060, 1062, 1064, 1065, 1066, 1067, 1068, 1069, 1071, 1074, 1080, 1110, 1111, 1218,
	1234, 1311, 1352, 1433, 1494, 1521, 1700, 1717, 1720, 1723, 1755, 1761, 1801, 1900, 1935, 1998,
	2000, 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010, 2049, 2065, 2103, 2105, 2107,
	2121, 2161, 2301, 2323, 2375, 2379, 2383, 2401, 2601, 2602, 2604, 2701, 2702, 2717, 2869, 2967,
	3000, 3001, 3052, 3128, 3260, 3268, 3269, 3283, 3306, 3333, 3389, 3689, 3690, 3703, 3986, 4000,
	4001, 4002, 4045, 4443, 4444, 4662, 4899, 5000, 5001, 5002, 5003, 5009, 5050, 5051, 5060, 5100,
	5101, 5120, 5190, 5225, 5226, 5357, 5432, 5550, 5555, 5631, 5666, 5800, 5801, 5900, 5901, 5902,
	6000, 6001, 6002, 6004, 6059, 6112, 6543, 6646, 6666, 6789, 7000, 7001, 7019, 7070, 7100, 7937,
	7938, 8000, 8002, 8008, 8009, 8010, 8031, 8080, 8081, 8082, 8089, 8192, 8193, 8194, 8443, 8651,
	8652, 8701, 8888, 9000, 9001, 9050, 9090, 9100, 9102, 9200, 9415, 9535, 9593, 9594, 9595, 9999,
	10000, 10001, 10010, 10250, 11211, 13782, 15000, 16992, 16993, 20828, 23502, 27017, 32768, 32769, 32770, 32771,
	32772, 33354, 35500, 42510, 49152, 49153, 49154, 49155, 49156, 49157, 50000, 50001, 52869, 55555, 55600, 64623,
	64680, 65000, 65389,
}

// DiscoverPorts are the ports tried to see whether an address is alive: any answer, even a refusal, means it is. Both
// engines use the same list.
var DiscoverPorts = []int{22, 80, 443, 445, 5000, 8080}

// ThoroughDiscoverPorts are the liveness ports of a thorough sweep: the quick ones plus every default port, so a device
// that answers on only one common service is still found. A device that answers on none of them is not.
var ThoroughDiscoverPorts = func() []int {
	out := append([]int{}, DiscoverPorts...)
	for _, p := range DefaultPorts {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}()

// LivenessPorts returns the ports a sweep tries, for a thorough scan or a quick one.
func LivenessPorts(thorough bool) []int {
	if thorough {
		return ThoroughDiscoverPorts
	}
	return DiscoverPorts
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
