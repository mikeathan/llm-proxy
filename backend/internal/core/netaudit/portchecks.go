package netaudit

import (
	"fmt"
	"slices"
	"strings"
)

// Port checks are findings that need no script: the port being open is the problem. They read only the port list every
// engine produces, so they work in any scan mode and in the limited Go engine too.
//
// A service name nmap took from its port table is only a guess (see Port.Guessed), so a rule fires on the port number
// alone and says so in its words; when the scan confirmed the service, the rule says that and ranks it higher.

// exposure is a service that is a problem simply by listening on the network. It fires on the port number alone, in
// words that say it is only the usual use of that port, and fires harder when the scan confirmed the service.
type exposure struct {
	check string
	// ports are the numbers that usually carry the service.
	ports []int
	// confirmed tells whether the scan itself identified the service. It reads the service and product names to decide
	// yes or no; neither is copied into a finding. It is nil for a service nmap has no way to recognise (adb, etcd and
	// the Kubelet: nmap 7.99's service-probes file has no match for them), where only the port can be judged.
	confirmed func(Port) bool
	// sureSeverity and sure are for a confirmed service; portSeverity and port (with the number in %d) are for the
	// port alone.
	sureSeverity, sure string
	portSeverity, port string
}

// serviceIs reports whether the scan identified the service under one of these names. Names are compared ignoring case,
// because nmap spells some of them in capitals ("X11", seen in a real capture).
func serviceIs(names ...string) func(Port) bool {
	return func(p Port) bool {
		return !p.Guessed && slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, p.Service) })
	}
}

// productHas reports whether the scan identified a product whose name contains the text, ignoring case.
func productHas(text string) func(Port) bool {
	return func(p Port) bool {
		return !p.Guessed && strings.Contains(strings.ToLower(p.Product), text)
	}
}

// judge applies the exposure to one open port.
func (e exposure) judge(p Port) (severity, evidence string, ok bool) {
	switch {
	case e.confirmed != nil && e.confirmed(p):
		return e.sureSeverity, e.sure, true
	case slices.Contains(e.ports, p.Number):
		return e.portSeverity, fmt.Sprintf(e.port, p.Number), true
	}
	return "", "", false
}

var exposures = []exposure{
	{
		check: CheckTelnetOpen, ports: []int{23, 2323}, confirmed: serviceIs("telnet"),
		sureSeverity: SeverityHigh, sure: "Telnet is open: a remote login that sends passwords and everything else unencrypted",
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally Telnet: a remote login that sends passwords unencrypted",
	},
	{
		check: CheckDockerAPI, ports: []int{2375}, confirmed: serviceIs("docker"),
		sureSeverity: SeverityCritical, sure: "Docker's remote control port is open with no password; anyone on the network can run programs on this machine",
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally Docker's remote control port with no password",
	},
	{
		check: CheckADBExposed, ports: []int{5555},
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally Android's debug bridge: anyone on the network could run commands on the device",
	},
	{
		check: CheckOpenProxy, ports: []int{1080, 3128},
		confirmed:    serviceIs("socks", "socks4", "socks5", "http-proxy", "squid-http"),
		sureSeverity: SeverityHigh, sure: "A proxy is open to the network; anyone on it can send their traffic through this device, which hides where it really comes from",
		portSeverity: SeverityMedium, port: "Port %d is open, which is normally a proxy: anyone on the network could send their traffic through this device",
	},
	{
		check: CheckElasticsearchExposed, ports: []int{9200}, confirmed: productHas("elasticsearch"),
		sureSeverity: SeverityHigh, sure: "Elasticsearch is reachable from the network; older versions have no login, so its data may be readable and changeable by anyone",
		portSeverity: SeverityMedium, port: "Port %d is open, which is normally Elasticsearch; older versions have no login, so its data may be open to anyone",
	},
	{
		check: CheckX11Exposed, ports: []int{6000}, confirmed: serviceIs("x11"),
		sureSeverity: SeverityCritical, sure: "A graphical display server is open to the network; anyone on it may be able to see the screen and read the keyboard",
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally a graphical display server (X11): anyone on the network may be able to see the screen",
	},
	{
		check: CheckRsyncExposed, ports: []int{873}, confirmed: serviceIs("rsync"),
		sureSeverity: SeverityHigh, sure: "The file-copy service rsync is open to the network; its shared folders may be readable without a password",
		portSeverity: SeverityMedium, port: "Port %d is open, which is normally the file-copy service rsync; its shared folders may be readable without a password",
	},
	{
		check: CheckKubeletExposed, ports: []int{10250},
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally a Kubernetes node's control port; without a login anyone could run commands in the containers",
	},
	{
		check: CheckEtcdExposed, ports: []int{2379},
		portSeverity: SeverityHigh, port: "Port %d is open, which is normally etcd, a cluster's key store holding its secrets",
	},
}

// portRule judges one open port. It returns the finding's words and severity, or false when the rule does not apply.
type portRule struct {
	check string
	judge func(p Port) (severity, evidence string, ok bool)
}

var portRules = func() []portRule {
	rules := make([]portRule, len(exposures))
	for i, e := range exposures {
		rules[i] = portRule{e.check, e.judge}
	}
	return rules
}()

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
