import type { ChoiceOption } from '../types/ui'
import type { DeviceAction, DeviceFilter, DeviceStatus, HelpSection, IPv6DiscoveryProblem, NearbyFilter, NearbyKind, NearbyStatus, NetauditHelpTopic, NetauditViewMode, RequirementCode, RequirementCopy } from '../types/netaudit'

// Network scan screens (plan network-audit, F-A): wording and fixed choices. The server names what is missing; the
// words around it live here.

export const NETAUDIT_VIEW_KEY = 'netaudit-view'
export const NETAUDIT_VIEW_VERSION = 1
export const DEFAULT_VIEW_MODE: NetauditViewMode = 'simple'

/** The Help button shows only a "?" to save width, so this is its name for a screen reader and its tooltip. */
export const HELP_LABEL = 'Help'

export const VIEW_MODE_OPTIONS: ChoiceOption[] = [
  { value: 'simple', label: 'Simple' },
  { value: 'advanced', label: 'Advanced' },
]

/** How long accepting a finding lasts; the server also refuses anything else it does not allow. */
export const DEFAULT_ACCEPT_DAYS = 90
export const ACCEPT_DAY_OPTIONS: ChoiceOption[] = [
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '1 year' },
]
export const ACCEPT_NOTE_MAX = 200
/** The most ports one deep check looks at (the server's netaudit.MaxDeepPorts). */
export const MAX_DEEP_PORTS = 16
/** The most ports one device's expectation may list; the server refuses more. */
export const MAX_EXPECTED_PORTS = 64

export const HELP_TOPIC_LABELS: Record<NetauditHelpTopic, string> = {
  basics: 'The basics',
  setup: 'Setting it up',
  reports: 'Reading a report',
  vulns: 'Known vulnerabilities',
  devices: 'New devices',
  nearby: 'Nearby devices',
  fixes: 'Fixing a finding',
  deep: 'The deep check',
  safety: 'Is it safe?',
  words: 'Words explained',
}

