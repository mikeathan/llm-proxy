package radio

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Kind is the sort of radio a sensor is.
type Kind string

// The kinds of radio the stack can find. Only Wi-Fi has a driver; the others are listed so the operator sees what the computer has.
const (
	KindWifi       Kind = "wifi"
	KindBluetooth  Kind = "bluetooth"
	KindIEEE802154 Kind = "ieee802154"
)

// Mode is what a Wi-Fi adapter is doing now, read from its link type.
type Mode string

// The modes discovery can tell apart without privilege.
const (
	ModeManaged Mode = "managed"
	ModeMonitor Mode = "monitor"
	ModeOther   Mode = "other"
)

// Use says what the stack does with a capability. The words on the Sensors page come from these ids, so a new capability needs no
// new screen code.
type Use string

// The answers.
const (
	// UseReady: the radio can do it and the stack can, and it is not doing it right now (it needs the operator to start listening).
	UseReady Use = "ready"
	// UseUnsupported: this radio cannot do it, said by the adapter's own probe: a fact about the hardware, not a gap in the stack.
	UseUnsupported Use = "unsupported"
	// UseUsed: the stack uses it now.
	UseUsed Use = "used"
	// UseByDesign: the radio can do it and the stack refuses to (Constitution I.16 draft: nothing is ever sent).
	UseByDesign Use = "unused_by_design"
	// UseNotBuilt: the radio can do it, or may be able to, and the stack does not yet; this is the operator's list for later.
	UseNotBuilt Use = "unused_not_built"
	// UseNoDriver: the radio was found and nothing in the stack drives this kind of radio yet.
	UseNoDriver Use = "found_no_driver"
)

// Capability is one thing a radio can do and what the stack does about it. Known is false until the privileged probe has read it (R2): the
// system's own records say what the adapter is, not every mode and band it supports.
type Capability struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Use   Use    `json:"use"`
	Known bool   `json:"known"`
	Note  string `json:"note"`
}

// Sensor is one radio found on this computer, described from the system's own records only. Every field is a fixed word, a number or a
// plain token: nothing a device or a neighbour sent.
type Sensor struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	Bus      string `json:"bus,omitempty"`
	Driver   string `json:"driver,omitempty"`
	USBID    string `json:"usb_id,omitempty"`
	Mode     Mode   `json:"mode,omitempty"`
	Eligible bool   `json:"eligible"`
	Why      string `json:"why"`
	// Role is the one word the page leads with: listening now, can listen, will not be used, or found with no driver.
	Role Role `json:"role"`
	// Bands and Standards say what the adapter's own probe found, and are empty until it has been read.
	Probe        *ProbeSummary `json:"probe,omitempty"`
	Capabilities []Capability  `json:"capabilities"`
}

// Role is what a sensor is to the stack right now.
type Role string

// The roles.
const (
	RoleListening Role = "listening"
	RoleCanListen Role = "can_listen"
	RoleWontUse   Role = "wont_use"
	RoleNoDriver  Role = "no_driver"
)

// ProbeSummary is what the probe read about one adapter, as the page shows it.
type ProbeSummary struct {
	ReadAt    string   `json:"read_at"`
	Modes     []string `json:"modes"`
	Bands     []string `json:"bands"`
	Standards []string `json:"standards"`
	Monitor   bool     `json:"monitor"`
}

// Snapshot is what discovery found. Reason is a fixed phrase and is set when there is nothing to list.
type Snapshot struct {
	Sensors []Sensor `json:"sensors"`
	Reason  string   `json:"reason,omitempty"`
}

// The fixed phrases discovery uses.
const (
	ReasonNoSystemRecords = "this computer has no system records for wireless adapters (the Radio stack needs Linux)"
	ReasonNoAdapter       = "no wireless adapter found"
	whyEligible           = "a USB adapter that is not carrying this computer's connection"
	whyNotUSB             = "not a USB adapter: the built-in card is never used for capture"
	whyInUse              = "this computer is connected through it"
	whyNoDriver           = "found, and nothing in the stack drives this kind of radio yet"
)

