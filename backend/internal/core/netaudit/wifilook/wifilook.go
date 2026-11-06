// Package wifilook lists the Wi-Fi access points this computer's NetworkManager already knows, by running nmcli as a guarded program
// (Constitution II.3, proposed amendment; ADR-0031), so a copy of the operator's network name can be told from the operator's own
// access points. The list is read, never asked for: the arguments are fixed here, nmcli is told not to scan (`--rescan no`), and what
// it prints is parsed as untrusted text, because anyone within radio range chooses the names.
package wifilook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"llm-proxy/internal/platform/progexec"
)

// Program is how nmcli is found and what to say when it is not there.
var Program = progexec.Program{
	Name:        "nmcli",
	Binary:      "nmcli",
	VersionArgs: []string{"--version"},
	InstallHint: map[string]string{"linux": "sudo apt install network-manager"},
}

// listArgs are the whole of the command line: terse output of four fields, from NetworkManager's own list, with no scan asked for. A
// rescan needs a permission the service does not have (polkit says "auth" for it), and a look must change nothing anyway.
var listArgs = []string{"-t", "-f", "SSID,BSSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "no"}

const (
	// MaxAPs bounds what one look keeps, however many networks a hostile neighbourhood advertises.
	MaxAPs = 512
	// MaxSSID is the longest name 802.11 allows, in bytes.
	MaxSSID = 32

	runDirPattern = "netaudit-wifi-*"
	listTimeout   = 15 * time.Second
	maxOutput     = 256 << 10
)

// Security is what an access point says it uses, reduced to the one word the checks need.
type Security string

const (
	Open  Security = "open"
	WEP   Security = "wep"
	WPA   Security = "wpa"
	WPA2  Security = "wpa2"
	WPA3  Security = "wpa3"
	Other Security = "other" // something nmcli names that this does not (for example OWE); never judged weak
)

// Rank orders the securities from none to the best; Other ranks with WPA2 so that an unknown word is never read as "open".
func (s Security) Rank() int {
	switch s {
	case Open:
		return 0
	case WEP:
		return 1
	case WPA:
		return 2
	case WPA3:
		return 4
	}
	return 3
}

// Weak says the access point protects nothing worth the name: no security, or WEP.
func (s Security) Weak() bool { return s.Rank() <= WEP.Rank() }

// AP is one access point. SSID is empty for a hidden network and for a name that cannot be a real one.
type AP struct {
	SSID     string
	BSSID    string // lower-case, colon separated
	Signal   int    // 0 to 100
	Security Security
}

var bssidPattern = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

// Parse reads nmcli's terse output: one access point per line, four fields, ':' and '\' escaped with a backslash. A line that is not
// exactly that shape is dropped, a name that cannot be a real one (over 32 bytes, not UTF-8, a control character) becomes the empty
// name so it can never equal the operator's, and at most MaxAPs are kept.
func Parse(out []byte) []AP {
	var aps []AP
	for _, line := range strings.Split(string(out), "\n") {
		if len(aps) >= MaxAPs {
			break
		}
		fields, ok := splitTerse(line)
		if !ok || len(fields) != 4 {
			continue
		}
		bssid := strings.ToLower(fields[1])
		signal, err := strconv.Atoi(fields[2])
		if !bssidPattern.MatchString(bssid) || err != nil || signal < 0 || signal > 100 {
			continue
		}
		aps = append(aps, AP{SSID: realName(fields[0]), BSSID: bssid, Signal: signal, Security: securityOf(fields[3])})
	}
	return aps
}

// splitTerse splits on unescaped colons and removes the escapes. A trailing lone backslash is not a valid line.
func splitTerse(line string) ([]string, bool) {
	var fields []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '\\':
			if i+1 >= len(line) {
				return nil, false
			}
			i++
			cur.WriteByte(line[i])
		case ':':
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(fields, cur.String()), line != ""
}

// realName is the name if it could be one, else "".
func realName(s string) string {
	if len(s) > MaxSSID || !utf8.ValidString(s) {
		return ""
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return s
}

func securityOf(flags string) Security {
	f := strings.ToUpper(flags)
	switch {
	case strings.TrimSpace(f) == "":
		return Open
	case strings.Contains(f, "WPA3"):
		return WPA3
	case strings.Contains(f, "WPA2"):
		return WPA2
	case strings.Contains(f, "WPA"):
		return WPA
	case strings.Contains(f, "WEP"):
		return WEP
	}
	return Other
}

// Runner starts guarded programs; progexec.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, p progexec.Program, inv progexec.Invocation) (progexec.Output, error)
}

// Lister reads the access points through a guarded-program runner.
type Lister struct{ runner Runner }

func New(runner Runner) Lister { return Lister{runner: runner} }

// Installed says why nmcli cannot be used (nil when it is on the PATH). It only looks the program up: running it, even for its
// version, happens only inside the sandbox.
func Installed() error {
	if _, err := exec.LookPath(Program.Binary); err != nil {
		return &progexec.NotInstalledError{Program: Program.Name, Hint: Program.InstallHint[runtime.GOOS]}
	}
	return nil
}

// List runs nmcli once. The program gets a private run folder that is removed afterwards, a clean environment (nothing that could steer
// where it connects), no network, and Unix sockets, which it needs to reach NetworkManager over D-Bus. A failed run is an error, never
// an empty list: "no networks nearby" and "could not look" must not read the same.
func (l Lister) List(ctx context.Context) ([]AP, error) {
	root, err := os.MkdirTemp("", runDirPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to create the Wi-Fi look's folder: %w", err)
	}
	defer os.RemoveAll(root)
	out, err := l.runner.Run(ctx, Program, progexec.Invocation{
		Args:             append([]string(nil), listArgs...),
		Env:              progexec.CleanEnv(root),
		Root:             root,
		NoNetwork:        true,
		AllowUnixSockets: true,
		Timeout:          listTimeout,
		MaxOutput:        maxOutput,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the Wi-Fi networks: %w", err)
	}
	return Parse(out.Stdout), nil
}

// Network is one of the operator's own network names with the hardware addresses the operator confirmed are theirs.
type Network struct {
	Name      string
	Addresses []string
}

// Lookalike is an access point that advertises one of the operator's names from a hardware address the operator did not confirm.
type Lookalike struct {
	Name     string
	BSSID    string
	Signal   int
	Security Security
}

// Lookalikes finds the access points that use one of the operator's names without being confirmed, sorted by hardware address. A name
// is judged only once the operator has confirmed at least one address for it: before that every address, the operator's own among them,
// would look like a copy. The name must match byte for byte: a hidden network (no name), another case or a trailing space is a different
// name, and the neighbours' networks are never looked at.
func Lookalikes(aps []AP, nets []Network) []Lookalike {
	var out []Lookalike
	seen := map[string]bool{}
	for _, n := range nets {
		if len(n.Addresses) == 0 {
			continue
		}
		mine := map[string]bool{}
		for _, a := range n.Addresses {
			mine[a] = true
		}
		for _, a := range aps {
			if a.SSID == "" || a.SSID != n.Name || mine[a.BSSID] || seen[a.BSSID] {
				continue
			}
			seen[a.BSSID] = true
			out = append(out, Lookalike{Name: n.Name, BSSID: a.BSSID, Signal: a.Signal, Security: a.Security})
		}
	}
	slices.SortFunc(out, func(a, b Lookalike) int { return strings.Compare(a.BSSID, b.BSSID) })
	return out
}
