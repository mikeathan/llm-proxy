import type { FailureExplanation } from '../../types/netaudit'

// A failed network audit says why in the backend's own words ("tool unavailable: operator action required: ...").
// This puts the common ones in plain language with the one thing to do. The matched phrases are the backend's
// error texts (scanscope, netaudit, progexec, the audit pre-flight); anything unmatched is shown as it came.
const KNOWN: { match: string; explanation: FailureExplanation }[] = [
  {
    match: 'outside the declared scan ranges',
    explanation: {
      title: "That network isn't allowed for scanning",
      body: 'The task asked to scan an address outside the networks you allowed.',
      fix: 'network-settings',
      showsAllowedNetworks: true,
    },
  },
  {
    match: 'scanning is off',
    explanation: { title: 'Scanning is switched off', body: 'No network has been allowed for scanning yet.', fix: 'network-settings' },
  },
  {
    match: 'nmap is required',
    explanation: { title: 'nmap is not installed', body: 'This scan needs the free nmap program on the computer that runs the app.', fix: 'network-settings' },
  },
  {
    match: 'no OS file confinement',
    explanation: {
      title: "This computer can't fence the scan in",
      body: 'Scans will not run here unless you allow them without that protection.',
      fix: 'network-settings',
    },
  },
  {
    match: 'target names too many addresses',
    explanation: { title: 'That is more addresses than one scan allows', body: 'Choose a smaller range for the audit.', fix: 'edit-automation' },
  },
  {
    match: 'another scan is already running',
    explanation: { title: 'Another scan was running', body: 'Only one scan runs at a time. Try again in a minute.', fix: 'none' },
  },
  {
    match: 'a network audit may only use a model on this machine',
    explanation: { title: 'This audit needs a model on this computer', body: 'Audits never send your devices to a cloud model. Choose a local model for the automation.', fix: 'edit-automation' },
  },
  {
    match: 'may only use a local model',
    explanation: { title: 'This audit needs a model on this computer', body: 'Audits never send your devices to a cloud model. Choose a local model for the automation.', fix: 'edit-automation' },
  },
]

/** The plain account of a failed run, or null when it is not one this knows. */
export function explainFailure(error: string | undefined | null): FailureExplanation | null {
  if (!error) return null
  return KNOWN.find((k) => error.includes(k.match))?.explanation ?? null
}
