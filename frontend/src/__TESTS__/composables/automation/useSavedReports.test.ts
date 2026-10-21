import { describe, it, expect, vi, beforeEach } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { SavedReport } from '../../../types/dispatcher'

const { listMock, readMock } = vi.hoisted(() => ({ listMock: vi.fn(), readMock: vi.fn() }))

vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getAutomationReports: listMock, readWorkspaceFile: readMock },
}))

import { useSavedReports } from '../../../composables/automation/useSavedReports'

const report = (id: string, hour: number): SavedReport => ({
  path: `reports/nightly/2026102${hour}T070000Z-run_${id}.md`, run_id: `run_${id}`, started_at: `2026-10-2${hour}T07:00:00Z`, size_bytes: 100 * hour,
})
const NEWEST = report('2', 2)
const OLDEST = report('1', 1)

function setup() {
  const name = ref('nightly')
  const refresh = ref('')
  const scope = effectScope()
  const reports = scope.run(() => useSavedReports(() => 'ws', () => name.value, () => refresh.value))!
  return { reports, name, refresh, scope }
}

describe('useSavedReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listMock.mockResolvedValue({ reports: [NEWEST, OLDEST], truncated: false })
    readMock.mockImplementation(async (_ws: string, file: string) => `text of ${file}`)
  })

  it('loads the automation\'s reports, newest first, and shows none until one is opened', async () => {
    const { reports, scope } = setup()
    await flushPromises()
    expect(listMock).toHaveBeenCalledWith('ws', 'nightly')
    expect(reports.reports.value).toEqual([NEWEST, OLDEST])
    expect(reports.truncated.value).toBe(false)
    expect(reports.selected.value).toBeNull()
    expect(readMock).not.toHaveBeenCalled()
    scope.stop()
  })

  it('opens a report by reading its file, and closes it again', async () => {
    const { reports, scope } = setup()
    await flushPromises()
    await reports.open(OLDEST)
    expect(readMock).toHaveBeenCalledWith('ws', OLDEST.path)
    expect(reports.selected.value).toEqual(OLDEST)
    expect(reports.content.value).toBe(`text of ${OLDEST.path}`)
    reports.close()
    expect(reports.selected.value).toBeNull()
    expect(reports.content.value).toBe('')
    scope.stop()
  })

  it('keeps the report that was opened last when an earlier read finishes late', async () => {
    let finishFirst!: (text: string) => void
    readMock.mockImplementationOnce(() => new Promise<string>((resolve) => { finishFirst = resolve }))
    const { reports, scope } = setup()
    await flushPromises()
    const first = reports.open(OLDEST)
    await reports.open(NEWEST)
    finishFirst('late text of the first')
    await first
    expect(reports.selected.value).toEqual(NEWEST)
    expect(reports.content.value).toBe(`text of ${NEWEST.path}`)
    scope.stop()
  })

  it('says so when the list or a report cannot be loaded', async () => {
    listMock.mockRejectedValueOnce(new Error('down'))
    const { reports, scope } = setup()
    await flushPromises()
    expect(reports.error.value).toBe('Could not load the saved reports. Try again in a moment.')
    expect(reports.reports.value).toEqual([])

    await reports.reload()
    expect(reports.error.value).toBe('')
    readMock.mockRejectedValueOnce(new Error('gone'))
    await reports.open(NEWEST)
    expect(reports.error.value).toBe('Could not open that report. It may have been deleted.')
    expect(reports.content.value).toBe('')
    scope.stop()
  })

  it('reloads for another automation, and when the latest report changes, and drops an open report', async () => {
    const { reports, name, refresh, scope } = setup()
    await flushPromises()
    await reports.open(NEWEST)

    refresh.value = NEWEST.path
    await nextTick()
    await flushPromises()
    expect(listMock).toHaveBeenCalledTimes(2)
    expect(reports.selected.value).toEqual(NEWEST) // a new run does not close what is being read

    name.value = 'other'
    await nextTick()
    await flushPromises()
    expect(listMock).toHaveBeenLastCalledWith('ws', 'other')
    expect(reports.selected.value).toBeNull()
    scope.stop()
  })

  it('reports that older reports were left out', async () => {
    listMock.mockResolvedValueOnce({ reports: [NEWEST], truncated: true })
    const { reports, scope } = setup()
    await flushPromises()
    expect(reports.truncated.value).toBe(true)
    scope.stop()
  })
})