// Link types the kernel reports for a network interface (ARPHRD_*).
const (
	linkEthernet  = "1"
	linkRadiotap  = "803"
	link802154    = "804"
	linkIEEE80211 = "801"
)

// token holds a value read from the system to a plain word before it is shown.
var token = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// Discover lists the radios the system's own records show, reading sys (the /sys tree) and nothing else. It sends nothing, needs no
// privilege and keeps nothing; it reads a few small files. A computer without /sys (macOS) or without a radio gives an empty list and a reason.
func Discover(sys fs.FS) Snapshot {
	entries, err := fs.ReadDir(sys, "class/net")
	if err != nil {
		entries = nil
	}
	if _, bterr := fs.Stat(sys, "class/bluetooth"); err != nil && bterr != nil {
		return Snapshot{Sensors: []Sensor{}, Reason: ReasonNoSystemRecords}
	}
	var out []Sensor
	for _, e := range entries {
		if s, ok := netSensor(sys, e.Name()); ok {
			out = append(out, s)
		}
	}
	if bt, err := fs.ReadDir(sys, "class/bluetooth"); err == nil {
		for _, e := range bt {
			if strings.HasPrefix(e.Name(), "hci") && !strings.Contains(e.Name(), ":") {
				out = append(out, bluetoothSensor(sys, e.Name()))
			}
		}
	}
	slices.SortFunc(out, func(a, b Sensor) int { return strings.Compare(string(a.Kind)+a.ID, string(b.Kind)+b.ID) })
	if len(out) == 0 {
		return Snapshot{Sensors: []Sensor{}, Reason: ReasonNoAdapter}
	}
	return Snapshot{Sensors: out}
}

// deviceDir resolves class/<group>/<name>, which the kernel makes a link into the device tree, to the path of the device in the tree.
func deviceDir(sys fs.FS, group, name string) (string, bool) {
	link := path.Join("class", group, name)
	target, err := fs.ReadLink(sys, link)
	if err != nil {
		return "", false
	}
	return path.Join(path.Dir(link), target), true
}

func netSensor(sys fs.FS, name string) (Sensor, bool) {
	dir, ok := deviceDir(sys, "net", name)
	if !ok || !token.MatchString(name) {
		return Sensor{}, false
	}
	linkType := readTrim(sys, path.Join(dir, "type"))
	switch {
	case linkType == link802154:
		return Sensor{ID: name, Kind: KindIEEE802154, Why: whyNoDriver, Role: RoleNoDriver, Capabilities: noDriverCapabilities(KindIEEE802154)}, true
	case !exists(sys, path.Join(dir, "phy80211")) && !exists(sys, path.Join(dir, "wireless")):
		return Sensor{}, false
	}
	s := Sensor{ID: name, Kind: KindWifi, Mode: modeOf(linkType)}
	s.Bus, s.Driver, s.USBID = describeDevice(sys, path.Dir(path.Dir(dir)))
	s.Eligible, s.Why = eligibility(s, readTrim(sys, path.Join(dir, "operstate")), linkType)
	s.Role = RoleWontUse
	if s.Eligible {
		s.Role = RoleCanListen
	}
	s.Capabilities = wifiCapabilities(s, nil, false)
	return s, true
}

func bluetoothSensor(sys fs.FS, name string) Sensor {
	s := Sensor{ID: name, Kind: KindBluetooth, Why: whyNoDriver, Role: RoleNoDriver, Capabilities: noDriverCapabilities(KindBluetooth)}
	if dir, ok := deviceDir(sys, "bluetooth", name); ok {
		s.Bus, s.Driver, s.USBID = describeDevice(sys, path.Dir(path.Dir(dir)))
	}
	return s
}

func modeOf(linkType string) Mode {
	switch linkType {
	case linkEthernet, linkIEEE80211:
		return ModeManaged
	case linkRadiotap:
		return ModeMonitor
	}
	return ModeOther
}

