import { describe, it, expect, vi, beforeEach } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'

const { getJournalMock, clearJournalMock } = vi.hoisted(() => ({
  getJournalMock: vi.fn(),
  clearJournalMock: vi.fn(),
}))

vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getAutomationJournal: getJournalMock, clearAutomationJournal: clearJournalMock },
}))

import { useAutomationJournal } from '../../../composables/automation/useAutomationJournal'

function setup(enabled = true) {
  const on = ref(enabled)
  const name = ref('nightly')
  const scope = effectScope()
  const journal = scope.run(() => useAutomationJournal(() => 'ws', () => name.value, () => on.value))!
  return { journal, on, name, scope }
}

describe('useAutomationJournal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getJournalMock.mockResolvedValue({ journal: '- query A' })
    clearJournalMock.mockResolvedValue({ status: 'cleared' })
  })

  it('loads the journal of an automation that keeps one', async () => {
    const { journal, scope } = setup()
    await flushPromises()
    expect(getJournalMock).toHaveBeenCalledWith('ws', 'nightly')
    expect(journal.journal.value).toBe('- query A')
    expect(journal.error.value).toBe('')
    scope.stop()
  })

  it('does not fetch while the journal is off, and loads once it is switched on', async () => {
    const { journal, on, scope } = setup(false)
    await flushPromises()
    expect(getJournalMock).not.toHaveBeenCalled()
    on.value = true
    await nextTick()
    await flushPromises()
    expect(journal.journal.value).toBe('- query A')
    scope.stop()
  })

  it('reloads when another automation is shown', async () => {
    const { name, scope } = setup()
    await flushPromises()
    getJournalMock.mockResolvedValue({ journal: 'other' })
    name.value = 'weekly'
    await nextTick()
    await flushPromises()
    expect(getJournalMock).toHaveBeenLastCalledWith('ws', 'weekly')
    scope.stop()
  })

  it('clear empties the journal through the service', async () => {
    const { journal, scope } = setup()
    await flushPromises()
    await journal.clear()
    expect(clearJournalMock).toHaveBeenCalledWith('ws', 'nightly')
    expect(journal.journal.value).toBe('')
    scope.stop()
  })

  it('keeps the text and explains a failed clear', async () => {
    clearJournalMock.mockRejectedValue(new Error('disk full'))
    const { journal, scope } = setup()
    await flushPromises()
    await journal.clear()
    expect(journal.journal.value).toBe('- query A')
    expect(journal.error.value).toMatch(/Could not clear the journal/)
    scope.stop()
  })

  it('explains a failed load without throwing', async () => {
    getJournalMock.mockRejectedValue(new Error('boom'))
    const { journal, scope } = setup()
    await flushPromises()
    expect(journal.error.value).toMatch(/Could not load the journal/)
    scope.stop()
  })
})
