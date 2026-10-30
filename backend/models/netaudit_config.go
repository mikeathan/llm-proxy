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
	// DeviceTimeoutMinutes is how long a scan may spend on one device before it gives up on it. 0 is the standard
	// (1 minute for a quick scan, 3 for a thorough one); the other allowed values are netaudit.AllowedDeviceTimeouts.
	DeviceTimeoutMinutes int `yaml:"device_timeout_minutes,omitempty" json:"device_timeout_minutes"`
	// PresenceCheck turns on the light presence check: every few minutes the host sweeps the declared scan ranges and reads
	// its neighbour table, so a device that was not on the network before is noticed in minutes. Off until the operator
	// turns it on.
	PresenceCheck bool `yaml:"presence_check,omitempty" json:"presence_check"`
	// PresenceIntervalMinutes is how often it runs; 0 is the default (netaudit.DefaultPresenceInterval). The allowed values
	// are netaudit.AllowedPresenceIntervals.
	PresenceIntervalMinutes int `yaml:"presence_interval_minutes,omitempty" json:"presence_interval_minutes"`
}
