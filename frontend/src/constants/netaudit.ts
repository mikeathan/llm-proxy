import type { ChoiceOption } from '../types/ui'
import type { DeviceAction, HelpSection, IPv6DiscoveryProblem, NetauditHelpTopic, NetauditViewMode, RequirementCode, RequirementCopy } from '../types/netaudit'

// Network scan screens (plan network-audit, F-A): wording and fixed choices. The server names what is missing; the
// words around it live here.

export const NETAUDIT_VIEW_KEY = 'netaudit-view'
export const NETAUDIT_VIEW_VERSION = 1
export const DEFAULT_VIEW_MODE: NetauditViewMode = 'simple'

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
      'The list comes from the U.S. National Vulnerability Database (NVD) and, to rank what matters first, CISA\'s Known Exploited Vulnerabilities catalog, which names flaws attackers are already using. Download it from Settings → Network scan. More than 100 MB is fetched, only when you press the button; the scan itself never uses the Internet.',
      'It can only look at what the scan recognised. Many routers, cameras and phones do not say which program or firmware they run, so they are not covered, and the report says how many services were looked at. Silence from this check does not mean a device is safe.',
      'Without the list, audits still run and say that the check was skipped. You can turn the check off for one audit in its settings.',
      'If this computer cannot reach the Internet, fetch the two files on another computer and pick them under "Import a file instead" (Advanced, or after a download stops). Importing asks for your passphrase again, because it replaces what every audit compares with.',
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
      'IPv6 addresses are shown beside a device when this computer knows them. It only lists devices it has already talked to, so it finds few that use IPv6 alone; those are listed and you are told about them, but they have no finding of their own.',
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

/**
 * How a finding's severity looks. A check stands out from the plain open ports around it: a thick coloured edge and a
 * tint (red for critical and high, amber for medium and low); open ports stay plain.
 */
export const SEVERITY_CARD_CLASS: Record<string, string> = {
  critical: 'border-state-error bg-state-error/[0.10]',
  high: 'border-state-error bg-state-error/[0.10]',
  medium: 'border-state-running bg-state-running/[0.10]',
  low: 'border-state-running bg-state-running/[0.10]',
}
export const PLAIN_CARD_CLASS = 'border-hairline bg-surface'

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

/** Settings · Network scan · Devices: the words the card uses. */
export const DEVICES_TITLE = 'Devices on your network'
export const DEVICES_LOAD_FAILED = 'The list of devices could not be read.'
export const DEVICES_ACTION_FAILED: Record<DeviceAction, string> = {
  name: 'The name could not be saved.',
  approve: 'The device could not be approved.',
  forget: 'The device could not be removed from the list.',
  'confirm-gateway': 'The router\'s address could not be confirmed.',
}
export const PRESENCE_LOAD_FAILED = 'The new-device check could not be read.'
export const PRESENCE_SAVE_FAILED = 'The new-device check could not be changed.'
export const PRESENCE_HINT = 'Looks for a device that was not on the network before, and sends you a short message the first time one shows up. It only connects to the networks allowed above; it never looks at what a device sends.'
export const IPV6_LOAD_FAILED = 'The IPv6 setting could not be read.'
export const IPV6_SAVE_FAILED = 'The IPv6 setting could not be changed.'
export const IPV6_LABEL = 'Look for IPv6 devices'
export const IPV6_HINT = 'Some devices only show up over IPv6. Each look sends one small message to the devices on your network asking them to announce themselves, and lists the ones that answer. It does not scan or connect to anything.'
export const IPV6_FIX_COMMAND = 'sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"'
export const IPV6_FIX_KEEP_COMMAND = 'echo \'net.ipv4.ping_group_range = 0 2147483647\' | sudo tee /etc/sysctl.d/90-ping-group.conf'
export const IPV6_PROBLEMS: Record<IPv6DiscoveryProblem, string> = {
  not_permitted: 'This computer does not let the program send that message (the ping_group_range setting). Run this on the computer, then look again. It also lets other accounts on it send pings.',
  unavailable: 'Only a Linux computer can look for IPv6 devices.',
  failed: 'The message could not be sent. Check the server log, then look again.',
}
export const IPV6_FIX_KEEP_NOTE = 'To keep it after a restart:'
export const DEVICE_NAME_MAX = 40
export const DEVICE_UNNAMED = 'Unknown device'
export const DEVICE_PRIVATE = 'Phone or laptop (private address)'
export const DEVICES_EMPTY = 'No devices seen yet. Turn on the check above, or run a network audit.'
export const DEVICES_BASELINE_NOTE = 'The first look is taken as the normal state: every device found then counts as approved, and only later arrivals are reported.'
export const GATEWAY_CHANGED_TITLE = 'The router\'s hardware address changed'
export const GATEWAY_CHANGED_BODY = 'Something else may be answering for the router, which would let it read or change the network\'s traffic. A replaced or reset router does this too. If you know why, confirm it.'
export const FORGET_CONFIRM = {
  title: 'Remove this device from the list?',
  message: 'It will come back as a new device if it is still on the network.',
  confirmText: 'Remove',
} as const

/** Settings · Network scan · Decoy services: the words the card uses. */
export const DECOYS_TITLE = 'Decoy services'
export const DECOYS_LOAD_FAILED = 'The decoy services could not be read.'
export const DECOYS_SAVE_FAILED = 'The decoy services could not be changed.'
export const DECOYS_HINT = 'Fake services on a few unused ports that nothing on your network should ever use. A device that connects to one is probing for services, which is what an intruder or malware does first. They only note who connected; they read a few bytes, throw them away and run nothing. Off until you turn them on, and turning them on asks for your passphrase.'
export const DECOYS_FIREWALL = 'If this computer has a firewall, the ports must be allowed in, or connections never arrive.'
export const DECOYS_NONE = 'Nothing has connected to a decoy.'
export const DECOYS_SCANNER_NOTE = 'A scanner app on your own phone or laptop will also touch them; accept that finding.'
