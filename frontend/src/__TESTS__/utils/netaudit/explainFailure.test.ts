import { describe, expect, it } from 'vitest'
import { explainFailure } from '../../../utils/netaudit/explainFailure'

describe('explainFailure', () => {
  it('explains the failure that sent a model to scan an address that is not allowed', () => {
    const e = explainFailure('agent execution failed: tool unavailable: operator action required: 192.168.1.0/24: target is outside the declared scan ranges: 192.168.1.0/24: target is outside the declared scan ranges')
    expect(e?.title).toBe("That network isn't allowed for scanning")
    expect(e?.fix).toBe('network-settings')
    expect(e?.showsAllowedNetworks).toBe(true)
  })

  it.each([
    ['scanning is off: no scan range has been declared', 'Scanning is switched off'],
    ['nmap is required for this scan', 'nmap is not installed'],
    ['no OS file confinement on this host; guarded programs do not run unconfined unless the operator allows it', "This computer can't fence the scan in"],
    ['another scan is already running', 'Another scan was running'],
    ['this run may only use a local model', 'This audit needs a model on this computer'],
    ['The model "gpt" runs in the cloud, and a network audit may only use a model on this machine. Choose a local model.', 'This audit needs a model on this computer'],
  ])('knows "%s"', (text, title) => {
    expect(explainFailure(text)?.title).toBe(title)
  })

  it('leaves an unknown failure to be shown as it came', () => {
    expect(explainFailure('the model timed out')).toBeNull()
    expect(explainFailure('')).toBeNull()
    expect(explainFailure(undefined)).toBeNull()
  })
})
