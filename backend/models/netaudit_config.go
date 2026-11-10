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
	// FollowUpScan scans a device that just appeared on the network once, at once, with an ordinary service scan of its own
	// address (no deep check, no model). Off until the operator turns it on: it adds scan traffic during the day.
	FollowUpScan bool `yaml:"follow_up_scan,omitempty" json:"follow_up_scan"`
	// DailyCheckIn sends one short note a day to every connector saying the program is running and which of its monitors have
	// stopped, so a computer that went down is noticed by the note that did not come. Off by default.
	DailyCheckIn bool `yaml:"daily_check_in,omitempty" json:"daily_check_in"`
	// Footprint is what the operator watches outside the local network (domains and email addresses) and which outside services
	// may be asked about them. Personal data: removed by a factory reset and never put in a model prompt.
	Footprint FootprintConfig `yaml:"footprint,omitempty" json:"footprint"`
	// OutsideLookupEnabled lets the nightly audit send the network's public address (as the router states it) to an outside scan
	// database and list what it says the internet can reach. The service learns the address, so it is off until the operator
	// turns it on (a step-up action).
	OutsideLookupEnabled bool `yaml:"outside_lookup_enabled,omitempty" json:"outside_lookup_enabled"`
	// OutsideLookupURL is the service's address (https only). Empty means netaudit's default service.
	OutsideLookupURL string `yaml:"outside_lookup_url,omitempty" json:"outside_lookup_url"`
	// OutsideLookupAddress is the network's public IPv4 address as the operator typed it, used only when the router states none
	// (UPnP off). Empty means none entered.
	OutsideLookupAddress string `yaml:"outside_lookup_address,omitempty" json:"outside_lookup_address"`
	// DNSCheckEnabled has the host compare, every DNSCheckIntervalMinutes, what the router's DNS and an outside resolver answer
	// for a few names, and log the result (a finding is a separate switch, DNSCheckFindingEnabled). The outside resolver learns the names and the network's public
	// address, so it is off until the operator turns it on (a step-up action).
	DNSCheckEnabled bool `yaml:"dns_check_enabled,omitempty" json:"dns_check_enabled"`
	// DNSCheckFindingEnabled lets the nightly report carry a finding when the router and an outside resolver disagree on a name on several
	// checks in a row (ADR-0036). Off until the operator has seen the check catch a resolver that lies.
	DNSCheckFindingEnabled bool `yaml:"dns_check_finding_enabled,omitempty" json:"dns_check_finding_enabled"`
	// DNSCheckIntervalMinutes is how often it runs; 0 is the default (netaudit.DefaultDNSCheckInterval).
	DNSCheckIntervalMinutes int `yaml:"dns_check_interval_minutes,omitempty" json:"dns_check_interval_minutes"`
	// DNSCheckNames are the names compared. Empty means netaudit's defaults.
	DNSCheckNames []string `yaml:"dns_check_names,omitempty" json:"dns_check_names"`
	// DNSCheckServer is the router's DNS server (a private IPv4 address). Empty means the default gateway.
	DNSCheckServer string `yaml:"dns_check_server,omitempty" json:"dns_check_server"`
	// DNSCheckResolverURL is the outside resolver's DNS-over-HTTPS address (https only). Empty means netaudit's default.
	DNSCheckResolverURL string `yaml:"dns_check_resolver_url,omitempty" json:"dns_check_resolver_url"`
	// PhoneHomeEnabled turns on the phone-home watch: the DNS names each device asks the router for are remembered (by hardware
	// address) so a new place, a steady beat and a device that avoids the router's DNS can be reported. Off until the operator turns it
	// on; it needs the router log (Constitution I.8).
	PhoneHomeEnabled bool `yaml:"phone_home_enabled,omitempty" json:"phone_home_enabled"`
	// WebChecksEnabled has the nightly audit ask the web pages of the devices it scanned a fixed list of questions (core/netaudit/webprobe).
	// Off until the operator turns it on; the operator's deep check of one device asks them whenever it runs.
	WebChecksEnabled bool `yaml:"web_checks_enabled,omitempty" json:"web_checks_enabled"`
	// UDPProbesEnabled has the nightly audit ask each device it scanned two fixed UDP questions, an SNMPv3 engine discovery and a unicast
	// SSDP search (core/netaudit/udpchecks, Constitution item 12). No community string is sent. It is off until the operator turns it on.
	UDPProbesEnabled bool `yaml:"udp_probes_enabled,omitempty" json:"udp_probes_enabled"`
	// LoginWatchEnabled has the program count the failed logins to this computer from the system's own record and tell of a burst
	// (core/netaudit loginwatch, Constitution I.10 proposed). Off until the operator turns it on; the service needs the utmp group.
	LoginWatchEnabled bool `yaml:"login_watch_enabled,omitempty" json:"login_watch_enabled"`
	// WifiLookalikesEnabled has the program look, every few minutes, at the Wi-Fi access points this computer's NetworkManager already
	// knows (nmcli, as a guarded program that cannot use the network: ADR-0031) and tell of one that advertises the operator's own network
	// name from a hardware address the operator has not confirmed. Off until the operator turns it on.
	WifiLookalikesEnabled bool `yaml:"wifi_lookalikes_enabled,omitempty" json:"wifi_lookalikes_enabled"`
	// WifiNetworks are the operator's own Wi-Fi network names and the hardware addresses of their access points, confirmed by the
	// operator from what a look saw. A name with no confirmed address is not judged yet.
	WifiNetworks []WifiNetwork `yaml:"wifi_networks,omitempty" json:"wifi_networks"`
	// KnownBadEnabled has the phone-home watch compare the names devices ask for with abuse.ch's list of hosts that serve malware
	// (matched on this computer). Off until the operator turns it on; it needs the watch and a downloaded list.
	KnownBadEnabled bool `yaml:"known_bad_enabled,omitempty" json:"known_bad_enabled"`
	// AttackPathsOff stops the attack paths (the Overview card and the report section). They only read what the audit already holds,
	// so they are on unless switched off.
	AttackPathsOff bool `yaml:"attack_paths_off,omitempty" json:"attack_paths_off"`
	// NearbyEnabled lets the host send two multicast questions (mDNS and SSDP) and read the answers, to learn what the devices on
	// the network say they are (Constitution I.9). It is off until the operator turns it on (a step-up action).
	NearbyEnabled bool `yaml:"nearby_enabled,omitempty" json:"nearby_enabled"`
}

// WifiNetwork is one of the operator's own Wi-Fi network names and the hardware addresses (lower case, colon separated) of the access
// points the operator confirmed are theirs: the router, a mesh node, each band.
type WifiNetwork struct {
	Name      string   `yaml:"name" json:"name"`
	Addresses []string `yaml:"addresses,omitempty" json:"addresses"`
}