// describeDevice reads the bus, the driver and, for USB, the vendor and product ids of the interface directory iface (for example .../1-2:1.0).
func describeDevice(sys fs.FS, iface string) (bus, driver, usbID string) {
	bus = busOf(iface)
	if target, err := fs.ReadLink(sys, path.Join(iface, "driver")); err == nil && token.MatchString(path.Base(target)) {
		driver = path.Base(target)
	}
	if bus == "usb" {
		dev := path.Dir(iface)
		v, p := readTrim(sys, path.Join(dev, "idVendor")), readTrim(sys, path.Join(dev, "idProduct"))
		if token.MatchString(v) && token.MatchString(p) {
			usbID = v + ":" + p
		}
	}
	return bus, driver, usbID
}

// busOf names the bus from where the device sits in the tree: a USB device has a usbN root above it.
func busOf(dir string) string {
	for _, part := range strings.Split(dir, "/") {
		switch {
		case strings.HasPrefix(part, "usb") && len(part) > 3 && part[3] >= '0' && part[3] <= '9':
			return "usb"
		case strings.HasPrefix(part, "mmc"), strings.HasPrefix(part, "sdio"):
			return "sdio"
		}
	}
	if strings.Contains(dir, "/pci") {
		return "pci"
	}
	return ""
}

// eligibility is the rule for a radio the stack may listen with: a USB adapter, and not the one this computer's connection runs through
// (an interface that is up and in managed mode is carrying it).
func eligibility(s Sensor, operstate, linkType string) (bool, string) {
	switch {
	case s.Bus != "usb":
		return false, whyNotUSB
	case operstate == "up" && linkType != linkRadiotap:
		return false, whyInUse
	}
	return true, whyEligible
}

// wifiCapabilities says, for one Wi-Fi adapter, what each thing it might do comes to. probe is what the adapter's own probe said (nil until
// it has been read); listening is whether the stack is listening with it now. An adapter the stack will not use gets one line: the rest would
// be a list of things nobody will do.
func wifiCapabilities(s Sensor, probe *PhyCaps, listening bool) []Capability {
	found := Capability{ID: "discovery", Title: "Found and described", Use: UseUsed, Known: true, Note: "Read from the system's own records: bus, driver, mode."}
	if !s.Eligible {
		return []Capability{found, {ID: "monitor_capture", Title: "Listen to nearby Wi-Fi", Use: UseByDesign, Known: true, Note: s.Why + "."}}
	}
	return []Capability{
		found,
		monitorCapability(probe, listening),
		{ID: "frame_injection", Title: "Send frames", Use: UseByDesign, Known: true, Note: "The adapter may be able to. The Radio stack never sends anything."},
		generationCapability(probe),
		bandCapability(probe),
		rangingCapability(probe),
	}
}

func monitorCapability(probe *PhyCaps, listening bool) Capability {
	c := Capability{ID: "monitor_capture", Title: "Listen to nearby Wi-Fi", Known: probe != nil}
	switch {
	case probe != nil && !probe.Monitor():
		c.Use, c.Note = UseUnsupported, "This adapter does not offer monitor mode, so it cannot listen."
	case listening:
		c.Use, c.Known, c.Note = UseUsed, true, "Listening now, in short windows, frame headers only."
	default:
		c.Use, c.Note = UseReady, "Ready: frame headers only, in short windows, when you start listening."
	}
	return c
}

func generationCapability(probe *PhyCaps) Capability {
	c := Capability{ID: "he_decode", Title: "Read Wi-Fi 6 traffic", Known: probe != nil}
	switch {
	case probe == nil:
		c.Use, c.Note = UseNotBuilt, "Older adapters cannot decode it, so volumes on 5 GHz are undercounted. Read from the adapter when you first listen."
	case !probe.HE:
		c.Use, c.Note = UseUnsupported, "This adapter cannot decode Wi-Fi 6, so volumes on 5 GHz are undercounted. Joins, leaves and beacons are still complete."
	default:
		c.Use, c.Note = UseNotBuilt, "The adapter lists Wi-Fi 6. Whether it decodes it in monitor mode has not been measured."
	}
	return c
}

