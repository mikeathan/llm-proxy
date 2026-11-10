package netaudit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"llm-proxy/models"
)

// Why a finding cannot be rechecked here.
var (
	// ErrUseDeepCheck means the finding came from the deep check, which the operator runs from the device's own dialog.
	ErrUseDeepCheck = errors.New("this finding comes from the deep check: run the deep check on the device again")
	// ErrNotRecheckable means no scan of one device can say whether the finding is gone (it is built from router logs, history or
	// the whole network).
	ErrNotRecheckable = errors.New("this finding cannot be checked again on its own; the next audit will show whether it is gone")
)

// scanChecks are the checks a scan of one device and one port produces by itself (the port rules and the safe script checks), so
// a new scan of that port can say yes or no. Every other check comes from something a single-device scan does not look at, and
// is not guessed at.
var scanChecks = map[string]bool{
	CheckSMBv1: true, CheckSMBSigning: true, CheckFTPAnonymous: true, CheckVNCNoAuth: true, CheckRedisNoAuth: true, CheckNFSExports: true,
	CheckCertExpired: true, CheckCertExpiring: true, CheckSSHWeakAlgorithms: true, CheckRDPNoNLA: true, CheckSSHWeakHostKey: true,
	CheckTLSWeakKey: true, CheckTLSWeakSignature: true, CheckMemcachedOpen: true, CheckMongoNoAuth: true, CheckMQTTAnonymous: true,
	CheckTelnetOpen: true, CheckDockerAPI: true, CheckADBExposed: true, CheckOpenProxy: true, CheckElasticsearchExposed: true,
	CheckX11Exposed: true, CheckRsyncExposed: true, CheckKubeletExposed: true, CheckEtcdExposed: true,
}

// RecheckOutcome is what a recheck found.
type RecheckOutcome string

const (
	RecheckFixed      RecheckOutcome = "fixed"
	RecheckStillThere RecheckOutcome = "still_there"
	// RecheckCouldNot means the check was not made. It is never "fixed": the same rule as the exposure look (ADR-0019).
	RecheckCouldNot RecheckOutcome = "could_not_check"
)

// RecheckResult is the answer shown to the operator. Reason is fixed words, never text from the device.
type RecheckResult struct {
	Outcome   RecheckOutcome `json:"outcome"`
	Reason    string         `json:"reason"`
	CheckedAt time.Time      `json:"checked_at"`
}

// RecheckDeps are what the recheck needs; the application wires them in one place.
type RecheckDeps struct {
	// Scan runs one scan through the scan service (the declared scope, one scan at a time); ErrScanBusy means one is running.
	Scan      func(ctx context.Context, req Request) (Result, error)
	Baselines interface {
		ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	}
	Life LifeStore
	Now  func() time.Time
}

// Recheck re-runs only what produced a finding, on its own device and port, and says fixed, still there or could not check.
// A fixed answer is written to the finding's history (resolved by recheck); the baseline is not touched, so the next audit
// still compares against the last full scan and the finding is confirmed or comes back there.
type Recheck struct{ deps RecheckDeps }

func NewRecheck(deps RecheckDeps) *Recheck {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Recheck{deps: deps}
}

// Check rechecks one finding of the automation's latest scan.
func (r *Recheck) Check(ctx context.Context, workspaceID, automation, key string) (RecheckResult, error) {
	base, err := r.deps.Baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		return RecheckResult{}, fmt.Errorf("failed to read the latest scan: %w", err)
	}
	f, ok := findingByKey(base.Findings, key)
	if !ok {
		return RecheckResult{}, ErrNoSuchFinding
	}
	if err := recheckable(f); err != nil {
		return RecheckResult{}, err
	}
	res, err := r.deps.Scan(ctx, Request{Targets: []string{f.Device}, Mode: ModeChecks, Ports: strconv.Itoa(f.Port), Unattended: true})
	if errors.Is(err, ErrScanBusy) {
		return RecheckResult{}, err
	}
	at := r.deps.Now()
	if err != nil {
		return RecheckResult{Outcome: RecheckCouldNot, Reason: couldNotReason(err), CheckedAt: at}, nil
	}
	outcome, reason := judgeRecheck(f, res)
	if err := r.record(workspaceID, f, outcome, at); err != nil {
		return RecheckResult{}, err
	}
	return RecheckResult{Outcome: outcome, Reason: reason, CheckedAt: at}, nil
}

func findingByKey(findings []Finding, key string) (Finding, bool) {
	for _, f := range findings {
		if f.Key() == key {
			return f, true
		}
	}
	return Finding{}, false
}

// recheckable says whether a scan of the finding's own device and port can judge it.
func recheckable(f Finding) error {
	switch {
	case IsDeepCheck(f.Check):
		return ErrUseDeepCheck
	case f.Device == HostDevice || f.Proto != "tcp" || f.Port <= 0 || !scanChecks[f.Check]:
		return ErrNotRecheckable
	}
	return nil
}

// CanRecheck says whether a finding has a "check again": a screen offers the button only then.
func CanRecheck(f Finding) bool { return recheckable(f) == nil }

func couldNotReason(err error) string {
	if errors.Is(err, ErrNmapRequired) {
		return "The scanner program (nmap) is needed to check this again and is not available."
	}
	return "The check could not run."
}

// judgeRecheck reads the new scan of one device. The finding is gone only when the device answered, was looked at to the end
// and the same finding is not produced again; anything less is "could not check".
func judgeRecheck(f Finding, res Result) (RecheckOutcome, string) {
	var host *Host
	for i := range res.Hosts {
		if res.Hosts[i].Addr.String() == f.Device {
			host = &res.Hosts[i]
		}
	}
	switch {
	case host == nil || !host.Up:
		return RecheckCouldNot, "The device did not answer, so it could not be checked."
	case host.Unscanned:
		return RecheckCouldNot, "The scan ran out of time on the device, so it could not be checked."
	}
	for _, again := range FindingsOf([]Host{*host}) {
		if again.Key() == f.Key() {
			return RecheckStillThere, "The check finds it again."
		}
	}
	return RecheckFixed, "The device was checked again and the problem is gone."
}

// record writes the outcome to the finding's history. A check that was not made writes nothing.
func (r *Recheck) record(workspaceID string, f Finding, outcome RecheckOutcome, at time.Time) error {
	if r.deps.Life == nil || outcome == RecheckCouldNot {
		return nil
	}
	return r.deps.Life.UpdateLife(workspaceID, func(records []models.FindingLife) []models.FindingLife {
		if outcome == RecheckStillThere {
			return AdvanceLife(records, []Finding{f}, nil, at)
		}
		return CloseLife(records, f, models.LifeByRecheck, at)
	})
}
