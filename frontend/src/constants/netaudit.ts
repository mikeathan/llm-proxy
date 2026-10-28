import type { ChoiceOption } from '../types/ui'
import type { HelpSection, NetauditHelpTopic, NetauditViewMode, RequirementCode, RequirementCopy } from '../types/netaudit'

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

export const HELP_TOPIC_LABELS: Record<NetauditHelpTopic, string> = {
  basics: 'The basics',
  setup: 'Setting it up',
  reports: 'Reading a report',
  vulns: 'Known vulnerabilities',
  fixes: 'Fixing a finding',
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
      'Checks look for common weak settings on the services found: SMBv1, anonymous FTP, Telnet, a VNC, Redis, MongoDB, Memcached or MQTT broker with no password, an open Docker control port, expired or weak certificates, weak SSH settings and a few more. They only read; nothing tries a password and nothing changes on a device.',
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
  safety: {
    title: HELP_TOPIC_LABELS.safety,
    paragraphs: [
      'Only addresses inside your home or office network can be scanned. Internet addresses are always refused, whatever you type.',
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
export const VULN_DOWNLOAD_SIZE = 'Downloading fetches more than 100 MB, only when you press the button.'