export const HELP_SECTIONS: Record<NetauditHelpTopic, HelpSection> = {
  basics: {
    title: HELP_TOPIC_LABELS.basics,
    paragraphs: [
      'A network scan looks at the devices on your own network and lists which doors are open on each one: the services that answer when something knocks.',
      'It only looks. It never changes a device, signs in to one or tries passwords.',
      'A nightly audit runs a scan on a schedule and tells you only what is new since the last time.',
      'Checks look for common weak settings on the services found: SMBv1, anonymous FTP, Telnet, a VNC, Redis, MongoDB, Memcached or MQTT broker with no password, an open Docker control port, Android debug bridge, open proxy, Elasticsearch, X11, rsync, Kubernetes or etcd port, expired or weak certificates, weak SSH settings and a few more. They only read; nothing tries a password and nothing changes on a device.',
    ],
  },
  setup: {
    title: HELP_TOPIC_LABELS.setup,
    paragraphs: [
      'Scanning stays off until you choose which network may be scanned. Use "Use my network" for your home or office network, or add addresses by hand in Advanced.',
      'A better scan needs a free program called nmap on the computer that runs this app. Without it only a limited scan (open ports, no names) works, and nightly audits do not run.',
      'The scan is also off while network access for agents is switched off in Sandboxing.',
    ],
  },
  reports: {
    title: HELP_TOPIC_LABELS.reports,
    paragraphs: [
      'Problems the checks found come first, grouped by kind: the same problem on several devices is one card that lists each place, with the fix written once. Below them, each device lists the services that answered. "New" means the last scan did not show it before. Something marked "Gone" was seen earlier and has disappeared.',
      'The first scan has nothing to compare with, so it marks nothing as new: what it found is saved as the baseline, and later scans point out what changed. Ports the scan could not identify are folded into one line per device.',
      '"This is fine" hides a finding you expect, such as your own file server, for 30 days, 90 days or a year. It comes back if the service changes, and you can take it back at any time.',
      'In Advanced, "Set as expected" on a device saves the ports open on it now as what it should offer. After that, any other open port is reported as a problem, and so is a listed port that stops answering. "Edit expected" changes the list, and "Remove expected" takes it back. Devices are told apart by their address, so if your router gives a device a new address, set it again.',
      'A scan can run out of time on a slow device, and is tried a second time before it gives up. Such a device is reported as not scanned to completion; that is not the same as having no open doors. In Advanced settings, "Time allowed per device" gives slow devices longer.',
    ],
  },
  vulns: {
    title: HELP_TOPIC_LABELS.vulns,
    paragraphs: [
      'When a scan recognises a program and its version, such as OpenSSH 9.6, the audit can compare it with a list of publicly known security flaws kept on this computer. If the version is on the list, the report says the software may be affected by those flaws.',
      '"May be affected" is not proof. A version number can look old while the maker has already fixed the flaw (Linux distributions often do), and some flaws only matter in a particular setup. If you have checked, use "This is fine" and it stays quiet until a new flaw appears for that version.',
      'The list comes from the U.S. National Vulnerability Database (NVD) and, to rank what matters first, CISA\'s Known Exploited Vulnerabilities catalog, which names flaws attackers are already using. Download it from Network → Settings. More than 100 MB is fetched, only when you press the button; the scan itself never uses the Internet.',
      'It can only look at what the scan recognised. Many routers, cameras and phones do not say which program or firmware they run, so they are not covered, and the report says how many services were looked at. Silence from this check does not mean a device is safe.',
      'Without the list, audits still run and say that the check was skipped. You can turn the check off for one audit in its settings.',
      'If this computer cannot reach the Internet, fetch the two files on another computer and pick them under "Import a file instead" (Advanced, or after a download stops). Importing asks for your passphrase again, because it replaces what every audit compares with.',
    ],
  },
  nearby: {
    title: HELP_TOPIC_LABELS.nearby,
    paragraphs: [
      'Nearby asks the whole network what each device is. It sends two small questions at once: one over mDNS ("which services do you offer?", how printers, TVs and cameras announce themselves) and one over UPnP ("who are you?"). It listens for about five seconds and keeps only the device type, the services, port numbers and a cleaned name, maker and model.',
      'It is off until you turn it on in Settings, and turning it on asks for your passphrase. Nothing is sent to the internet.',
      'What a device says is only a claim, so a type is shown as "advertises as". A device can say anything, which is why the problems Nearby reports use fixed words and never repeat what a device said.',
      'Silence is not proof. Some routers and access points block these questions, and some devices stay quiet, so a device that did not answer is shown as "not heard", never as gone, and a problem does not clear just because a look was quiet.',
      'A problem Nearby can report: a device that says it is an internet gateway but is not your router. The Fix button opens the steps.',
    ],
  },
  devices: {
    title: HELP_TOPIC_LABELS.devices,
    paragraphs: [
      'This computer keeps a list of the devices it has seen on your network, told apart by their hardware (MAC) address, which is read from this computer\'s own list of neighbours. It cannot see devices on another network, or what any device sends.',
      'The first time it looks, every device it finds counts as yours. After that a device it has not seen before is reported as new, until you press "It\'s mine" or give it a name. Naming a device also means you want to hear if it goes missing for three days.',
      'Phones and laptops often use a private hardware address to avoid being tracked. They are reported as new, but only as a note, and never as missing. Some phones pick a different private address every time they join, so each rejoin looks like a new device and sends a message. To stop that for your own phone, open its settings for this Wi-Fi network and use the phone\'s real address (iPhone: turn off Private Wi-Fi Address; Android: choose "Use device MAC"), join again, and press "It\'s mine" once.',
      '"Look for new devices" checks every few minutes, not only at night, and sends one short message when something new shows up. The message lists the addresses of the new devices, never their names. It is off until you turn it on. It needs Linux and at least one chat connector; otherwise it says so here.',
      'If the router\'s own hardware address changes, the report says so in red. A replaced or reset router does this, but so does someone pretending to be the router to read your traffic, so check before you confirm. Confirming asks for your passphrase again.',
      'An address that now belongs to a different device than the one you approved, one device answering for several addresses, and a device that answers the network but nothing else are reported as well. Each has a note on what is harmless and what is not.',
      'A full audit also asks your router, once, whether it offers UPnP: a setting that lets any device on the network open a port to the internet by itself. It is common and often needed by games and video calls, but malware uses it too, so it is reported as a note you can turn off in the router. It also reads the router\'s list of ports it has opened this way: each one is reported on the device it leads to, because anyone on the internet can try to connect to it.',
      'Decoy services are fake services on a few unused ports (an SSH login, file sharing and Telnet). Nothing on your network should ever use them, so a device that connects to one is probing for services, and you get a message with its address. They are off until you turn them on, and turning them on asks for your passphrase. If this computer has a firewall, allow the ports in. A scanner app on your own phone will also touch them; accept that finding.',
      'The IPv6 tab lists each device by its IPv6 addresses. Without help, this computer only knows devices it has already talked to, so "Look for IPv6 devices" makes each look send one small message to the devices on your network asking them to announce themselves. It needs a one-time setting on this computer (the screen shows the command if it is missing). The first look after you turn it on counts what it finds as yours; later arrivals are reported as new. A device found only over IPv6 is listed and you are told about it, but it has no finding of its own.',
      'A full audit also remembers each SSH server\'s host keys and each web service\'s certificate, and reports one that was replaced: a reinstalled server does this, and so does someone pretending to be it. A certificate renewed under the same name and issuer is only a note.',
    ],
  },
  fixes: {
    title: HELP_TOPIC_LABELS.fixes,
    paragraphs: [
      'For a problem a check found, "How to fix it" opens the steps, what could go wrong and how to undo it. The app never changes a device for you: you apply the fix yourself.',
      'After you have done it, press "I\'ve done this". That only records what you did and when. The problem stays in the report until a scan no longer finds it.',
      'If the next scan still shows it, the report says so plainly ("Still showing after you marked it fixed"). Check the steps again, or use "This is fine" if you have decided to live with it. If the scan no longer finds it, the report lists it as fixed and says you marked it.',
      'Marking something as done never hides it and never changes the alerts you get. "Take back" removes your mark.',
      'In Advanced, the commands are there to copy. Read each one first: they are written for common systems and yours may differ.',
    ],
  },
  deep: {
    title: HELP_TOPIC_LABELS.deep,
    paragraphs: [
      'The deep check looks harder at one device, and only when you ask for it from the Advanced view. It is the one thing here that sends more than ordinary connections, so it never runs on a schedule and no assistant can start it. You confirm your passphrase first.',
      'It reads which ports the last scan found open and looks at those, up to 16. On a service that speaks TLS (secure web pages and similar) it opens about 60 connections in a row to list which TLS versions and ciphers the service still accepts. A fragile device, such as an old printer or camera, can slow down or restart under that.',
      'On an SSH service it starts one login with a made-up user name, only to learn whether the server accepts passwords. It guesses no password and changes nothing, but the device will log a failed login from this computer.',
      'It reports old TLS versions, weak ciphers and SSH password login. A device with no TLS or SSH service gives it nothing to look at: the result then says so, which is not the same as nothing being wrong.',
      'A device can be checked once every ten minutes. The result is kept, and every later nightly audit carries its findings, marked with the date of the deep check, until the device is checked again (after 90 days an old result is dropped).',
    ],
  },
  safety: {
    title: HELP_TOPIC_LABELS.safety,
    paragraphs: [
      'Only addresses inside your home or office network can be scanned. Internet addresses are always refused, whatever you type.',
      'A normal scan or audit only looks: it never signs in to a device or tries a password. The deep check is the one exception, and it is described in its own part of this help.',
      'The scan runs in a private folder, and the program is limited to it by the operating system where that is possible. Where it is not, scans refuse to run unless you allow it for development.',
      'Nightly audits use a model that runs on this computer, so the list of your devices never leaves it.',
      'The Internet view is one of two exceptions to that. It is off until you turn it on. When it is on, each nightly audit asks your router for its internet address (or uses the one you typed in) and sends that address, and nothing else, to an outside scan database, which learns your public internet address and lists the ports it has seen open there. Its information can be up to a week old. If the router does not share its address and you have not entered it, or the service cannot be reached, the audit says it could not look, which does not mean nothing is open.',
      'The Router DNS check is the other. It is off until you turn it on, and turning it on asks for your passphrase. When it is on, this computer asks your router for the address of a few well-known names, asks an outside resolver the same, and records whether the answers agree. The outside resolver learns the names it asks and your public internet address. It only keeps a record and raises no alert. If either side does not answer, that is recorded as not compared, which does not mean the answers agree.',
    ],
  },
  words: {
    title: HELP_TOPIC_LABELS.words,
    paragraphs: [
      'Port: a numbered door on a device. Each service listens at one, such as 22 for remote login or 443 for secure web pages.',
      'Range: a block of addresses, written like 192.168.1.0/24, which means every device from .1 to .254 on that network.',
      'Service: the program answering at a port. When the scan could only guess it from the port number, it says "(guess)".',
      'Confinement: the operating system keeping a program inside its own folder.',
    ],
  },
}

export const SIMPLE_USE_NETWORK = 'Use my network'
export const SIMPLE_ACCEPT = 'This is fine'

/** The readiness "confirm" fix the settings page can meet (the server's core/netaudit.ActionAllowUnconfined). */
export const ACTION_ALLOW_UNCONFINED = 'allow_unconfined'

