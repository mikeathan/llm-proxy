package netaudit

import (
	"context"
	"net/netip"

	"llm-proxy/internal/core/netaudit/udpchecks"
	"llm-proxy/models"
)

// The nightly UDP probes (Constitution item 12, plan network-nightly-udp-probes) ask each scanned device two fixed questions, an SNMPv3
// engine discovery and a unicast SSDP search, and report an answer as the same udp_service finding a helper scan makes (probefindings.go),
// with the same fixed words. What a device said is never kept: only that it answered in the expected form. A silent device is unknown,
// never fixed, so an earlier finding for it is kept (UDPResult.Carry).

// UDPAsker asks devices the fixed questions: *udpchecks.Prober.
type UDPAsker interface {
	Look(ctx context.Context, addrs []netip.Addr) []udpchecks.Answer
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

// Findings asks every scanned, live IPv4 host and returns the answers as findings. The guarded dial the asker uses refuses any address
// outside the declared scan scope, so none is asked then.
func (c *UDPChecks) Findings(ctx context.Context, hosts []Host) UDPResult {
	var addrs []netip.Addr
	for _, h := range hosts {
		if h.Up && !h.Unscanned && h.Addr.Is4() {
			addrs = append(addrs, h.Addr)
		}
	}
	res := UDPResult{unknown: map[string]bool{}}
	if len(addrs) == 0 {
		return res
	}
	answered := map[string]bool{}
	for _, a := range c.Asker.Look(ctx, addrs) {
		rule, judged := udpRules[a.Port]
		if !judged {
			continue
		}
		f := Finding{Device: a.Addr.String(), Port: a.Port, Proto: protoUDPFinding, Service: rule.Name, Check: CheckUDPService, Severity: rule.Severity, Evidence: nightlyEvidence(rule)}
		answered[f.Key()] = true
		res.Findings = append(res.Findings, f)
	}
	asked := addrs
	if len(asked) > udpchecks.MaxDevices {
		asked = asked[:udpchecks.MaxDevices]
	}
	for _, a := range asked {
		for _, p := range udpchecks.Probes {
			k := Finding{Device: a.String(), Port: p.Port, Proto: protoUDPFinding, Check: CheckUDPService}.Key()
			if !answered[k] {
				res.unknown[k] = true
			}
		}
	}
	return res
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
