import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { StorageReport } from '../../../types/storage'

const service = vi.hoisted(() => ({ storage: vi.fn() }))
vi.mock('../../../services/admin/adminService', () => ({ AdminApiService: service }))

import StorageReportCard from '../../../components/settings/StorageReportCard.vue'

const report = (over: Partial<StorageReport> = {}): StorageReport => ({
  root: '/data', total: 3 * 1024 * 1024, free: 40 * 1024 ** 3, size: 100 * 1024 ** 3, level: 'ok',
  rows: [
    { id: 'runs', title: 'Run history and recordings', owner: 'core', entries: ['runs'], bytes: 2 * 1024 * 1024, described: true, clean: 'Clear runtime data (Settings → Security).' },
    { id: 'stack:radio:radio.db', title: 'Radio stack: radio.db', owner: 'radio', entries: ['radio.db'], bytes: 1024 * 1024, described: true, cap_bytes: 64 * 1024 * 1024, records: 12 },
    { id: 'other:mystery.bin', title: 'mystery.bin', owner: '', entries: ['mystery.bin'], bytes: 10, described: false },
  ],
  workspaces: { id: 'workspaces', title: 'Workspace folders (your own work: size only)', owner: 'people', entries: ['/ws'], bytes: 2048, described: true },
  ...over,
})

describe('StorageReportCard', () => {
  beforeEach(() => service.storage.mockReset())

  it('lists every store with its size, a stack file with its cap, and what it does not recognise', async () => {
    service.storage.mockResolvedValue(report())
    const w = mount(StorageReportCard)
    await flushPromises()
    expect(w.get('[data-test="storage-total"]').text()).toBe('3 MB in the data folder · 40 GB free of 100 GB')
    expect(w.get('[data-test="storage-row-runs"]').text()).toContain('Clear runtime data')
    expect(w.get('[data-test="storage-row-stack:radio:radio.db"]').text()).toContain('at most 64 MB · 12 records')
    expect(w.get('[data-test="storage-row-other:mystery.bin"]').find('[data-test="storage-unknown"]').exists()).toBe(true)
    expect(w.get('[data-test="storage-row-workspaces"]').find('[data-test="storage-clean"]').exists()).toBe(false)
  })

  it('says when the disk is getting full', async () => {
    service.storage.mockResolvedValue(report({ level: 'critical' }))
    const w = mount(StorageReportCard)
    await flushPromises()
    expect(w.get('[data-test="storage-level"]').text()).toContain('almost full')
  })
})