export const REQUIREMENT_COPY: Record<RequirementCode, RequirementCopy> = {
  scope_empty: { title: 'Choose which network to look at', simple: 'Scanning is off until you say which network is yours.', help: 'setup' },
  host_network_off: { title: 'Network access is switched off', simple: 'This computer is not letting agents use the network, so nothing can be scanned.', help: 'setup' },
  nmap_missing: { title: 'A helper program is missing', simple: 'A free program called nmap gives better results. Until it is installed only a limited scan works.', help: 'setup' },
  vuln_data_missing: { title: 'Known-vulnerability data is not downloaded', simple: 'Audits still run, but they skip the check for known software flaws until the data is downloaded.', help: 'vulns' },
  vuln_data_old: { title: 'Known-vulnerability data is out of date', simple: 'Audits compare against an old list, so newer software flaws are missed. Updating downloads more than 100 MB.', help: 'vulns' },
  no_sandbox: { title: "This computer can't fence the scan in", simple: 'Scans will not run here unless you allow them without that protection.', help: 'safety' },
}

/** The readiness notices (not requirements) about the vulnerability data; they never stop a scan. */
export const VULN_NOTICES: readonly string[] = ['vuln_data_missing', 'vuln_data_old']
/** The one notice the banner can act on in place: the data is held but old. */
export const VULN_OLD_NOTICE = 'vuln_data_old'

/** The agent profile whose automations scan the network; their pages show the readiness banner and the report. */
export const AUDITOR_PROFILE = 'security-auditor'

/** The playbook an audit starts from; it declares the agent, network access, journal and first schedule it needs. */
export const AUDIT_PLAYBOOK = 'network-audit'

/** The page anchor of the scan-time control, so a warning can take you straight to it. */
export const SCAN_TIME_ANCHOR = 'scan-time'

/** The per-device time limits the server offers (netaudit.AllowedDeviceTimeouts); 0 is each depth's standard. */
export const DEVICE_TIME_OPTIONS: ChoiceOption[] = [
  { value: '0', label: 'Standard' },
  { value: '5', label: '5 minutes' },
  { value: '10', label: '10 minutes' },
]

/** The depths an audit may scan at. */
export const SCAN_DEPTH_OPTIONS: ChoiceOption[] = [
  { value: 'thorough', label: 'Thorough (a few minutes)' },
  { value: 'quick', label: 'Quick (seconds)' },
]

/** The wording around the vulnerability data: where it comes from, and what a match does and does not say. */
export const VULN_ATTRIBUTION = "Data from the U.S. National Vulnerability Database (NVD) and CISA's Known Exploited Vulnerabilities catalog. Not endorsed by NIST or CISA."
export const VULN_NOT_PROOF = 'A match means the software may be affected, not proof that it is: the maker may have fixed it without changing the version number.'
export const VULN_IMPORT_INTRO = 'If this computer cannot download the data, fetch the file on another computer and pick it here. It is checked before it is used, and nothing in it is shown. Pick the compressed .gz file as NVD publishes it: it is much smaller than the unpacked one.'
/** The two official files an import takes, with where to get each (the same fixed addresses the download uses). */
export const VULN_IMPORT_FILES: readonly { source: 'nvd' | 'kev'; name: string; where: string }[] = [
  { source: 'nvd', name: 'NVD yearly file', where: 'https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-YEAR.json.gz' },
  { source: 'kev', name: "CISA's exploited list", where: 'https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json' },
]
export const VULN_DOWNLOAD_SIZE = 'Downloading fetches more than 100 MB, only when you press the button.'

/** Network · Devices: the words the card uses. */
export const DEVICES_TITLE = 'Devices on your network'
export const DEVICES_LOAD_FAILED = 'The list of devices could not be read.'
export const DEVICES_ACTION_FAILED: Record<DeviceAction, string> = {
  name: 'The name could not be saved.',
  approve: 'The device could not be approved.',
  important: 'The mark could not be saved.',
  forget: 'The device could not be removed from the list.',
  'dismiss-suggestion': 'The suggested name could not be dismissed.',
  'confirm-gateway': 'The router\'s address could not be confirmed.',
}
export const SUGGESTION_DISMISS = 'Not this'
export const UNNAMED_HINT = 'A name lets the check tell you when a device you know has not been on the network for days. A phone with a private address, or a device with no IPv4 address, is not counted, because that check cannot follow it.'
export const PRESENCE_LOAD_FAILED = 'The new-device check could not be read.'
export const PRESENCE_SAVE_FAILED = 'The new-device check could not be changed.'
export const PRESENCE_HINT = 'Notices a device that was not on the network before and messages you.'
export const PRESENCE_DETAILS = 'Looks for a device that was not on the network before, and sends you a short message the first time one shows up. It only connects to the networks allowed above; it never looks at what a device sends.'
export const IPV6_LOAD_FAILED = 'The IPv6 setting could not be read.'
export const IPV6_HINT = 'Finds devices that only show up over IPv6.'
export const IPV6_DETAILS = 'Some devices only show up over IPv6. Each look sends one small message to the devices on your network asking them to announce themselves, and lists the ones that answer. It does not scan or connect to anything.'
export const IPV6_FIX_COMMAND = 'sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"'
export const IPV6_FIX_KEEP_COMMAND = 'echo \'net.ipv4.ping_group_range = 0 2147483647\' | sudo tee /etc/sysctl.d/90-ping-group.conf'
export const IPV6_PROBLEMS: Record<IPv6DiscoveryProblem, string> = {
  not_permitted: 'This computer does not let the program send that message (the ping_group_range setting). Run this on the computer, then look again. It also lets other accounts on it send pings.',
  unavailable: 'Only a Linux computer can look for IPv6 devices.',
  failed: 'The message could not be sent. Check the server log, then look again.',
}
export const IPV6_FIX_KEEP_NOTE = 'To keep it after a restart:'
export const DEVICES_NONE_YET = 'none yet'
/** A device counts as online when it was seen this recently. The server keeps when a device was last seen, not whether it is there now; 30 minutes is the longest gap the new-device check can be set to. */
export const DEVICE_ONLINE_WINDOW_MS = 30 * 60 * 1000
export const DEVICE_FILTER_LABEL = 'Show devices'
export const DEVICE_FILTERS: readonly DeviceFilter[] = ['all', 'attention', 'online', 'offline', 'unnamed']
export const DEVICE_FILTER_LABELS: Record<DeviceFilter, string> = { all: 'All', attention: 'Needs a look', online: 'Online', offline: 'Offline', unnamed: 'Unnamed' }
export const DEVICE_GROUP_TITLES: Record<DeviceStatus, string> = { attention: 'Needs a look', online: 'Online', offline: 'Offline' }
export const DEVICE_SEARCH_LABEL = 'Search devices'
export const DEVICE_SEARCH_PLACEHOLDER = 'Search by name or address'
export const DEVICES_NO_MATCH = 'No device matches. Clear the search or choose All.'
export const DEVICES_CHECK_OFF = 'The new-device check is off, so Online is only as fresh as the last scan. Turn it on in Network → Settings.'
export const deviceCount = (count: number) => `${count} ${count === 1 ? 'device' : 'devices'}`
export const DEVICE_NAME_MAX = 40
export const DEVICE_UNNAMED = 'Unknown device'
export const DEVICE_PRIVATE = 'Phone or laptop (private address)'
export const DEVICES_EMPTY = 'No devices seen yet. Turn on the new-device check in Network → Settings, or run a network audit.'
export const DEVICES_BASELINE_NOTE = 'The first look is taken as the normal state: every device found then counts as approved, and only later arrivals are reported.'
export const GATEWAY_CHANGED_TITLE = 'The router\'s hardware address changed'
export const GATEWAY_CHANGED_BODY = 'Something else may be answering for the router, which would let it read or change the network\'s traffic. A replaced or reset router does this too. If you know why, confirm it.'
export const FORGET_CONFIRM = {
  title: 'Remove this device from the list?',
  message: 'It will come back as a new device if it is still on the network.',
  confirmText: 'Remove',
} as const

