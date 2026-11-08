package netaudit

import (
	"fmt"

	"llm-proxy/models"
)

// CheckUDPService is a UDP service that answered a scan the operator ran through the privileged helper, on a port whose open state is itself
// worth knowing about.
const CheckUDPService = "udp_service"

const protoUDPFinding = "udp"

// stateAnswered is the only UDP state that makes a finding: the port replied. "open|filtered" means no reply came back, which says nothing
// about whether the port is open, so it never makes one.
const stateAnswered = "open"

// probeDateLayout is the form of the scan date in the evidence.
const probeDateLayout = "2006-01-02"

// udpRule is one UDP port that is reported when it answers: the fixed name for it and how serious it is. The name is written here and never
// taken from nmap's service name, so no text from a device or from nmap reaches a finding.
type udpRule struct {
	Name     string
	Severity string
	Why      string
}

// udpRules are the ports judged. Three are chosen because an answer on them is a known weakness of an ordinary home device, not just a
// service being there. The OS guesses are not judged: nmap's guess is too uncertain to raise an alert on.
var udpRules = map[int]udpRule{
	161:  {"SNMP", SeverityMedium, "SNMP often answers to a default password (\"public\") and then gives away how the device is set up"},
	69:   {"TFTP", SeverityMedium, "TFTP hands out files with no password at all"},
	1900: {"UPnP", SeverityLow, "UPnP lets other devices on the network ask this one to open ports to the internet"},
}

// ProbeFindings reports each device whose last UDP scan (kept on the device, run by the operator) got an answer on one of the judged ports. It
// reads typed fields only: the port number, the fixed rule and the date of the scan. A device that was never scanned says nothing, which
// is not a clean result, and the Network page says how many devices have one. The finding stays while the kept result does: a scan the
// operator has not repeated is not a scan that found it gone.
func ProbeFindings(devices []models.NetworkDevice) []Finding {
	var out []Finding
	for _, d := range devices {
		if d.IP == "" || d.UDPScannedAt.IsZero() {
			continue
		}
		for _, p := range d.UDPPorts {
			rule, judged := udpRules[p.Port]
			if !judged || p.State != stateAnswered {
				continue
			}
			out = append(out, Finding{
				Device: d.IP, Port: p.Port, Proto: protoUDPFinding, Service: rule.Name, Check: CheckUDPService, Severity: rule.Severity,
				Evidence: fmt.Sprintf("%s answered a UDP scan on %s. %s", rule.Name, d.UDPScannedAt.Format(probeDateLayout), rule.Why),
			})
		}
	}
	return out
}

func init() {
	checkTitles[CheckUDPService] = "A UDP service answered on a device"
	remediations[CheckUDPService] = "Turn the service off in the device's own settings if you do not use it. If you do, change its default password or restrict who may reach it. If you set it up on purpose, accept the finding with a note."
	guides[CheckUDPService] = udpServiceGuide
}

var udpServiceGuide = FixGuide{
	Steps: []string{
		"Find the device (its address is in the finding) and open its settings or its app.",
		"SNMP: switch it off if you do not use it, or change the community string from \"public\" to a long secret and allow only your own computer. TFTP: switch it off; almost nothing at home needs it. UPnP: switch it off on the device, or in the router if the router offers it.",
		"Run the UDP scan on the device again from its row on the Devices tab. The finding goes when the scan no longer gets an answer.",
		"If you turned the service on yourself, accept the finding with a note saying why.",
	},
	Commands: []GuideCommand{
		{platformLinux, "sudo nmap -Pn -sU -p 69,161,1900 <the device's address>"},
	},
	Risk: "Switching a service off can stop a monitoring tool, a firmware update or a smart-home app that used it from reaching the device.",
	Undo: "Switch the service on again in the device's settings.",
}
