import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ findings: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useDeviceProblems } from '../../../composables/netaudit/useDeviceProblems'

const finding = (device: string, check: string, severity: string, accepted = false) => ({ device, check, severity, accepted, key: `${device}|tcp|0|${check}`, port: 0 })
const audit = { workspaceId: 'ws', automation: 'net' }

describe('useDeviceProblems', () => {
  beforeEach(() => service.findings.mockReset())

  it('counts the open problems of each device and keeps the worst severity', async () => {
    service.findings.mockResolvedValue({ findings: [finding('192.0.2.1', 'a', 'low'), finding('192.0.2.1', 'b', 'high'), finding('192.0.2.2', 'c', 'medium')] })
    const p = useDeviceProblems(() => audit)
    await p.load()
    expect(p.of('192.0.2.1')).toEqual({ count: 2, severity: 'high' })
    expect(p.of('192.0.2.2')).toEqual({ count: 1, severity: 'medium' })
    expect(p.of('192.0.2.3')).toBeNull()
  })

  it('does not count an accepted problem or a bare open port', async () => {
    service.findings.mockResolvedValue({ findings: [finding('192.0.2.1', 'a', 'high', true), finding('192.0.2.1', 'open_port', 'info'), { device: '192.0.2.1', severity: 'info', accepted: false, key: 'k', port: 22 }] })
    const p = useDeviceProblems(() => audit)
    await p.load()
    expect(p.of('192.0.2.1')).toBeNull()
  })

  it('shows nothing, and asks for nothing, when there is no audit', async () => {
    const none = useDeviceProblems(() => null)
    await none.load()
    expect(service.findings).not.toHaveBeenCalled()
    expect(none.of('192.0.2.1')).toBeNull()
  })

  it('shows nothing when the report cannot be read', async () => {
    service.findings.mockRejectedValueOnce(new Error('down'))
    const p = useDeviceProblems(() => audit)
    await p.load()
    expect(p.of('192.0.2.1')).toBeNull()
  })

  it('forgets the old counts when the report cannot be read again', async () => {
    service.findings.mockResolvedValueOnce({ findings: [finding('192.0.2.1', 'a', 'low')] })
    const p = useDeviceProblems(() => audit)
    await p.load()
    service.findings.mockRejectedValueOnce(new Error('down'))
    await p.load()
    expect(p.of('192.0.2.1')).toBeNull()
  })
})