/** Network · Settings · Decoy services: the words the card uses. */
export const DECOYS_TITLE = 'Decoy services'
export const DECOYS_LOAD_FAILED = 'The decoy services could not be read.'
export const DECOYS_CLEAR_FAILED = 'The list could not be cleared.'
export const DECOYS_CLEAR_LABEL = 'Clear list'
export const DECOYS_CLEAR_TITLE = 'Clear the list of connections?'
export const DECOYS_CLEAR_MESSAGE = 'The list of devices that connected to a decoy is emptied. The decoy services stay as they are, and the next connection is listed and alerted again.'
export const DECOYS_HINT = 'Fake services that note any device that connects to them.'
export const DECOYS_DETAILS = 'Fake services on a few unused ports that nothing on your network should ever use. A device that connects to one is probing for services, which is what an intruder or malware does first. They only note who connected; they read a few bytes, throw them away and run nothing. Off until you turn them on, and turning them on asks for your passphrase. A scanner app on your own phone or laptop will also touch them; accept that finding.'
export const DECOYS_FIREWALL = 'If this computer has a firewall, the ports must be allowed in, or connections never arrive.'
export const DECOYS_CONNECTIONS_TITLE = 'Connections'
export const DECOYS_NEEDS_LOOK = 'Needs a look'
export const DECOYS_NONE = 'Nothing has connected to a decoy.'

/** The Network page: tabs, headlines and the words around the numbers. */
export const NETWORK_TAB_LABELS = { devices: 'Devices', findings: 'Findings', timeline: 'Timeline', audits: 'Audits', modules: 'Modules', settings: 'Settings' } as const
export const NETWORK_OVERVIEW_LABEL = 'Overview'
export const NETWORK_TITLE = 'Network'
export const NETWORK_SUBTITLE = 'Who is on your network, what needs a look, and what changed.'
export const NETWORK_OVERVIEW_LOAD_FAILED = 'The network overview could not be read.'
export const NETWORK_REFRESH_MS = 60_000
export const NETWORK_TREND_DAYS = 30
export const NETWORK_POSTURE_HEADLINES = {
  unknown: 'Not looked at yet',
  quiet: 'Your network looks as you left it',
  attention: 'Something needs a look',
} as const
export const NETWORK_POSTURE_UNKNOWN_NEXT = 'Turn on the new-device check in Devices, or set up a network audit, and this page fills in.'
export const NETWORK_NO_AUDIT = 'No network audit is set up, so findings are not shown.'
export const DEVICE_SETTINGS_TITLE = 'Devices'
export const AUDIT_HISTORY_SHOWN = 5
export const AUDIT_HISTORY_LABEL = 'Recent runs'
export const SETTINGS_SAVES_AT_ONCE = 'Every change saves at once.'
export const SETTINGS_WATCHING_TITLE = 'Watching for changes'
export const SETTINGS_EXTRAS_TITLE = 'Optional extras'
export const ADVANCED_SETTINGS_TITLE = 'Advanced'
export const MORE_ACTIONS_LABEL = 'More actions'
export const AUDITS_NONE = 'Scan on a schedule and be told only what is new, changed or gone. This opens the new automation form with everything filled in; you can change any of it.'
export const AUDITS_SET_UP = 'Set up a network audit'
export const AUDITS_ADD_ANOTHER = 'Add another audit'
export const AUDITS_ADD_ANOTHER_HINT = 'Another audit can use a different schedule, depth or ports. All audits use the networks allowed in Network → Settings.'
export const AUDITS_FIX_FIRST = 'Fix the items at the top of the page first.'
export const AUDITS_NEED_NETWORK = 'Scanning is off until a network is allowed.'
export const AUDITS_OPEN_SETTINGS = 'Choose the networks in Settings'
export const NETWORK_NEXT_DEVICES = 'Review devices'
export const NETWORK_NEXT_FINDINGS = 'Review findings'
export const NETWORK_NEXT_SET_UP_AUDIT = 'Set up a network audit'
export const NETWORK_NEXT_TURN_ON_CHECK = 'Turn on the new-device check'
export const NETWORK_CHECK_PROBLEM = 'The new-device check cannot look right now'
export const NETWORK_RECENT_TITLE = 'What changed lately'
export const NETWORK_RECENT_NONE = 'Nothing has changed in the last two weeks.'
export const NETWORK_AUDITS_TITLE = 'Network audits'
export const NETWORK_FINDINGS_NEEDS_AUDIT = 'Findings come from a network audit. Set one up and its results appear here.'
export const NETWORK_TREND_NOTE = 'Counts only, kept for 13 months. Looking never adds scanning: these come from checks that already run.'
export const NETWORK_POINTER_BODY = 'Part of Network: the devices, trends and what changed are on that page; this one runs and schedules the audit.'
export const NETWORK_POINTER_LINK = 'Open Network'

/** The name a device gave when asked: a hint, never the device's identity. */
export const SUGGESTION_NEW_DEVICE = (name: string): string => `It calls itself “${name}”. Anyone can choose that, so check it is yours first.`
export const SUGGESTION_APPROVED = (name: string): string => `It calls itself “${name}”.`
export const SUGGESTION_ACCEPT = 'Use this name'

/** Devices · Suggest names for devices. */
export const NAMES_LOAD_FAILED = 'The device names setting could not be read.'
export const NAMES_HINT = 'Asks devices you have not named what they call themselves.'
export const NAMES_DETAILS = 'Each look asks a device you have not named what it calls itself: one small question, no more than once a week for a device that answers, and less and less often for one that does not. The answer is only offered to you to confirm.'
export const FOLLOW_UP_HINT = 'Scans a device once, as soon as the new-device check finds it.'
export const FOLLOW_UP_DETAILS = 'When the new-device check finds a device nobody has approved or named, this runs one ordinary scan of that device alone, so you learn within minutes what it offers (an open Telnet port, a camera stream) instead of at the next nightly audit. The alert gives the count of open ports and what is worth a look; the nightly audit still compares it with the rest. It scans only inside your scan networks, one scan at a time, never the deep check, and no model is involved. Devices with a private hardware address (phones and laptops that change it) are left out, each device is scanned at most once a day, and no more than three an hour. Scans run one at a time, so a nightly audit that starts while one runs is told a scan is already running, as it is during a new-device sweep. It adds scan traffic during the day, so it is off until you turn it on, and turning it on asks for your passphrase. It needs the new-device check on.'
export const FOLLOW_UP_LOAD_FAILED = 'The scan-on-arrival setting could not be read.'

