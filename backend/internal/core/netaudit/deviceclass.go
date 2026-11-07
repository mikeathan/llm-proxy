package netaudit

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"llm-proxy-stacks/network/oui"
	"llm-proxy/models"
)

// CheckClassMismatch is a device that looks like a kind that has no use for an administration service, and has one open.
const CheckClassMismatch = "class_mismatch"

// DeviceClass is a kind of device the program is willing to name. The table is closed: a class never comes from text a device
// sent, only from a registry maker, a scan port and a service type that matches a fixed list.
type DeviceClass string

const (
	ClassCamera    DeviceClass = "camera"
	ClassPrinter   DeviceClass = "printer"
	ClassMedia     DeviceClass = "media_player"
	ClassSmartHome DeviceClass = "smart_home"
)

// ClassEvidence is everything a class is judged from.
type ClassEvidence struct {
	// Vendor is the maker from the bundled registry, empty for a randomized address.
	Vendor string
	// Services are the service types the device announced (Nearby), already checked for shape.
	Services []string
	// Ports are the TCP ports the last finished scan found open.
	Ports []int
}

// classServices are service types that name a kind. A type shared with computers and phones (AirPlay, Spotify Connect, SMB) is
// deliberately absent: a Mac that casts audio and has remote login on is not a mismatch.
var classServices = map[string]DeviceClass{
	"_rtsp._tcp": ClassCamera, "_axis-video._tcp": ClassCamera,
	"_ipp._tcp": ClassPrinter, "_ipps._tcp": ClassPrinter, "_printer._tcp": ClassPrinter, "_pdl-datastream._tcp": ClassPrinter,
	"_scanner._tcp": ClassPrinter, "_uscan._tcp": ClassPrinter,
	"_googlecast._tcp": ClassMedia, "_androidtvremote2._tcp": ClassMedia,
	"_esphomelib._tcp": ClassSmartHome, "_hap._tcp": ClassSmartHome,
}

// classPorts are TCP ports that only that kind of device listens on.
var classPorts = map[int]DeviceClass{554: ClassCamera, 8554: ClassCamera, 9100: ClassPrinter, 631: ClassPrinter, 515: ClassPrinter}

// classMakers are whole words of a maker's registry name for makers that build one kind of device. A word has to be the whole
// word: "Brotherhood" is not "Brother".
var classMakers = map[string]DeviceClass{
	"hikvision": ClassCamera, "dahua": ClassCamera, "reolink": ClassCamera, "amcrest": ClassCamera, "foscam": ClassCamera,
	"uniview": ClassCamera, "ezviz": ClassCamera, "annke": ClassCamera, "lorex": ClassCamera,
	"brother": ClassPrinter, "lexmark": ClassPrinter, "kyocera": ClassPrinter, "xerox": ClassPrinter, "ricoh": ClassPrinter, "epson": ClassPrinter,
	"roku": ClassMedia, "vizio": ClassMedia, "sonos": ClassMedia,
	"espressif": ClassSmartHome, "tuya": ClassSmartHome, "shelly": ClassSmartHome, "allterco": ClassSmartHome, "signify": ClassSmartHome,
}

// classAdminPorts are the administration services a kind does not normally need. An open one is how such devices are taken over.
var classAdminPorts = map[DeviceClass][]int{
	ClassCamera:    {22, 23, 445, 3389, 5900},
	ClassPrinter:   {22, 23, 3389, 5900},
	ClassMedia:     {22, 23, 3389, 5900},
	ClassSmartHome: {22, 23, 445, 3389, 5900},
}

var adminPortWords = map[int]string{
	22: "remote login (SSH, port 22)", 23: "Telnet (port 23)", 445: "file sharing (SMB, port 445)",
	3389: "remote desktop (port 3389)", 5900: "screen sharing (VNC, port 5900)",
}

var classPhrases = map[DeviceClass]string{
	ClassCamera: "a camera", ClassPrinter: "a printer", ClassMedia: "a media player", ClassSmartHome: "a smart-home device",
}

