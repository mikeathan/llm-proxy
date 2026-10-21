import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import type { SavedReport } from '../../../../types/dispatcher'

const { listMock, readMock } = vi.hoisted(() => ({ listMock: vi.fn(), readMock: vi.fn() }))
vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getAutomationReports: listMock, readWorkspaceFile: readMock },
}))

import SavedReportsPanel from '../../../../components/AgentIde/automation/SavedReportsPanel.vue'

const NEWEST: SavedReport = { path: 'reports/nightly/20261021T070000Z-run_2.md', run_id: 'run_2', started_at: '2026-10-21T07:00:00Z', size_bytes: 2048 }
const OLDEST: SavedReport = { path: 'reports/nightly/20261019T070000Z-run_1.md', run_id: 'run_1', started_at: '2026-10-19T07:00:00Z', size_bytes: 300 }
const STUBS = {
  Icon: true,
  MarkdownViewer: { props: ['content'], template: '<div class="md">{{ content }}</div>' },
}

const mounted: VueWrapper[] = []
async function mountPanel(props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  const w = mount(SavedReportsPanel, {
    props: { workspaceId: 'ws', automation: 'nightly', latest: NEWEST.path, ...props },
    global: { plugins: [router], stubs: STUBS },
    attachTo: document.body,
  })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('SavedReportsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listMock.mockResolvedValue({ reports: [NEWEST, OLDEST], truncated: false })
    readMock.mockImplementation(async (_ws: string, file: string) => `# Report\n${file}`)
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('lists the saved reports with their time, size and run', async () => {
    const w = await mountPanel()
    expect(w.text()).toContain('Saved reports')
    const rows = w.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('run_2')
    expect(rows[0]!.text()).toContain('2 KB')
    expect(rows[1]!.text()).toContain('run_1')
    expect(w.find('.md').exists()).toBe(false)
  })

  it('opens a report in place when its row is activated, with a link to the file and a way to close it', async () => {
    const w = await mountPanel()
    await w.findAll('tbody tr')[1]!.trigger('click')
    await flushPromises()
    expect(readMock).toHaveBeenCalledWith('ws', OLDEST.path)
    expect(w.get('.md').text()).toContain(OLDEST.path)
    const link = w.get('a[aria-label="Open saved report"]')
    expect(decodeURIComponent(link.attributes('href')!)).toBe(`/workspaces/ws/files/${OLDEST.path}`)
    const copy = w.findAllComponents({ name: 'CopyButton' }).find((c) => c.props('title') === 'Copy report')
    expect(copy?.props('text')).toContain(OLDEST.path)

    await w.get('button[aria-label="Close report"]').trigger('click')
    expect(w.find('.md').exists()).toBe(false)
  })

  it('says what this is when nothing has been saved yet', async () => {
    listMock.mockResolvedValue({ reports: [], truncated: false })
    const w = await mountPanel()
    expect(w.text()).toContain('No saved reports yet')
  })

  it('tells the reader when older reports were left out', async () => {
    listMock.mockResolvedValue({ reports: [NEWEST], truncated: true })
    const w = await mountPanel()
    expect(w.text()).toContain('Showing the newest reports only')
  })

  it('shows why the list could not be loaded', async () => {
    listMock.mockRejectedValue(new Error('down'))
    const w = await mountPanel()
    expect(w.text()).toContain('Could not load the saved reports')
  })

  it('shows why a report could not be opened', async () => {
    readMock.mockRejectedValue(new Error('gone'))
    const w = await mountPanel()
    await w.findAll('tbody tr')[0]!.trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('Could not open that report')
  })
})