export const ROUTER_LOG_TITLE = 'Router log'
/** The extra router setting the Phone-home watch needs: the router must log the names devices ask for. */
export const ROUTER_LOG_DNS_NOTE = 'The Phone-home watch also needs the router to log DNS queries. On an Asuswrt-Merlin router, add the line log-queries to /jffs/configs/dnsmasq.conf.add, set the log level to debug (nvram set log_level=7, then nvram commit), and restart dnsmasq or the router. Other routers: look for "log DNS queries" in the router\'s settings.'
/** Advanced: how busy the router's DNS log is, so the limit on those lines can be tuned from the real rate. */
export const routerLogQueriesLine = (read: number, limited: number, peak: number, since: string): string =>
  `DNS lines since ${since}: ${read.toLocaleString('en-US')} read, ${limited.toLocaleString('en-US')} turned away by the limit, ${peak} in the busiest second.`
export const ROUTER_LOG_LOAD_FAILED = 'The router log setting could not be read.'
export const ROUTER_LOG_HINT = 'Lets the router tell this computer at once when a device joins.'
export const ROUTER_LOG_DETAILS = 'Lets your router tell this computer, as it happens, that a device was given an address or joined the Wi-Fi. If the list does not know the device, the presence check looks straight away instead of at its next turn, so a new device is noticed in seconds instead of minutes. It only wakes the check, and a device\'s own host name in the log may be offered as a name suggestion on the Devices card, which you confirm. The log also tells this computer when the router refuses a connection attempt from the internet: only the protocol and the port are counted, as a number for the day (shown on the Overview and the Timeline, never as an alert), and no address and no line of the log is kept. Nothing else from the log is kept, shown or acted on, and the list of devices still comes from this computer\'s own view of the network. Needs the presence check to be on. Off until you turn it on, and turning it on asks for your passphrase. Anything on your network can send a message that looks like the router; at worst that makes the check look early, never more than once a minute, or makes the day\'s count of refused attempts wrong.'
export const ROUTER_LOG_NOT_HEARD = 'Nothing has been heard from your router yet. Check the router setting and the redirect below.'
export const ROUTER_LOG_SETUP_TITLE = 'Point the router at this computer'
export const ROUTER_LOG_SETUP_STEPS = [
  "In the router's settings, open the system log and set the remote log server to this computer's address (the port stays 514).",
  'The router can only send to port 514, which the program may not open, so this computer passes it on to a port the program may use. Run the commands below once, on this computer. They are shown, not run for you.',
]
export const ROUTER_LOG_SETUP_COMMAND = [
  'sudo nft add table ip syslogredirect',
  "sudo nft 'add chain ip syslogredirect prerouting { type nat hook prerouting priority dstnat; policy accept; }'",
  'sudo nft add rule ip syslogredirect prerouting udp dport 514 redirect to :5514',
].join('\n')
export const ROUTER_LOG_SETUP_AFTER = 'These commands last until the next restart. To keep them, add the same table to /etc/nftables.conf and enable the nftables service. To check them, run "sudo nft list ruleset", or listen on port 5514 and see datagrams from the router arrive. To undo, run:'
export const ROUTER_LOG_UNDO_COMMAND = 'sudo nft delete table ip syslogredirect'

export const SCAN_CHECKS_HINT = 'Looks for common weak settings, read only.'
export const SCAN_CHECKS_DETAILS = 'SMBv1, anonymous FTP, a VNC or Redis with no password, exposed NFS shares, expired certificates and a few more. They only read; nothing tries a password and nothing changes on a device.'
export const SCAN_KNOWN_VULNS_HINT = 'Matches recognised software against publicly known flaws.'
export const SCAN_KNOWN_VULNS_DETAILS = 'Compares the software the scan recognises with a list of publicly known flaws kept on this computer. A match means it may be affected, not that it is. The list is downloaded in Network → Settings; without it the audit still runs and says this check was skipped.'

