package netaudit

import (
	"net/netip"
	"slices"
)

// CheckOwnModelServerOpen is the AI-server check when the server it found is this program's own model server, on this computer's
// address and a port this program starts it on. The generic guide tells the operator to find the program and change its flags;
// for our own server the fix is one setting of this program, and the /v1 key the operator may have made does not protect it.
const CheckOwnModelServerOpen = "own_model_server_open"

// ownModelServerWords is the finding's evidence. The words are fixed.
const ownModelServerWords = "This program's own model server listens on every network address, so anyone on the network can use it directly, without the /v1 inference key"

func init() {
	checkTitles[CheckOwnModelServerOpen] = "This program's model server is open to the network"
	remediations[CheckOwnModelServerOpen] = "Open Settings, System, and set the model server address to 127.0.0.1. The program reaches its model server on this computer, so nothing it does needs the server open to the network."
	guides[CheckOwnModelServerOpen] = FixGuide{
		Steps: []string{
			"Open Settings, System, and set the model server address to 127.0.0.1, then save.",
			"The next time a model starts it listens on this computer only. Stop the running model on the Models page if you want the change at once.",
			"If another computer used the model server directly, point it at this program's /v1 address with an inference key instead.",
			"Run the audit again to confirm.",
		},
		Risk: "A program on another computer that called the model server's port directly stops working until it uses /v1.",
		Undo: "Set the model server address back to what it was in Settings, System.",
	}
}

// probeCheckOf is the check of the probe that found a finding: the own-model-server finding is the AI-server probe's, relabelled
// after the probe ran, so a night that probe could not judge must keep it (ServiceResult.Carry).
func probeCheckOf(check string) string {
	if check == CheckOwnModelServerOpen {
		return CheckAIAPIExposed
	}
	return check
}

// OwnModelServer says which address and ports are this program's own model server. Both are read at the moment of the report.
type OwnModelServer struct {
	IsOwnAddr func(netip.Addr) bool
	Ports     func() []int
}

// Relabel turns the AI-server finding on this computer's own model port into the own-model-server finding, with its own words
// and guide. Every other finding is returned as it was. Without both sources nothing changes.
func (o OwnModelServer) Relabel(findings []Finding) []Finding {
	if o.IsOwnAddr == nil || o.Ports == nil {
		return findings
	}
	var ports []int
	for i, f := range findings {
		if f.Check != CheckAIAPIExposed {
			continue
		}
		addr, err := netip.ParseAddr(f.Device)
		if err != nil || !o.IsOwnAddr(addr) {
			continue
		}
		if ports == nil {
			ports = o.Ports()
		}
		if slices.Contains(ports, f.Port) {
			findings[i].Check, findings[i].Evidence = CheckOwnModelServerOpen, ownModelServerWords
		}
	}
	return findings
}
