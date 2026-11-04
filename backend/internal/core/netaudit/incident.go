package netaudit

import (
	"strings"
)

// CheckDeviceIncident is one device whose decoy touch is still in the report (the last DecoyReportFor) and that, in this audit,
// shows another sign of trouble: it is a device nobody approved that is active now, it looks like one kind of device but offers
// another's services, its address was taken over from a device the operator approved, or it opened a port since the last audit. Each alone is a lower finding; together they are the likeliest sign of a
// compromised or hostile device, so they are told as one story. It is built from the findings the audit already has: no probe.
const CheckDeviceIncident = "device_incident"

// incidentSignals are the findings that join a decoy touch, with the fixed words for each.
var incidentSignals = []struct {
	check string
	words string
}{
	{CheckNewDevice, "it is a new device nobody has approved"},
	{CheckClassMismatch, "it looks like one kind of device but offers the services of another kind"},
	{CheckMACChanged, "its address now belongs to a different device than the one you approved"},
}

const openedPortWords = "it opened a port since the last audit"

// IncidentFindings are the incidents among the current findings; previous are the last audit's, for the ports that opened. A
// device that had no finding in the last audit has no ports to compare with, so its ports do not count as opened (a new device
// is its own signal).
func IncidentFindings(current, previous []Finding) []Finding {
	touched := map[string]bool{}
	var order []string
	for _, f := range current {
		if f.Check == CheckDecoyTouched && !touched[f.Device] {
			touched[f.Device] = true
			order = append(order, f.Device)
		}
	}
	if len(order) == 0 {
		return nil
	}
	var out []Finding
	for _, device := range order {
		if words := incidentWords(device, current, previous); len(words) > 0 {
			out = append(out, Finding{
				Device: device, Check: CheckDeviceIncident, Severity: SeverityHigh,
				Evidence: "This device connected to a decoy service (in the last 30 days), and in this audit " + strings.Join(words, "; ") +
					". Together these are the likeliest sign of a compromised or hostile device",
			})
		}
	}
	return out
}

// incidentWords lists the signals that joined the touch of one device, in a fixed order.
func incidentWords(device string, current, previous []Finding) []string {
	var words []string
	for _, s := range incidentSignals {
		if hasCheck(current, device, s.check) {
			words = append(words, s.words)
		}
	}
	if portOpened(device, current, previous) {
		words = append(words, openedPortWords)
	}
	return words
}

func hasCheck(findings []Finding, device, check string) bool {
	for _, f := range findings {
		if f.Device == device && f.Check == check {
			return true
		}
	}
	return false
}

// portOpened reports whether the device has an open port now that it did not have in the last audit, which saw the device.
func portOpened(device string, current, previous []Finding) bool {
	before := map[string]bool{}
	seen := false
	for _, f := range previous {
		if f.Device == device {
			seen = true
			if f.Check == CheckOpenPort {
				before[f.Key()] = true
			}
		}
	}
	if !seen {
		return false
	}
	for _, f := range current {
		if f.Device == device && f.Check == CheckOpenPort && !before[f.Key()] {
			return true
		}
	}
	return false
}

func init() {
	checkTitles[CheckDeviceIncident] = "A device that probed the decoys shows other signs of trouble"
	remediations[CheckDeviceIncident] = "Find the device (its address and hardware address are on the Devices tab). If you do not know it, block it on the router and change the Wi-Fi password; if you do, treat it as compromised: update or reset it before it joins the network again."
	guides[CheckDeviceIncident] = FixGuide{
		Steps: []string{
			"Open the device on the Devices tab: the other findings on it say which signals joined the decoy touch.",
			"If you do not know the device, block it on the router (most routers list connected devices with a block button) and change the Wi-Fi password, since it may have it.",
			"If it is yours, treat it as compromised: disconnect it, update it or reset it to factory settings, and change the passwords it held.",
			"If you probed the decoys yourself from that device (a scan you ran), accept the finding with a note.",
		},
		Risk: "Blocking a device you need cuts it off until you unblock it on the router.",
		Undo: "Unblock the device on the router.",
	}
}