// Network → Devices → Internet view: the opt-in look at what the internet can reach.
export const OUTSIDE_LOOKUP_TITLE = 'Internet view'
export const OUTSIDE_LOOKUP_HINT = 'Asks an outside service which of your ports it can see open.'
export const OUTSIDE_LOOKUP_DETAILS = 'Each nightly audit asks your router for its internet address, then sends that address, and nothing else, to an outside scan database. That service learns your public internet address and that it is looked up from a home network. It answers with the ports it has seen open there; its information can be up to a week old, and it can miss a port. Any port it lists is reported as a finding on the router. It works only when your router tells this computer its internet address (UPnP) or you type that address in below, and it says so when it cannot look. It looks at your IPv4 address only: a device with its own public IPv6 address is reachable in other ways this check does not see. Off until you turn it on, and turning it on asks for your passphrase.'
export const OUTSIDE_LOOKUP_LOAD_FAILED = 'The internet view setting could not be read.'
export const OUTSIDE_LOOKUP_SAVE_FAILED = 'The internet view setting could not be changed.'
export const OUTSIDE_LOOKUP_NOT_YET = 'Not looked yet. The next nightly audit makes the first look.'
export const OUTSIDE_LOOKUP_UNKNOWN = 'That does not mean nothing is open.'
export const OUTSIDE_LOOKUP_NO_PORTS = 'no open ports'
export const OUTSIDE_LOOKUP_PORTS_SEEN = 'They are in Findings.'
export const OUTSIDE_LOOKUP_FAILED = 'The outside service could not be reached, so nothing was found. Check that internet access is allowed in the network settings.'
export const OUTSIDE_LOOKUP_LAST_LOOKED = 'Last looked'
export const OUTSIDE_LOOKUP_LISTS = 'the service lists'
export const OUTSIDE_LOOKUP_ON_ADDRESS = 'on your address.'
export const OUTSIDE_LOOKUP_READ_FAILED_TITLE = 'The internet view setting could not be read'
export const OUTSIDE_LOOKUP_READ_FAILED_NEXT = 'Check that the server is running, then retry.'
export const OUTSIDE_LOOKUP_READING = 'Reading the internet view setting'
export const OUTSIDE_LOOKUP_RETRY = 'Retry'
export const OUTSIDE_LOOKUP_INTERNET_OFF = 'internet_off'
export const OUTSIDE_LOOKUP_OPEN_SETTINGS = 'Open the guardrail settings'
export const OUTSIDE_LOOKUP_REASONS: Record<string, string> = {
  internet_off: 'Could not look: internet access is off, so nothing was sent. Allow "Internet" under "Agent network access" in Settings → Guardrails.',
  router_not_asked: 'Could not look: your router could not be asked this time. Nothing was sent.',
  upnp_off: 'Could not look: your router does not share its internet address (UPnP is off or does not answer). Nothing was sent.',
  no_address: 'Could not look: your router did not give an internet address. Nothing was sent.',
  private_address: 'Could not look: the internet address your router reports is a private one, which usually means another router sits in front of it. Nothing was sent.',
  shared_address: 'Could not look: your provider shares one internet address between many homes, so there is no address of yours to look up. Nothing was sent.',
  link_local_address: 'Could not look: your router has no internet address yet. Nothing was sent.',
}
export const OUTSIDE_LOOKUP_REASON_OTHER = 'Could not look. Nothing was sent.'
export const OUTSIDE_LOOKUP_AT_ENTERED = 'at the address you entered'
export const OUTSIDE_LOOKUP_DIFFERS = "Your router reports a different public address than the one you entered, so the one you entered may be out of date. The router's was used."
export const OUTSIDE_LOOKUP_ADDRESS_LABEL = 'Your public internet address (optional)'
export const OUTSIDE_LOOKUP_ADDRESS_HINT = 'Used only when your router does not share it (UPnP is off). Find it on your router\'s status page, often called "WAN IP" or "Internet IP". If your provider changes it, update it here. It is sent to the outside service like the one the router gives.'
export const OUTSIDE_LOOKUP_ADDRESS_SAVE = 'Save address'
export const OUTSIDE_LOOKUP_NEEDS_ADDRESS = ['upnp_off', 'no_address', 'private_address']
export const OUTSIDE_LOOKUP_URL_LABEL = 'Service address'
export const OUTSIDE_LOOKUP_URL_HINT = 'Must start with https://. Leave empty to use the default service.'
export const OUTSIDE_LOOKUP_URL_SAVE = 'Save address'
export const DNS_CHECK_TITLE = 'Router DNS check'
export const DNS_CHECK_HINT = 'Records whether your router and an outside resolver agree. It raises no alerts yet.'
export const DNS_CHECK_DETAILS = "Every so often this computer asks your router's DNS for a few names, then asks an outside resolver the same questions over HTTPS, and records whether the answers agree. A router that has been taken over can send you to the wrong place for a name; this is one way to notice. The outside resolver learns which names are asked and your public internet address. Names whose addresses never change are used, so a difference means something, and a connection that cannot be made is recorded as \"could not compare\", never as a match. It catches a router that redirects everything, not one that lies about a single site. For now it only keeps a record: after a week of results it will be decided whether a difference should raise an alert. Off until you turn it on, and turning it on asks for your passphrase."
export const DNS_CHECK_LOAD_FAILED = 'The DNS check setting could not be read.'
export const DNS_CHECK_SAVE_FAILED = 'The DNS check setting could not be changed.'
export const DNS_CHECK_READ_FAILED_TITLE = 'The DNS check setting could not be read'
export const DNS_CHECK_READ_FAILED_NEXT = 'Check that the server is running, then retry.'
export const DNS_CHECK_READING = 'Reading the DNS check setting'
export const DNS_CHECK_RETRY = 'Retry'
export const DNS_CHECK_INTERNET_OFF = 'internet_off'
export const DNS_CHECK_INTERNET_OFF_LINE = 'Internet access is off, so nothing could be compared and nothing was sent. Allow "Internet" under "Agent network access" in Settings → Guardrails.'
export const DNS_CHECK_LAN_OFF = 'lan_off'
export const DNS_CHECK_LAN_OFF_LINE = 'Local network access is off, so your router could not be asked and nothing was sent. Allow "Local network (LAN)" under "Agent network access" in Settings → Guardrails.'
export const DNS_CHECK_SERVER_LABEL = "Router's DNS address"
export const DNS_CHECK_SERVER_HINT = 'A private address such as 192.168.1.1. Leave empty to use your router (the default gateway). Change it only if your network hands out a different DNS server.'
export const DNS_CHECK_OPEN_SETTINGS = 'Open the guardrail settings'
export const DNS_CHECK_NOT_YET = 'Nothing compared yet. The first comparison is made soon after you turn it on.'
export const DNS_CHECK_NONE_COMPARABLE = 'Nothing could be compared yet: your router or the outside resolver did not answer. That does not mean the answers agree.'
export const DNS_CHECK_RECORD_ONLY = 'This only keeps a record.'
export const DNS_CHECK_NAMES_LABEL = 'Names to compare'
export const DNS_CHECK_NAMES_HINT = 'One per line, up to 5. Use names whose address never changes. Leave empty for the defaults.'
export const DNS_CHECK_INTERVAL_LABEL = 'How often to compare'
export const DNS_CHECK_URL_LABEL = 'Outside resolver address'
export const DNS_CHECK_URL_HINT = 'A DNS-over-HTTPS address that starts with https://. Leave empty to use the default.'
export const DNS_CHECK_LAST_CHECKED = 'Last checked'
export const DNS_CHECK_STOPPED_AFTER_RESTART = 'No check has run since the program started.'
export const DNS_CHECK_LAST_COMPARISON = 'The last comparison was'
export const DNS_CHECK_LATE_BUT = 'but one is due every'
export const DNS_CHECK_LATE_MAY_HAVE_STOPPED = 'minutes, so it may have stopped. If this stays, restart the program.'
export const DNS_CHECK_LAST_CHECK_WAS = 'The last check was'
export const DNS_CHECK_MAX_DIFFERENCES = 5
export const DNS_CHECK_NO_ADDRESS = 'no address'
export const DNS_CHECK_DIFFERENCES_TITLE = 'Differences'
export const DNS_CHECK_DIFFERENCES_HINT = 'A difference is not proof of a problem: some names answer differently by place and time. Only a difference that keeps coming back for a name that should not change is worth acting on.'
export const DNS_CHECK_ROUTER_SAID = 'Router:'
export const DNS_CHECK_OUTSIDE_SAID = 'Outside:'
export const DNS_CHECK_PARTLY = 'partly differed'
export const DNS_CHECK_MORE_DIFFERENCES = 'more differences are recorded. The newest 20 are in the results you can copy (Advanced view).'
export const DNS_CHECK_SAVE = 'Save settings'
export const DNS_CHECK_COPY = 'Copy the results'
export const DNS_CHECK_COPY_HINT = 'Copies the counts and any differences, to bring back after a week.'
export const DNS_CHECK_COPIED = 'Copied'
export const DNS_CHECK_COPY_FAILED = 'Copy failed'
export const SCAN_UNCONFINED_HINT = 'For development only.'
export const SCAN_UNCONFINED_DETAILS = 'Normally nmap is kept inside its own folder by the operating system, and refuses to run where that is not possible.'

// ---- Nearby: what devices say they are (mDNS and SSDP) ----

