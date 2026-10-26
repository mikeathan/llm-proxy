package models

import "context"

// AuditorProfileID is the built-in prompt profile a network audit runs under.
const AuditorProfileID = "security-auditor"

// Scan depths: quick checks the common ports in seconds; thorough looks harder and takes minutes.
const (
	ScanDepthQuick    = "quick"
	ScanDepthThorough = "thorough"
)

// ScanSettings are a network audit's own settings: what it scans and how. They belong to the automation and are
// applied by code when the audit runs; the model never chooses them, whatever it passes to the scan tool. The zero
// value is the usual audit: every allowed network, thorough, the common ports, checks on.
type ScanSettings struct {
	// Targets are the addresses, ranges or names to scan; empty means every network the operator has allowed.
	Targets []string `yaml:"targets,omitempty" json:"targets,omitempty"`
	// Depth is ScanDepthQuick or ScanDepthThorough; empty is thorough.
	Depth string `yaml:"depth,omitempty" json:"depth,omitempty"`
	// Ports is a port list such as "22,80,8000-8100"; empty means the common ports.
	Ports string `yaml:"ports,omitempty" json:"ports,omitempty"`
	// Checks says whether the safe checks run; nil is on. A pointer so an explicit "off" survives a save.
	Checks *bool `yaml:"checks,omitempty" json:"checks,omitempty"`
}

// Thorough reports whether the scan looks harder (anything but an explicit quick).
func (s ScanSettings) Thorough() bool { return s.Depth != ScanDepthQuick }

// ChecksOn reports whether the safe checks run (anything but an explicit off).
func (s ScanSettings) ChecksOn() bool { return s.Checks == nil || *s.Checks }

// Normalized returns the settings with the depth and checks filled in, so a screen shows what will run without
// knowing the defaults.
func (s ScanSettings) Normalized() ScanSettings {
	depth := ScanDepthThorough
	if !s.Thorough() {
		depth = ScanDepthQuick
	}
	s.Depth = depth
	on := s.ChecksOn()
	s.Checks = &on
	return s
}

// EffectiveScan is the scan settings this automation's runs use: its own when it has them, the defaults for a network
// audit that has none (so even an old audit never leaves the choice to the model), and nil for any other automation.
func (a *Automation) EffectiveScan() *ScanSettings {
	switch {
	case a.Scan != nil:
		n := a.Scan.Normalized()
		return &n
	case a.Profile == AuditorProfileID:
		n := ScanSettings{}.Normalized()
		return &n
	}
	return nil
}

type scanSettingsKey struct{}

// WithScanSettings puts an audit's settings in the run's context, where the scan tool reads them.
func WithScanSettings(ctx context.Context, s ScanSettings) context.Context {
	return context.WithValue(ctx, scanSettingsKey{}, s)
}

// ScanSettingsFrom returns the settings of the run, and whether it has any.
func ScanSettingsFrom(ctx context.Context) (ScanSettings, bool) {
	s, ok := ctx.Value(scanSettingsKey{}).(ScanSettings)
	return s, ok
}