// ClassOf names the kind of a device, or "" when the evidence names none or names two. Two signals of one kind agree; two kinds
// are a disagreement, and a disagreement is not guessed at.
func ClassOf(e ClassEvidence) DeviceClass {
	kinds := map[DeviceClass]bool{}
	for _, s := range e.Services {
		if c, ok := classServices[strings.ToLower(s)]; ok {
			kinds[c] = true
		}
	}
	for _, p := range e.Ports {
		if c, ok := classPorts[p]; ok {
			kinds[c] = true
		}
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(e.Vendor), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if c, ok := classMakers[w]; ok {
			kinds[c] = true
		}
	}
	if len(kinds) != 1 {
		return ""
	}
	for c := range kinds {
		return c
	}
	return ""
}

// ClassMismatchFindings reports each device that looks like a camera, printer, media player or smart-home device and has an
// administration service open, in one finding that lists the services. It reads the open TCP ports of hosts the scan finished,
// the maker from the registry and the service types Nearby kept; a host the scan did not finish says nothing.
func ClassMismatchFindings(devices []models.NetworkDevice, hosts []Host, nearby models.NearbyState) []Finding {
	macByIP := make(map[string]string, len(devices))
	for _, d := range devices {
		if d.IP != "" {
			macByIP[d.IP] = d.MAC
		}
	}
	servicesByIP := make(map[string][]string, len(nearby.Devices))
	for _, d := range nearby.Devices {
		servicesByIP[d.IP] = d.Services
	}
	var out []Finding
	for _, h := range hosts {
		if !h.Up || h.Unscanned {
			continue
		}
		ip := h.Addr.String()
		ports := openTCPPorts(h)
		class := ClassOf(ClassEvidence{Vendor: oui.Vendor(macByIP[ip]), Services: servicesByIP[ip], Ports: ports})
		if class == "" {
			continue
		}
		var open []string
		for _, p := range classAdminPorts[class] {
			if slices.Contains(ports, p) {
				open = append(open, adminPortWords[p])
			}
		}
		if len(open) > 0 {
			out = append(out, Finding{Device: ip, Check: CheckClassMismatch, Severity: SeverityMedium, Evidence: classMismatchEvidence(class, open)})
		}
	}
	return out
}

func openTCPPorts(h Host) []int {
	var ports []int
	for _, p := range h.Ports {
		if p.State == "open" && p.Proto == "tcp" {
			ports = append(ports, p.Number)
		}
	}
	return ports
}

func classMismatchEvidence(class DeviceClass, open []string) string {
	return fmt.Sprintf("This looks like %s, judged from its maker, the ports it has open and the services it announces. It has %s open, which this kind of device does not normally need and which is a common way for such devices to be taken over", classPhrases[class], strings.Join(open, " and "))
}

func init() {
	checkTitles[CheckClassMismatch] = "A device has a service open that its kind does not need"
	remediations[CheckClassMismatch] = "Turn the service off in the device's own settings, or move the device to a guest network that cannot reach the rest of the house. If you set it up on purpose, accept the finding with a note."
	guides[CheckClassMismatch] = classMismatchGuide
}

var classMismatchGuide = FixGuide{
	Steps: []string{
		"Find the device (its address is in the finding) and check that it is what the finding says it looks like. The program judges from the maker, the ports and the services the device announces, so it can be wrong.",
		"Open the device's settings or its app and switch off the remote login, Telnet, file sharing, remote desktop or screen sharing named in the finding. Most of these devices never need them.",
		"If it cannot be switched off, put the device on a guest network, or block it from the rest of the house in the router, so a taken-over device cannot reach your computers.",
		"Update the device's firmware, and change its default password if it has one.",
		"If you turned the service on yourself, accept the finding with a note saying why.",
	},
	Commands: []GuideCommand{
		{platformLinux, "nmap -Pn -p 22,23,445,3389,5900 <the device's address>"},
	},
	Risk: "Switching a service off can stop an app or a service you use from reaching the device; a device on a guest network can no longer be reached from the rest of the house.",
	Undo: "Switch the service on again in the device's settings, or move the device back to the main network.",
}
