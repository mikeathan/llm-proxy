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
	// DecoysEnabled turns on the decoy services: fake services on a few high ports that nothing legitimate uses, so a
	// connection means a device is probing the network. It opens listening sockets, so it is off until the operator turns it
	// on (a step-up action), and switching it off closes them.
	DecoysEnabled bool `yaml:"decoys_enabled,omitempty" json:"decoys_enabled"`
	// RouterLogEnabled opens a UDP port that takes the router's remote syslog, only to wake the presence check early when the
	// router hands an address to, or sees join the Wi-Fi, a device the list does not know. It opens a listening socket, so it
	// is off until the operator turns it on (a step-up action) and switching it off closes it (Constitution I.8).
	RouterLogEnabled bool `yaml:"router_log_enabled,omitempty" json:"router_log_enabled"`
	// IPv6Discovery turns on active IPv6 discovery: on each presence check and audit the host sends one echo request to the
	// link-local all-nodes group on the links that hold in-scope devices, so the IPv6 neighbour table lists the whole link.
	// Off until the operator turns it on (Constitution I.7).
	IPv6Discovery bool `yaml:"ipv6_discovery,omitempty" json:"ipv6_discovery"`
	// WeeklySummary sends a short summary (counts only) of the week to every connector, once a week. It rides on the presence
	// check, so it is sent only while that is on. Off until the operator turns it on.
	WeeklySummary bool `yaml:"weekly_summary,omitempty" json:"weekly_summary"`
	// DeviceNamesOff turns off asking unnamed devices what they call themselves (one small question to each, rarely, through
	// the guarded dialer). It is a negative flag so that the default, with nothing stored, is on.
	DeviceNamesOff bool `yaml:"device_names_off,omitempty" json:"device_names_off"`
	// OutsideLookupEnabled lets the nightly audit send the network's public address (as the router states it) to an outside scan
	// database and list what it says the internet can reach. The service learns the address, so it is off until the operator
	// turns it on (a step-up action).
	OutsideLookupEnabled bool `yaml:"outside_lookup_enabled,omitempty" json:"outside_lookup_enabled"`
	// OutsideLookupURL is the service's address (https only). Empty means netaudit's default service.
	OutsideLookupURL string `yaml:"outside_lookup_url,omitempty" json:"outside_lookup_url"`
	// DNSCheckEnabled has the host compare, every DNSCheckIntervalMinutes, what the router's DNS and an outside resolver answer
	// for a few names, and log the result (it raises no finding). The outside resolver learns the names and the network's public
	// address, so it is off until the operator turns it on (a step-up action).
	DNSCheckEnabled bool `yaml:"dns_check_enabled,omitempty" json:"dns_check_enabled"`
	// DNSCheckIntervalMinutes is how often it runs; 0 is the default (netaudit.DefaultDNSCheckInterval).
	DNSCheckIntervalMinutes int `yaml:"dns_check_interval_minutes,omitempty" json:"dns_check_interval_minutes"`
	// DNSCheckNames are the names compared. Empty means netaudit's defaults.
	DNSCheckNames []string `yaml:"dns_check_names,omitempty" json:"dns_check_names"`
	// DNSCheckServer is the router's DNS server (a private IPv4 address). Empty means the default gateway.
	DNSCheckServer string `yaml:"dns_check_server,omitempty" json:"dns_check_server"`
	// DNSCheckResolverURL is the outside resolver's DNS-over-HTTPS address (https only). Empty means netaudit's default.
	DNSCheckResolverURL string `yaml:"dns_check_resolver_url,omitempty" json:"dns_check_resolver_url"`
}
