package nmap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"llm-proxy/models"
)

// Bounds on what is kept of a privileged-helper probe (Constitution I.11): the helper's output is bytes from a program that talked
// to a device, so it is parsed here, in the unprivileged service, and only these typed, cleaned and bounded fields leave this file.
const (
	maxProbePorts = 16
	maxOSGuesses  = 5
	maxAccuracy   = 100
	protoUDP      = "udp"
	stateOpenUDP  = "open|filtered"
)

// The probe types live in the leaf models package because the device inventory keeps the last result; this package reads nmap into them.
type (
	// UDPPort is one UDP port that answered or could not be ruled out. "open|filtered" is what nmap says when a UDP port gave no reply:
	// it is kept as such and never shown as open.
	UDPPort = models.ProbeUDPPort
	// OSGuess is one of nmap's operating-system guesses with the accuracy nmap reported (0 to 100).
	OSGuess = models.ProbeOSGuess
	// Probe is what a UDP scan or an OS detection of one device found. A field is empty when its action was not the one that ran.
	Probe = models.DeviceProbe
)

type probeRun struct {
	Hosts []probeHost `xml:"host"`
}

type probeHost struct {
	Ports     []xmlPort `xml:"ports>port"`
	OSMatches []struct {
		Name     string `xml:"name,attr"`
		Accuracy int    `xml:"accuracy,attr"`
	} `xml:"os>osmatch"`
}

// ParseProbe reads the -oX output of the helper's scan_udp or os_detect for one target. Output that is not nmap XML is an error, not a
// guess, and a scan that found nothing is an empty probe, not an error.
func ParseProbe(data []byte) (Probe, error) {
	var run probeRun
	if err := xml.NewDecoder(io.LimitReader(bytes.NewReader(data), maxXMLBytes)).Decode(&run); err != nil {
		return Probe{}, fmt.Errorf("failed to read nmap output: %w", err)
	}
	probe := Probe{UDPPorts: []UDPPort{}, OS: []OSGuess{}}
	for _, h := range run.Hosts {
		for _, p := range h.Ports {
			if len(probe.UDPPorts) == maxProbePorts {
				break
			}
			if p.Protocol != protoUDP || p.ID < 1 || p.ID > 65535 || (p.State.State != stateOpen && p.State.State != stateOpenUDP) {
				continue
			}
			probe.UDPPorts = append(probe.UDPPorts, UDPPort{Port: p.ID, State: p.State.State, Service: clean(p.Service.Name)})
		}
		for _, m := range h.OSMatches {
			if len(probe.OS) == maxOSGuesses {
				break
			}
			probe.OS = append(probe.OS, OSGuess{Name: clean(m.Name), Accuracy: min(max(m.Accuracy, 0), maxAccuracy)})
		}
	}
	return probe, nil
}
