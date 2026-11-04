package netaudit

import (
	"context"
	"fmt"

	"llm-proxy/models"
)

// NearbyFinder reads the kept Nearby state and says what is wrong in it: NearbyFindings.
type NearbyFinder interface {
	Findings() ([]Finding, error)
}

// ServiceFinder probes the open ports of a scan that a service check covers: *ServiceChecks. The result's Carry keeps the
// earlier finding of a port that run could not judge.
type ServiceFinder interface {
	Findings(ctx context.Context, hosts []Host) ServiceResult
}

// ClassFinder says which devices look like a kind that has no use for an administration service they have open:
// ClassChecks.
type ClassFinder interface {
	Findings(devices []models.NetworkDevice, hosts []Host) ([]Finding, error)
}

// ClassChecks judges devices by their maker, their open ports and what Nearby heard them announce. State reads the kept Nearby
// state; when it cannot, the judgement still runs on the maker and the ports, and the error is returned for the report to state.
type ClassChecks struct {
	State func() (models.NearbyState, error)
}

// Findings returns the class_mismatch findings and, separately, trouble reading what devices announced.
func (c ClassChecks) Findings(devices []models.NetworkDevice, hosts []Host) ([]Finding, error) {
	var state models.NearbyState
	var err error
	if c.State != nil {
		if state, err = c.State(); err != nil {
			state = models.NearbyState{}
		}
	}
	return ClassMismatchFindings(devices, hosts, state), err
}

// WithClassChecks makes the reporter judge what kind of device each scanned device looks like. Without it nothing is judged.
func (r *RunReporter) WithClassChecks(c ClassFinder) *RunReporter {
	r.classes = c
	return r
}

// addClasses appends the class findings for the devices this scan looked at. Trouble reading Nearby is stated; the findings
// that the maker and the ports alone support are still added.
func (r *RunReporter) addClasses(result *Result, devices []models.NetworkDevice) string {
	if r.classes == nil {
		return ""
	}
	findings, err := r.classes.Findings(devices, result.Hosts)
	result.Findings = append(result.Findings, restrictTo(findings, scannedDevices(result.Hosts))...)
	if err != nil {
		return fmt.Sprintf("\n\n> Note: what devices announced could not be read, so a kind that only that would show was not judged (%v).", err)
	}
	return ""
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

// addServices appends the findings of the service checks and returns this run's result, whose Carry keeps what it could not judge.
func (r *RunReporter) addServices(ctx context.Context, result *Result) ServiceResult {
	if r.service == nil {
		return ServiceResult{}
	}
	services := r.service.Findings(ctx, result.Hosts)
	result.Findings = append(result.Findings, services.Findings...)
	return services
}

// carriedByChecks is what the router check and this run's service checks keep from the last scan when they could not judge.
func (r *RunReporter) carriedByChecks(earlier []Finding, services ServiceResult) []Finding {
	return append(r.carriedByRouter(earlier), services.Carry(earlier)...)
}
