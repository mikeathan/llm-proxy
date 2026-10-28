package netaudit

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/core/netaudit/vulnfeed"
	"time"

	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/models"
)

// Settings section the readiness fixes point at.
const (
	scanSettingsSection = "network-scan"
	// ActionAllowUnconfined is the confirm action that lets scans run without OS file confinement.
	ActionAllowUnconfined = "allow_unconfined"
)

// ReadinessDeps are what the readiness checks read. Each is a small port so the checks can be tested without a host.
type ReadinessDeps struct {
	// Config returns the saved scan policy.
	Config func() models.NetauditConfig
	// Nmap reports why nmap cannot be used; nil means it can.
	Nmap interface {
		Available(ctx context.Context) error
	}
	// Sandbox reports what file confinement the host enforces.
	Sandbox interface{ Effective() sandbox.EffectiveState }
	// HostNetworkOn reports whether the host allows agent network traffic at all.
	HostNetworkOn func() bool
	// VulnData reports how much of the vulnerability data is downloaded; nil means notices about it are not made.
	VulnData func() VulnDataView
}

// OldVulnDataAge is how old held vulnerability data may get before a notice offers the update.
const OldVulnDataAge = vulnfeed.StaleAfter

// VulnDataView is what the notice about the vulnerability data needs: whether a download is running, how many of the
// wanted files are held, and how long ago the oldest held source was downloaded (zero when nothing is held).
type VulnDataView struct {
	Running               bool
	FilesDone, FilesTotal int
	Age                   time.Duration
}

// Readiness answers "can a scan run, and if not, what is missing?" — the one source for the tool's visibility, the
// settings banner and the nightly audit's pre-flight.
type Readiness struct {
	deps ReadinessDeps
}

func NewReadiness(deps ReadinessDeps) *Readiness { return &Readiness{deps: deps} }

// Blocking lists what makes the scan tool unusable. It reads settings only, so it is cheap enough to run on every
// turn: it never starts a process.
func (r *Readiness) Blocking() []models.Requirement {
	var out []models.Requirement
	if r.deps.HostNetworkOn != nil && !r.deps.HostNetworkOn() {
		out = append(out, models.Requirement{
			Code: models.RequirementHostNetworkOff, Blocking: true,
			Message: "Agent network access is switched off for this host, so nothing can be scanned.",
			Fix:     &models.RequirementFix{Kind: models.FixOpenSettings, Section: "security"},
		})
	}
	if len(r.deps.Config().ScanRanges) == 0 {
		out = append(out, models.Requirement{
			Code: models.RequirementScopeEmpty, Blocking: true,
			Message: "No network has been allowed for scanning. Choose the network or addresses that may be scanned.",
			Fix:     &models.RequirementFix{Kind: models.FixOpenSettings, Section: scanSettingsSection},
		})
	}
	return out
}

// Check lists every unmet requirement: the blocking ones, then the ones that only limit scans to the reduced engine.
func (r *Readiness) Check(ctx context.Context) []models.Requirement {
	out := r.Blocking()
	nmapErr := r.deps.Nmap.Available(ctx)
	if nmapErr != nil {
		out = append(out, nmapMissing(nmapErr))
	}
	if nmapErr == nil && r.confinementRefuses() {
		out = append(out, models.Requirement{
			Code:    models.RequirementNoSandbox,
			Message: "This host cannot confine programs to a private folder, so nmap will not run. Allowing it anyway is for development only.",
			Fix:     &models.RequirementFix{Kind: models.FixConfirm, Action: ActionAllowUnconfined},
		})
	}
	return out
}

// confinementRefuses is true when progexec would refuse to start nmap: no OS file confinement and no opt-in.
func (r *Readiness) confinementRefuses() bool {
	if r.deps.Sandbox == nil || r.deps.Sandbox.Effective().Filesystem.Mechanism != sandbox.EnforcementNone {
		return false
	}
	return !r.deps.Config().AllowUnconfined
}

func nmapMissing(err error) models.Requirement {
	req := models.Requirement{
		Code:    models.RequirementNmapMissing,
		Message: "nmap is not available: " + err.Error() + ". Without it only a limited scan (open ports, no service detection) can run, and nightly audits will not run.",
	}
	var notInstalled *progexec.NotInstalledError
	if errors.As(err, &notInstalled) && notInstalled.Hint != "" {
		req.Fix = &models.RequirementFix{Kind: models.FixCopy, Text: notInstalled.Hint}
	}
	return req
}

// Notices lists what would improve an audit but never stops one: unlike Check, an audit runs without them, so the
// pre-flight does not read them. Today that is the vulnerability data, which an audit works without (the report then says
// the known-vulnerability check was skipped).
func (r *Readiness) Notices() []models.Requirement {
	if r.deps.VulnData == nil {
		return nil
	}
	v := r.deps.VulnData()
	fix := &models.RequirementFix{Kind: models.FixOpenSettings, Section: scanSettingsSection}
	notice := func(message string) []models.Requirement {
		return []models.Requirement{{Code: models.RequirementVulnDataMissing, Message: message, Fix: fix}}
	}
	switch {
	case v.Running:
		return notice(fmt.Sprintf("Vulnerability data is downloading (%d of %d files).", v.FilesDone, v.FilesTotal))
	case v.FilesDone >= v.FilesTotal && v.Age > OldVulnDataAge:
		days := int(v.Age / (24 * time.Hour))
		return []models.Requirement{{
			Code: models.RequirementVulnDataOld, Fix: fix,
			Message: fmt.Sprintf("The vulnerability data was downloaded %d days ago, so newer flaws are missing from audits. Update it in the scan settings.", days),
		}}
	case v.FilesDone >= v.FilesTotal:
		return nil
	case v.FilesDone == 0:
		return notice("Vulnerability data is not downloaded, so audits skip the check for known vulnerabilities. Download it in the scan settings.")
	}
	return notice(fmt.Sprintf("The vulnerability data download did not finish (%d of %d files). Audits use what was downloaded; resume it in the scan settings.", v.FilesDone, v.FilesTotal))
}
