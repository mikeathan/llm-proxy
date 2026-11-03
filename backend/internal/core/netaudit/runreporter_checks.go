package netaudit

import (
	"context"
	"fmt"
)

// NearbyFinder reads the kept Nearby state and says what is wrong in it: NearbyFindings.
type NearbyFinder interface {
	Findings() ([]Finding, error)
}

// ServiceFinder probes the open ports of a scan that a service check covers: *ServiceChecks. Carry keeps the earlier finding of
// a port it could not judge this time.
type ServiceFinder interface {
	Findings(ctx context.Context, hosts []Host) []Finding
	Carry(earlier []Finding) []Finding
}

// WithNearby makes the reporter add what the Nearby state says is wrong. Without it nothing is read.
func (r *RunReporter) WithNearby(n NearbyFinder) *RunReporter {
	r.nearby = n
	return r
}

// WithServiceChecks makes the reporter probe open ports that a service check covers. Without it nothing is probed.
func (r *RunReporter) WithServiceChecks(s ServiceFinder) *RunReporter {
	r.service = s
	return r
}

// addNearby appends the findings for devices this scan looked at. They come from the kept state, not from a look, so a quiet
// look never resolves one. Trouble is stated and never hides the scan.
func (r *RunReporter) addNearby(result *Result) string {
	if r.nearby == nil {
		return ""
	}
	findings, err := r.nearby.Findings()
	if err != nil {
		return fmt.Sprintf("\n\n> Note: what devices said about themselves could not be read (%v).", err)
	}
	result.Findings = append(result.Findings, restrictTo(findings, scannedDevices(result.Hosts))...)
	return ""
}

// addServices appends the findings of the service checks.
func (r *RunReporter) addServices(ctx context.Context, result *Result) string {
	if r.service == nil {
		return ""
	}
	result.Findings = append(result.Findings, r.service.Findings(ctx, result.Hosts)...)
	return ""
}

// carriedByChecks is what the router check and the service checks keep from the last scan when they could not judge this time.
func (r *RunReporter) carriedByChecks(earlier []Finding) []Finding {
	out := r.carriedByRouter(earlier)
	if r.service != nil {
		out = append(out, r.service.Carry(earlier)...)
	}
	return out
}