export const NEARBY_TITLE = 'Nearby'
export const NEARBY_SETTINGS_TITLE = 'Nearby devices'
export const NEARBY_HINT = 'Sends two small questions to the whole network and listens for the answers. Off until you turn it on.'
export const NEARBY_DETAILS = 'When you check, this computer sends two questions to every device on your network at once: one over mDNS ("which services do you offer?", the way printers and TVs announce themselves) and one over UPnP ("who are you?"). It then listens for about five seconds and reads the answers, and may read a short description page from a device that says where it is. Only the device type, services, port numbers and a cleaned name, maker and model are kept. Nothing else a device sends is stored, and nothing is ever sent to the internet. What a device says is only a claim: a device can say anything, so it is shown as "advertises as". Some routers and access points block these questions, and some devices stay quiet, so a device that does not answer is not gone. Off until you turn it on, and turning it on asks for your passphrase.'
export const NEARBY_CHECK_NOW = 'Check now'
export const NEARBY_CHECKING = 'Looking…'
export const NEARBY_OFF_BODY = 'Nearby is off. When it is on, this computer sends two small questions to everything on your network ("which services do you offer?" and "who are you?") and listens for a few seconds for the answers. Nothing is sent while it is off.'
export const NEARBY_TURN_ON = 'Turn it on in Settings'
export const NEARBY_NEVER = 'Nearby is on but has not looked yet. Press Check now.'
export const NEARBY_RUNNING = 'Looking… this takes a few seconds.'
export const NEARBY_NOTHING = 'Nothing answered. That does not mean nothing is there: some routers and access points block these questions, and some devices stay quiet.'
export const NEARBY_FAILED: Record<string, string> = {
  lan_off: 'Could not look: local network access is off in Guardrails. Turn it on in Settings, then check again.',
  no_interface: 'Could not look: no network connection inside the networks you allowed is up.',
  refused: 'Could not look: the system would not let this program send the questions.',
  unavailable: 'Nearby is not available on this system.',
}
export const NEARBY_READ_FAILED_TITLE = 'Nearby could not be read'
export const NEARBY_LOAD_FAILED = 'Failed to read what devices said about themselves.'
export const NEARBY_LOOK_FAILED = 'Failed to start a look.'
export const NEARBY_NO_MATCH = 'No device matches this filter.'
export const NEARBY_SILENCE_NOTE = 'A device that did not answer is not gone: some do not answer every time.'
export const NEARBY_ADVERTISES_AS = 'Advertises as'
export const NEARBY_FILTER_LABEL = 'Show nearby devices'
export const NEARBY_FILTERS: readonly NearbyFilter[] = ['all', 'attention', 'new', 'quiet']
export const NEARBY_FILTER_LABELS: Record<NearbyFilter, string> = { all: 'All', attention: 'Needs a look', new: 'New', quiet: 'Not heard' }
export const NEARBY_STATUS_LABELS: Record<NearbyStatus, string> = { attention: 'Needs a look', new: 'New', heard: 'Heard in the last look', quiet: 'Not heard in the last look' }
export const NEARBY_GROUP_TITLES: Record<NearbyStatus, string> = { attention: 'Needs a look', new: 'New', heard: 'Heard in the last look', quiet: 'Not heard in the last look' }
export const NEARBY_KIND_LABELS: Record<NearbyKind, string> = { printer: 'Printer', tv: 'TV or speaker', camera: 'Camera', computer: 'Computer or server', gateway: 'Internet gateway', other: 'Something else' }
export const NEARBY_KIND_ICONS: Record<NearbyKind, string> = { printer: 'printer', tv: 'tv', camera: 'camera', computer: 'computer', gateway: 'globe', other: 'device' }
export const NEARBY_NOT_LISTED = 'Not in your device list'
export const NEARBY_FIX = 'Fix'
export const NEARBY_CLOSE_DETAIL = 'Close details'
export const NEARBY_COPY = 'Copy the results'
export const NEARBY_COPY_HINT = 'Copies a short summary of what was heard, to bring back for review.'
export const NEARBY_COPIED = 'Copied.'
export const NEARBY_COPY_FAILED = 'Could not copy. Select and copy it by hand.'
export const NEARBY_SERVICE_NAMES: Record<string, string> = {
  '_ipp._tcp': 'Printing (IPP)', '_ipps._tcp': 'Printing (IPP)', '_printer._tcp': 'Printing', '_pdl-datastream._tcp': 'Printing', '_airplay._tcp': 'AirPlay',
  '_googlecast._tcp': 'Google Cast', '_raop._tcp': 'AirPlay audio', '_spotify-connect._tcp': 'Spotify Connect', '_rtsp._tcp': 'Video stream (RTSP)', '_ssh._tcp': 'SSH',
  '_smb._tcp': 'File sharing (SMB)', '_afpovertcp._tcp': 'File sharing (AFP)', '_http._tcp': 'Web page', '_https._tcp': 'Web page (secure)', '_workstation._tcp': 'Computer',
  '_companion-link._tcp': 'Apple device', '_homekit._tcp': 'HomeKit', '_hap._tcp': 'HomeKit',
}

// The Timeline tab: one timeline of what happened lately. Simple shows the latest few rows, Advanced a long list with each
// device's address.
export const ACTIVITY_TITLE = 'What happened lately'
export const ACTIVITY_LIMIT_SIMPLE = 20
export const ACTIVITY_LIMIT_ADVANCED = 100
export const ACTIVITY_FILTER_LABEL = 'Show activity'
export const ACTIVITY_FILTER_OPTIONS = [
  { value: 'all', label: 'All' },
  { value: 'devices', label: 'Devices' },
  { value: 'ports', label: 'Ports' },
  { value: 'findings', label: 'Findings' },
  { value: 'alerts', label: 'Alerts' },
]
/** Ports a person would know by name; any other port is shown as its number. */
export const PORT_NAMES: Record<number, string> = {
  21: 'FTP', 22: 'SSH', 23: 'Telnet', 25: 'mail', 53: 'DNS', 80: 'web', 110: 'POP3', 143: 'IMAP', 443: 'HTTPS', 445: 'file sharing',
  1433: 'SQL Server', 3306: 'MySQL', 3389: 'remote desktop', 5060: 'SIP', 5432: 'PostgreSQL', 5900: 'VNC', 6379: 'Redis', 8080: 'web',
}
/** How many of the busiest ports a row or the card names. */
export const ROUTER_PORTS_SHOWN = 3
export const ROUTER_FILTER_OPTION = { value: 'router', label: 'Router' }
export const ROUTER_CARD_TITLE = 'At the router'
export const ROUTER_CARD_LABEL = 'Refused today'
export const ROUTER_CARD_NOTE = 'Counted from the router\u2019s own log, which this program cannot check. These are attempts that reached your router from the internet side; if the router sits behind another device, the number says nothing about your public address. It is a figure to watch, never an alert.'
export const ROUTER_CARD_NO_PORTS = 'No port is named yet today.'
export const ROUTER_STAT_LABEL = 'Refused at the router'
export const ROUTER_STAT_CAPTION = 'attempts from the internet today'
export const ACTIVITY_LOAD_FAILED = 'The activity could not be read.'
export const ACTIVITY_EMPTY = 'Nothing has happened lately. New devices, changed ports, new findings and alerts show up here.'
export const ACTIVITY_EMPTY_FILTERED = 'Nothing of this kind has happened lately.'
export const ACTIVITY_SEE_ALL = 'See the timeline'
export const ACTIVITY_RELATED_TAG = 'Possibly related'
export const ACTIVITY_RELATED_NOTE = 'The timing is all the program can say; the two may be unrelated.'
export const TIMELINE_EXPLAIN_LABEL = 'Explain this week'
export const TIMELINE_EXPLAINING = 'Writing the summary'
export const TIMELINE_EXPLAIN_FAILED = 'The summary could not be written.'
export const TIMELINE_EXPLAIN_NOTHING = 'There is nothing from the last week to explain.'
export const TIMELINE_EXPLAIN_NOTE = 'Written by a model on this machine from the list of events only. It was not shown any device name or address, and it can be wrong.'
export const TIMELINE_EXPLAIN_HINT = 'Uses the audit workspace’s model, and only if that model runs on this machine.'

