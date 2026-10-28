package netaudit

// Port checks are findings that need no script: the port being open is the problem. They read only the port list every
// engine produces, so they work in any scan mode and in the limited Go engine too.
//
// A service name nmap took from its port table is only a guess (see Port.Guessed), so a rule fires on the port number
// alone and says so in its words; when the scan confirmed the service, the rule says that and ranks it higher.

const (
	telnetPort = 23
	dockerPort = 2375

	serviceTelnet = "telnet"
	serviceDocker = "docker"
)

// portRule judges one open port. It returns the finding's words and severity, or false when the rule does not apply.
type portRule struct {
	check string
	judge func(p Port) (severity, evidence string, ok bool)
}

// confirmed reports whether the scan itself identified the service, rather than naming it from the port number.
func confirmed(p Port, service string) bool { return !p.Guessed && p.Service == service }

var portRules = []portRule{
	{CheckTelnetOpen, func(p Port) (string, string, bool) {
		switch {
		case confirmed(p, serviceTelnet):
			return SeverityHigh, "Telnet is open: a remote login that sends passwords and everything else unencrypted", true
		case p.Number == telnetPort:
			return SeverityHigh, "Port 23 is open, which is normally Telnet: a remote login that sends passwords unencrypted", true
		}
		return "", "", false
	}},
	{CheckDockerAPI, func(p Port) (string, string, bool) {
		switch {
		case confirmed(p, serviceDocker):
			return SeverityCritical, "Docker's remote control port is open with no password; anyone on the network can run programs on this machine", true
		case p.Number == dockerPort:
			return SeverityHigh, "Port 2375 is open, which is normally Docker's remote control port with no password", true
		}
		return "", "", false
	}},
}

// portFindings applies the port rules to one open port.
func portFindings(device string, p Port) []Finding {
	var out []Finding
	for _, r := range portRules {
		if severity, evidence, ok := r.judge(p); ok {
			out = append(out, Finding{
				Device: device, Port: p.Number, Proto: p.Proto, Service: serviceLabel(p),
				Check: r.check, Severity: severity, Evidence: evidence,
			})
		}
	}
	return out
}
