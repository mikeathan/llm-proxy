package netaudit

import (
	"context"
	"net/netip"

	"llm-proxy/models"
)

// The nightly UDP probes (Constitution item 12, plan network-nightly-udp-probes) ask each scanned device two fixed questions, an SNMPv3
// engine discovery and a unicast SSDP search, and report an answer as the same udp_service finding a helper scan makes (probefindings.go),
// with the same fixed words. What a device said is never kept: only that it answered in the expected form. A silent device is unknown,
// never fixed, so an earlier finding for it is kept (UDPResult.Carry).
//
// The questions are asked by the Network stack's sensor (ADR-0038). It reports typed pairs and nothing a device said; the words, the severity,
// the key, the carry rule and the replacement of a helper-kept finding stay here, so a finding is the same whichever way it was asked.

// UDPPair is a device address and a UDP port.
type UDPPair struct {
	Addr netip.Addr
	Port int
}

// UDPLooked is what the sensor reports: every pair it asked and the pairs that answered in the expected form.
type UDPLooked struct {
	Asked    []UDPPair
	Answered []UDPPair
}

// UDPAsker asks devices the fixed questions: the Network stack's sensor through the stack host. An error means it could not look at all.
type UDPAsker interface {
	Look(ctx context.Context, addrs []netip.Addr) (UDPLooked, error)
}

// UDPFinder is what the run reporter calls: *UDPChecks.
type UDPFinder interface {
	Findings(ctx context.Context, hosts []Host) UDPResult
}

// UDPChecks turns the answers of the devices a scan found into findings. It holds no state between runs.
type UDPChecks struct{ Asker UDPAsker }

// UDPResult is what one run found, and the device and port pairs it asked and got no answer from, which Carry keeps.
type UDPResult struct {
	Findings []Finding
	unknown  map[string]bool
	have     map[string]bool
}

// nightlyEvidence is the finding's words. It names no date, so a finding that stays does not read as changed every night.
func nightlyEvidence(rule udpRule) string {
	return rule.Name + " answered a check from this computer. " + rule.Why
}

// udpKey is the key of the finding for one device and port, as Finding.Key makes it.
func udpKey(addr netip.Addr, port int) string {
	return Finding{Device: addr.String(), Port: port, Proto: protoUDPFinding, Check: CheckUDPService}.Key()
}

// scannedIPv4 is the addresses of the hosts the scan found live and finished, which are the only ones worth asking.
func scannedIPv4(hosts []Host) []netip.Addr {
	var addrs []netip.Addr
	for _, h := range hosts {
		if h.Up && !h.Unscanned && h.Addr.Is4() {
			addrs = append(addrs, h.Addr)
		}
	}
	return addrs
}

// Findings asks every scanned, live IPv4 host and returns the answers as findings. The sensor's dial refuses any address outside the declared
// scan scope, so none is asked then. When the sensor cannot look at all, nothing is found and every judged port of every such host is
// unknown, so no earlier finding is resolved by a night nobody looked.
func (c *UDPChecks) Findings(ctx context.Context, hosts []Host) UDPResult {
	addrs := scannedIPv4(hosts)
	res := UDPResult{unknown: map[string]bool{}}
	if len(addrs) == 0 {
		return res
	}
	looked, err := c.Asker.Look(ctx, addrs)
	if err != nil {
		res.unknownEverywhere(addrs)
		return res
	}
	answered := res.judge(looked.Answered)
	for _, p := range looked.Asked {
		if k := udpKey(p.Addr, p.Port); !answered[k] {
			res.unknown[k] = true
		}
	}
	return res
}

// unknownEverywhere marks every judged port of every address unknown: the sensor said nothing, so nothing is fixed.
func (r *UDPResult) unknownEverywhere(addrs []netip.Addr) {
	for _, a := range addrs {
		for port := range udpRules {
			r.unknown[udpKey(a, port)] = true
		}
	}
}

// judge turns the pairs that answered on a judged port into the rule's finding and returns the keys it made.
func (r *UDPResult) judge(answers []UDPPair) map[string]bool {
	made := map[string]bool{}
	for _, a := range answers {
		rule, judged := udpRules[a.Port]
		if !judged {
			continue
		}
		f := Finding{Device: a.Addr.String(), Port: a.Port, Proto: protoUDPFinding, Service: rule.Name, Check: CheckUDPService, Severity: rule.Severity, Evidence: nightlyEvidence(rule)}
		made[f.Key()] = true
		r.Findings = append(r.Findings, f)
	}
	return made
}

// seeing records the findings already in the report, so Carry does not add a second copy of one.
func (r UDPResult) seeing(present []Finding) UDPResult {
	r.have = map[string]bool{}
	for _, f := range present {
		if f.Check == CheckUDPService {
			r.have[f.Key()] = true
		}
	}
	return r
}

// Carry returns the earlier findings of a port that was asked tonight and did not answer: silence is not a fix. A finding the report
// already holds (a helper result that is still kept) is not returned again.
func (r UDPResult) Carry(earlier []Finding) []Finding {
	var out []Finding
	for _, f := range earlier {
		if f.Check == CheckUDPService && r.unknown[f.Key()] && !r.have[f.Key()] {
			out = append(out, f)
		}
	}
	return out
}

// UDPChecksConfig reads and changes whether the nightly audit asks the UDP questions. It is off until the operator turns it on.
type UDPChecksConfig struct{ store PolicyStore }

func NewUDPChecksConfig(store PolicyStore) *UDPChecksConfig { return &UDPChecksConfig{store: store} }

// Enabled reports whether the nightly UDP probes are on.
func (c *UDPChecksConfig) Enabled() bool { return c.store.Get().UDPProbesEnabled }

// Set switches the nightly UDP probes on or off.
func (c *UDPChecksConfig) Set(enabled bool) error {
	return c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.UDPProbesEnabled = enabled
		return nil
	})
}

// WithUDPChecks makes the reporter ask scanned devices the two UDP questions while enabled returns true. Without it nothing is asked.
func (r *RunReporter) WithUDPChecks(enabled func() bool, f UDPFinder) *RunReporter {
	r.udpOn, r.udp = enabled, f
	return r
}

// addUDPChecks asks the scanned hosts and adds what answered. A nightly answer replaces a helper-kept finding for the same device and
// port: it is newer. It returns this run's result for Carry.
func (r *RunReporter) addUDPChecks(ctx context.Context, result *Result, hosts []Host) UDPResult {
	if r.udp == nil || r.udpOn == nil || !r.udpOn() {
		return UDPResult{}
	}
	res := r.udp.Findings(ctx, hosts)
	fresh := map[string]bool{}
	for _, f := range res.Findings {
		fresh[f.Key()] = true
	}
	kept := result.Findings[:0:0]
	for _, f := range result.Findings {
		if !(f.Check == CheckUDPService && fresh[f.Key()]) {
			kept = append(kept, f)
		}
	}
	result.Findings = append(kept, res.Findings...)
	return res.seeing(result.Findings)
}
