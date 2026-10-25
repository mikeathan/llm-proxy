package models

// NetauditConfig is the host-wide network-audit policy (settings.yml → netaudit:). It is deliberately not part of
// UserSettings, so saving the ordinary settings form cannot overwrite it.
type NetauditConfig struct {
	// ScanRanges are the CIDR blocks the operator allows scans to touch, in canonical form. Empty means scanning is
	// off. The operator may type an address, a range or a name; the server stores the CIDR blocks they stand for.
	ScanRanges []string `yaml:"scan_ranges,omitempty" json:"scan_ranges"`
	// AllowUnconfined lets a scan program run on a host with no OS file confinement. Development only: without it
	// such a host refuses to run guarded programs.
	AllowUnconfined bool `yaml:"allow_unconfined,omitempty" json:"allow_unconfined"`
}