// Network · Settings · Ubuntu security data: Canonical's notices for this computer's release, which the audit compares the
// installed packages with.
export const UBUNTU_TITLE = 'Ubuntu security data'
export const UBUNTU_LOAD_FAILED = 'The Ubuntu security data could not be checked.'
export const UBUNTU_UPDATE_FAILED = 'The Ubuntu security data could not be updated.'
export const UBUNTU_CLEAR_FAILED = 'The Ubuntu security data could not be deleted.'
export const UBUNTU_UNSUPPORTED = 'This computer is not running Ubuntu, so there are no security notices to compare its packages with.'
export const UBUNTU_EMPTY = 'Not downloaded yet. With it, the audit can say which installed packages are behind a security fix.'
export const UBUNTU_SIZE = 'One download of about 1.3 MB from security-metadata.canonical.com, only when you press Update.'
export const UBUNTU_NOTE = 'Fixes that only an Ubuntu Pro subscription installs are not counted, so a package can be behind a fix this list does not show. A flaw that CISA lists as exploited makes the finding high.'
export const UBUNTU_ATTRIBUTION = 'Security notices © Canonical Ltd., from security-metadata.canonical.com. Canonical states no licence for this data; it is fetched once, kept on this computer and never sent anywhere.'
export const UBUNTU_OLD_DAYS = 14
export const UBUNTU_OLD_NOTE = 'This data is more than two weeks old, so newer fixes are missing. Update it.'

/** The Overview's attack paths card. */
export const ATTACK_PATHS_TITLE = 'Ways in that matter'
export const ATTACK_PATHS_NONE = 'No way in found that leads to this computer, your router or a device you marked as important.'
export const ATTACK_PATHS_MARK_HINT = 'A device you mark as important gets its own paths. Mark one on the Devices tab.'
export const ATTACK_PATHS_FIX_LABEL = 'The one fix'
export const DEVICE_MARK_IMPORTANT = 'Mark as important'
export const DEVICE_UNMARK_IMPORTANT = 'Not important'
export const DEVICE_IMPORTANT_NOTE = 'Marked as important.'

/** Network · Settings · Known-bad names. */
export const KNOWN_BAD_TITLE = 'Known-bad names'
export const KNOWN_BAD_LOAD_FAILED = 'The known-bad names could not be read.'
export const KNOWN_BAD_SAVE_FAILED = 'The address could not be saved.'
export const KNOWN_BAD_DOWNLOAD_FAILED = 'The list could not be downloaded.'
export const KNOWN_BAD_INTRO = 'Compares the names your devices ask for with abuse.ch\u2019s list of hosts that serve malware. The comparison happens on this computer; nothing a device asked for is sent anywhere. It needs a free abuse.ch account.'
export const KNOWN_BAD_LINK_LABEL = 'Get your list address at abuse.ch'
export const KNOWN_BAD_LINK_URL = 'https://urlhaus.abuse.ch/api/'
export const KNOWN_BAD_HOWTO = 'Sign in at abuse.ch, create an Auth-Key in your profile, then open the URLhaus API page while signed in. In the section "host file (domains only)" copy the address of the file. Paste it below. It contains your key, so it is kept encrypted and is never shown again.'
export const KNOWN_BAD_ADDRESS_LABEL = 'List address (from abuse.ch)'
export const KNOWN_BAD_ADDRESS_HINT = 'An https address on abuse.ch. Saving it asks for your passphrase.'
export const KNOWN_BAD_ADDRESS_SAVED = 'An address is saved.'
export const KNOWN_BAD_NO_ADDRESS = 'No address saved yet.'
export const knownBadLine = (count: number, since: string): string =>
  count > 0 ? `${count.toLocaleString('en-US')} malware host names held, downloaded ${since}.` : 'No list downloaded yet.'
export const KNOWN_BAD_SWITCH_NOTE = 'The comparison is off until you switch it on.'

// Settings · Network · Wi-Fi look-alikes.
export const WIFI_TITLE = 'Wi-Fi look-alikes'
export const WIFI_LOAD_FAILED = 'The Wi-Fi look-alikes setting could not be read.'
export const WIFI_HINT = 'Tells you when an access point uses your network\u2019s name from an address you have not confirmed, which is how a fake copy of your Wi-Fi catches devices.'
export const WIFI_DETAILS = 'It reads the list of networks this computer\u2019s own NetworkManager already hears, in a sandbox that cannot use the network, and never scans or connects. Only your own network\u2019s name and addresses are kept; other people\u2019s networks are not.'
export const WIFI_STEPS = [
  'Type your Wi-Fi network\u2019s name exactly as it appears on your devices, and add it.',
  'Press Look now. Every access point that uses the name is listed with its address.',
  'Tick the ones that are yours (the router, a mesh node, each band) and confirm them. From then on any other address using the name is reported.',
]
export const WIFI_NAME_LABEL = 'Network name'
export const WIFI_ADD = 'Add'
export const WIFI_LOOK = 'Look now'
export const WIFI_CONFIRM = 'These are mine'
export const WIFI_REMOVE = 'Stop watching'
export const WIFI_NONE_SEEN = 'No access point with this name was heard in the last look.'
export const WIFI_NOT_LOOKED = 'No look has been made yet.'
export const WIFI_UNCONFIRMED_NOTE = 'None of its access points is confirmed yet, so nothing is reported for this name.'
export const wifiSeenLine = (signal: number, security: string): string => `${signal}% signal \u00b7 ${security === 'open' ? 'no security' : security.toUpperCase()}`

