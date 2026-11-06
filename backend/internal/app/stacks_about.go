package app

import (
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stack"
)

// What each module says about itself: what it watches, what it never does and where its results appear, one plain sentence each
// (stack.About), and the checks only it produces (stack.Module.Finds). The Modules tab, the Help and the "found by" line on a finding
// read these, so this is the only place the words are written. A module with no check of its own (it adds to others' findings, or
// shares a check with the audit's own scan) lists none.
var moduleAbout = map[string]stack.About{
	"presence": {
		Watches:  "Which devices are on the network, looked at every few minutes, and whether the router, or a device you marked important, now answers from a different hardware address.",
		Never:    "It sends nothing to the devices: it reads this computer's own list of neighbours.",
		ShowsUp:  "New devices and changed addresses on the Devices tab, in the nightly findings, as a message and on the bell.",
		Settings: true,
	},
	"ipv6_discovery": {
		Watches: "Devices that speak only IPv6, found with one echo on the local link.",
		Never:   "It stays on the local link and never reaches the internet.",
		ShowsUp: "On the Devices tab.",
	},
	"device_names": {
		Watches: "What each device calls itself, asked in the ordinary way.",
		Never:   "It sends only ordinary name questions to your own devices.",
		ShowsUp: "As names on the Devices tab.",
	},
	"wifi_lookalikes": {
		Watches:  "The Wi-Fi networks this computer's own NetworkManager can hear, for one that uses your network's name from an access point you have not confirmed.",
		Never:    "It never scans, connects or sends anything: it reads NetworkManager's list in a sandbox that cannot use the network, and keeps only your own network's name and addresses.",
		ShowsUp:  "As a message and a bell entry at once, and as a finding in the nightly report.",
		Settings: true,
	},
	"nearby": {
		Watches: "What devices announce about themselves: names, services and kinds.",
		Never:   "It sends two small fixed messages to the local network and reads the answers; it does not sign in to anything.",
		ShowsUp: "On the Devices tab and in the findings.",
	},
	"router_log": {
		Watches:  "The router's own log: refused attempts, new devices, failed sign-ins to the router and Wi-Fi disconnects.",
		Never:    "It keeps no line of the log, only counts and fixed facts, because any device on the network can forge a line.",
		ShowsUp:  "As counts on the Overview, in the findings, and as a message and a bell entry for a burst.",
		Settings: true,
	},
	"decoys": {
		Watches:  "Fake services that nothing real uses, so any touch is a probe.",
		Never:    "They offer nothing real: each answers with a short fixed banner and closes.",
		ShowsUp:  "As a message at once, on the Timeline and in the findings.",
		Settings: true,
	},
	"phone_home": {
		Watches: "Which names each device asks the router for: a new place, a steady beat, a device that avoids the router's DNS, and one that asks for a public encrypted-DNS server.",
		Never:   "It sends nothing; names stay on this computer and are never sent to a model.",
		ShowsUp: "In the findings, and as a message for a new place or a beat.",
	},
	"known_bad": {
		Watches:  "The names devices ask for, compared with abuse.ch's list of hosts that serve malware.",
		Never:    "No name a device asked for is sent anywhere; only the list is downloaded, when you press Download.",
		ShowsUp:  "As a high finding and a message at once.",
		Settings: true,
	},
	"login_watch": {
		Watches: "Failed logins to this computer, counted from the system's own record.",
		Never:   "It reads no user name and no address, only how many and when.",
		ShowsUp: "As a message for a burst, and a finding in the nightly report.",
	},
	"scan_on_arrival": {
		Watches: "A new device, scanned once when it arrives.",
		Never:   "It scans only inside the network you allowed, and only once for each device.",
		ShowsUp: "As a message with what the scan found.",
	},
	"web_checks": {
		Watches: "The web pages and certificates of the devices the audit finds: 24 fixed pages, and one handshake with each secure service.",
		Never:   "It sends plain page requests and handshakes only, and never signs in or sends data.",
		ShowsUp: "As findings in the nightly report and in a deep check of one device.",
	},
	"outside_lookup": {
		Watches:  "Which of your ports the internet can reach, asked of a public service.",
		Never:    "It sends only the router's public address.",
		ShowsUp:  "As the Internet view and in the findings.",
		Settings: true,
	},
	"dns_check": {
		Watches:  "Whether the router answers honestly, compared with a public resolver.",
		Never:    "It only records, and a missing answer is never read as agreement.",
		ShowsUp:  "On the Settings tab under Router DNS check.",
		Settings: true,
	},
	"attack_paths": {
		Watches: "The findings, joined into the ways in that matter, each with the one fix.",
		Never:   "It sends nothing and changes nothing.",
		ShowsUp: "On the Findings tab and in the nightly report.",
	},
	"weekly_summary": {
		Watches: "The counts of the week: devices, findings and alerts.",
		Never:   "It names no device and no address.",
		ShowsUp: "As a message to your phone once a week.",
	},
	"daily_check_in": {
		Watches: "That the Network module itself is still running.",
		Never:   "It tells you nothing else.",
		ShowsUp: "As a short message once a day.",
	},
	"agent_activity": {
		Watches: "Refused tool calls, approvals and new destinations of the agents on this computer.",
		Never:   "It reads a record of what the agents did and does not stop them.",
		ShowsUp: "As messages and on the Agents page.",
	},
	"secrets_pass": {
		Watches: "Workspace files, each night, for credentials left in them.",
		Never:   "It never shows a secret, only the file and the kind.",
		ShowsUp: "As a finding and a message.",
	},
}

// moduleFinds are the checks only one module produces, so the finding can say what found it. A check the audit's own scan also makes
// (an expired certificate, which nmap's script reports too) is left out: switching the module off would not make it go away. The
// Presence check lists none for the same reason: the nightly audit refreshes the device list itself, so its findings appear with the
// module off.
var moduleFinds = map[string][]string{
	"presence":        {netaudit.CheckNewDevice, netaudit.CheckMACChanged, netaudit.CheckGatewayChanged, netaudit.CheckDuplicateMAC, netaudit.CheckDeviceMissing},
	"nearby":          {netaudit.CheckRogueGateway},
	"router_log":      {netaudit.CheckRouterSignInFailures, netaudit.CheckWifiDisconnectBurst, netaudit.CheckWifiDeviceDisconnects},
	"decoys":          {netaudit.CheckDecoyTouched},
	"phone_home":      {netaudit.CheckPhoneHomeNewPlace, netaudit.CheckPhoneHomeBeaconing, netaudit.CheckPhoneHomeAroundDNS, netaudit.CheckPhoneHomeEncryptedDNS},
	"known_bad":       {netaudit.CheckPhoneHomeKnownBad},
	"login_watch":     {netaudit.CheckHostLoginFailures},
	"wifi_lookalikes": {netaudit.CheckWifiLookalike},
	"web_checks":      {netaudit.CheckWebExposure, netaudit.CheckSelfSigned},
	"outside_lookup":  {netaudit.CheckInternetExposedPort},
}

// described fills in the words of each module.
func described(modules []stack.Module) []stack.Module {
	for i := range modules {
		modules[i].About = moduleAbout[modules[i].ID]
		modules[i].Finds = moduleFinds[modules[i].ID]
	}
	return modules
}