func bandCapability(probe *PhyCaps) Capability {
	c := Capability{ID: "six_ghz", Title: "6 GHz band", Known: probe != nil}
	switch {
	case probe == nil:
		c.Use, c.Note = UseNotBuilt, "Only on adapters that support it. Read from the adapter when you first listen."
	case probe.Channels6 == 0:
		c.Use, c.Note = UseUnsupported, "This adapter does not list any 6 GHz channel."
	default:
		c.Use, c.Note = UseNotBuilt, "The adapter lists 6 GHz channels. The stack does not listen there yet."
	}
	return c
}

func rangingCapability(probe *PhyCaps) Capability {
	c := Capability{ID: "ranging", Title: "Measure distance by timing", Known: probe != nil}
	switch {
	case probe == nil:
		c.Use, c.Note = UseNotBuilt, "Rare in cheap adapters. Distance comes from signal strength for now, and is only a band."
	case !probe.Ranging:
		c.Use, c.Note = UseUnsupported, "This adapter does not do timing measurement. Distance comes from signal strength, and is only a band."
	default:
		c.Use, c.Note = UseNotBuilt, "The adapter lists timing measurement. The stack does not use it yet."
	}
	return c
}

// Describe adds what the stack knows beyond the system's records: what each adapter's probe said, and which adapter is listening now. phyOf maps a
// network interface to its wireless device name. The result is a new list; sensors is not changed.
func Describe(sensors []Sensor, read *ProbeRead, listeningOn string, phyOf func(string) string) []Sensor {
	out := make([]Sensor, len(sensors))
	for i, s := range sensors {
		out[i] = s
		if s.Kind != KindWifi {
			continue
		}
		var probe *PhyCaps
		if read != nil {
			for j := range read.Phys {
				if read.Phys[j].Phy == phyOf(s.ID) && phyOf(s.ID) != "" {
					probe = &read.Phys[j]
				}
			}
		}
		listening := s.Eligible && s.ID == listeningOn
		if listening {
			out[i].Role = RoleListening
		}
		if probe != nil {
			out[i].Probe = summarise(probe, read.At.Format("2006-01-02"))
		}
		out[i].Capabilities = wifiCapabilities(s, probe, listening)
	}
	return out
}

func summarise(p *PhyCaps, readAt string) *ProbeSummary {
	sum := &ProbeSummary{ReadAt: readAt, Modes: append([]string{}, p.Modes...), Bands: []string{}, Standards: []string{}, Monitor: p.Monitor()}
	for _, b := range []struct {
		n    int
		name string
	}{{p.Channels24, "2.4 GHz"}, {p.Channels5, "5 GHz"}, {p.Channels6, "6 GHz"}} {
		if b.n > 0 {
			sum.Bands = append(sum.Bands, b.name)
		}
	}
	for _, g := range []struct {
		on   bool
		name string
	}{{p.HT, "Wi-Fi 4"}, {p.VHT, "Wi-Fi 5"}, {p.HE, "Wi-Fi 6"}, {p.EHT, "Wi-Fi 7"}} {
		if g.on {
			sum.Standards = append(sum.Standards, g.name)
		}
	}
	return sum
}

func noDriverCapabilities(k Kind) []Capability {
	switch k {
	case KindBluetooth:
		return []Capability{{ID: "ble_scan", Title: "Bluetooth LE scan", Use: UseNoDriver, Note: "Found. Nothing in the stack drives it yet."}}
	case KindIEEE802154:
		return []Capability{{ID: "zigbee_sniff", Title: "Zigbee and Thread frames", Use: UseNoDriver, Note: "Found. Nothing in the stack drives it yet."}}
	}
	return nil
}

func exists(sys fs.FS, name string) bool {
	_, err := fs.Stat(sys, name)
	return err == nil
}

func readTrim(sys fs.FS, name string) string {
	b, err := fs.ReadFile(sys, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
