import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ScanScope } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ readiness: vi.fn(), scope: vi.fn(), updateScope: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const confirmMock = vi.hoisted(() => vi.fn())
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

import { useScanScope } from '../../../composables/netaudit/useScanScope'

const lan = { interface: 'en0', cidr: '192.168.1.0/24', devices: 254 }
const empty: ScanScope = { ranges: [], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [lan] }
const withLan: ScanScope = { ranges: [{ cidr: lan.cidr, devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [lan] }

describe('useScanScope', () => {
  beforeEach(() => {
    for (const fn of [...Object.values(service), confirmMock]) fn.mockReset()
    service.readiness.mockResolvedValue({ ready: true, requirements: [] })
  })

  it('"Use my network" adds the first network not yet allowed, keeping the ranges already there', async () => {
    service.scope.mockResolvedValue(empty)
    service.updateScope.mockResolvedValue(withLan)
    const scope = useScanScope()
    await scope.load()
    expect(scope.unusedSuggestions.value).toHaveLength(1)
    await scope.useMyNetwork()
    expect(service.updateScope).toHaveBeenCalledWith({ entries: ['192.168.1.0/24'] })
    expect(scope.ranges.value).toHaveLength(1)
    expect(scope.unusedSuggestions.value).toHaveLength(0)
    expect(await scope.useMyNetwork()).toBe(false)
  })

  it('removes a range by replacing the list without it', async () => {
    service.scope.mockResolvedValue(withLan)
    service.updateScope.mockResolvedValue(empty)
    const scope = useScanScope()
    await scope.load()
    await scope.removeRange(lan.cidr)
    expect(service.updateScope).toHaveBeenCalledWith({ entries: [] })
  })

  it('shows the server\'s refusal of a bad entry and keeps what was saved', async () => {
    service.scope.mockResolvedValue(withLan)
    service.updateScope.mockRejectedValue(new Error('8.8.8.8 is a public address'))
    const scope = useScanScope()
    await scope.load()
    expect(await scope.addRange('8.8.8.8')).toBe(false)
    expect(scope.saveError.value).toContain('public address')
    expect(scope.ranges.value).toHaveLength(1)
  })

  it('asks before allowing unconfined scans, and saves nothing when declined', async () => {
    service.scope.mockResolvedValue(withLan)
    service.updateScope.mockResolvedValue({ ...withLan, allow_unconfined: true })
    const scope = useScanScope()
    await scope.load()
    confirmMock.mockResolvedValue(false)
    expect(await scope.setAllowUnconfined(true)).toBe(false)
    expect(service.updateScope).not.toHaveBeenCalled()
    confirmMock.mockResolvedValue(true)
    expect(await scope.setAllowUnconfined(true)).toBe(true)
    expect(service.updateScope).toHaveBeenCalledWith({ allow_unconfined: true })
    expect(await scope.setAllowUnconfined(false)).toBe(true)
  })

  it('reports a failed load', async () => {
    service.scope.mockRejectedValue(new Error('down'))
    const scope = useScanScope()
    await scope.load()
    expect(scope.loadError.value).toBe('down')
  })

  it('saves the time allowed per device on its own, leaving the ranges alone', async () => {
    service.scope.mockResolvedValue(withLan)
    service.updateScope.mockResolvedValue({ ...withLan, device_timeout_minutes: 7 })
    const scope = useScanScope()
    await scope.load()
    await scope.setDeviceTimeout(7)
    expect(service.updateScope).toHaveBeenCalledWith({ device_timeout_minutes: 7 })
    expect(scope.scope.value?.device_timeout_minutes).toBe(7)
    expect(scope.ranges.value).toHaveLength(1)
  })
})
