package radio

import (
	"bufio"
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// The read of what an adapter can do: the output of `iw list`, which the privileged radio_probe action returns and which is parsed here, in the
// unprivileged stack, with fixed bounds (Constitution I.11: text a driver produced is untrusted). Nothing in the output is kept as text: every
// field of PhyCaps is a number, a flag or a word from a fixed list, so a driver that prints something odd can only produce fewer facts.
const (
	// maxProbeBytes is the most output read; a real `iw list` is a few tens of kilobytes.
	maxProbeBytes = 512 << 10
	maxProbeLine  = 512
	maxPhys       = 16
)

// PhyCaps is what one wireless device (a "phy") says it can do.
type PhyCaps struct {
	Phy   string   `json:"phy"`
	Modes []string `json:"modes"`
	// Channels counts the enabled channels on each band; a band with none is not listed by the adapter (or is all disabled).
	Channels24 int `json:"channels_24"`
	Channels5  int `json:"channels_5"`
	Channels6  int `json:"channels_6"`
	// HT, VHT, HE and EHT are the Wi-Fi generations the adapter lists: 4, 5, 6 and 7. An adapter with no HE cannot decode Wi-Fi 6 traffic.
	HT      bool `json:"ht"`
	VHT     bool `json:"vht"`
	HE      bool `json:"he"`
	EHT     bool `json:"eht"`
	Ranging bool `json:"ranging"`
}

// Monitor says the adapter lists monitor mode, which listening needs.
func (p PhyCaps) Monitor() bool { return slices.Contains(p.Modes, "monitor") }

var (
	phyLine   = regexp.MustCompile(`^Wiphy (phy[0-9]{1,4})$`)
	modeLine  = regexp.MustCompile(`^\* ([A-Za-z0-9_/ -]{1,24})$`)
	freqLine  = regexp.MustCompile(`^\* ([0-9]{4,5}) MHz \[[0-9]{1,3}\]`)
	rangeLine = regexp.MustCompile(`FTM_(RESPONDER|INITIATOR)`)
)

// knownModes are the interface modes iw prints that the page may show. Anything else is dropped, never shown.
var knownModes = []string{"IBSS", "managed", "AP", "AP/VLAN", "WDS", "monitor", "mesh point", "P2P-client", "P2P-GO", "P2P-device", "OCB", "NAN"}

// ParseProbe reads `iw list` output. It never fails: input that is not iw's gives no phys.
func ParseProbe(raw []byte) []PhyCaps {
	if len(raw) > maxProbeBytes {
		raw = raw[:maxProbeBytes]
	}
	var (
		out     []PhyCaps
		cur     *PhyCaps
		inModes bool
	)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, maxProbeLine), maxProbeLine)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := phyLine.FindStringSubmatch(line); m != nil {
			if len(out) >= maxPhys {
				break
			}
			out = append(out, PhyCaps{Phy: m[1], Modes: []string{}})
			cur, inModes = &out[len(out)-1], false
			continue
		}
		if cur == nil {
			continue
		}
		inModes = cur.readLine(line, inModes)
	}
	return out
}

// readLine applies one trimmed line to the phy and says whether the next line still belongs to the list of interface modes.
func (p *PhyCaps) readLine(line string, inModes bool) bool {
	switch {
	case line == "Supported interface modes:":
		return true
	case inModes:
		if m := modeLine.FindStringSubmatch(line); m != nil {
			if slices.Contains(knownModes, m[1]) && !slices.Contains(p.Modes, m[1]) {
				p.Modes = append(p.Modes, m[1])
			}
			return true
		}
		return false
	case strings.HasPrefix(line, "HT Max RX data rate"), strings.HasPrefix(line, "HT TX/RX MCS rate indexes"):
		p.HT = true
	case strings.HasPrefix(line, "VHT Capabilities"):
		p.VHT = true
	case strings.HasPrefix(line, "HE Iftypes"), strings.HasPrefix(line, "HE MAC Capabilities"):
		p.HE = true
	case strings.HasPrefix(line, "EHT Iftypes"), strings.HasPrefix(line, "EHT MAC Capabilities"):
		p.EHT = true
	case rangeLine.MatchString(line):
		p.Ranging = true
	default:
		p.readChannel(line)
	}
	return false
}

func (p *PhyCaps) readChannel(line string) {
	m := freqLine.FindStringSubmatch(line)
	if m == nil || strings.Contains(line, "(disabled)") {
		return
	}
	mhz := 0
	for _, c := range m[1] {
		mhz = mhz*10 + int(c-'0')
	}
	switch {
	case mhz >= 2300 && mhz < 2600:
		p.Channels24++
	case mhz >= 4900 && mhz < 5900:
		p.Channels5++
	case mhz >= 5925 && mhz <= 7125:
		p.Channels6++
	}
}
