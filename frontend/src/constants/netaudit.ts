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
      'Checks look for common weak settings on the services found: SMBv1, anonymous FTP, a VNC or Redis with no password, expired certificates and a few more. They only read; nothing tries a password and nothing changes on a device.',
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
      'Each line is a device and a service that answered. "New" means the last scan did not show it before. Something marked "Gone" was seen earlier and has disappeared.',
      '"This is fine" hides a finding you expect, such as your own file server, for 30 days, 90 days or a year. It comes back if the service changes, and you can take it back at any time.',
      'A scan can run out of time on a slow device, and is tried a second time before it gives up. Such a device is reported as not scanned to completion; that is not the same as having no open doors. In Advanced settings, "Time allowed per device" gives slow devices longer.',
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
  no_sandbox: { title: "This computer can't fence the scan in", simple: 'Scans will not run here unless you allow them without that protection.', help: 'safety' },
}

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
