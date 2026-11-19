import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { useDashboard } from '../../../composables/dashboard/useDashboard'
import { DashboardService } from '../../../services/dashboard/dashboardService'
import { useToast } from '../../../composables/useToast'
import { BOARD_TEMPLATES, HOME_BOARD_ID } from '../../../constants/dashboard'
import { memoryStorage } from '../../helpers/memoryStorage'
import type { DashboardLayout } from '../../../types/dashboard'

vi.mock('../../../services/dashboard/dashboardService', () => ({
  DashboardService: { layout: vi.fn(), save: vi.fn(), reset: vi.fn() },
}))

const known = new Set(BOARD_TEMPLATES.flatMap((t) => t.cards.map((c) => c.id)))
const service = vi.mocked(DashboardService)

// Home ships minimal, so tests that arrange cards start from a Home a person has filled.
const filledHome: DashboardLayout = { boards: [{ id: HOME_BOARD_ID, name: 'Home', cards: [{ id: 'host-health', cols: 12, rows: 1 }, { id: 'running-now', cols: 4, rows: 1 }, { id: 'recent-runs', cols: 8, rows: 1 }] }] }

async function opened(stored: DashboardLayout = { boards: [] }) {
  service.layout.mockResolvedValue(stored)
  service.save.mockImplementation(async (l) => l)
  const d = useDashboard(known)
  await d.load()
  return d
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('localStorage', memoryStorage())
  useToast().toasts.value = []
})

describe('useDashboard', () => {
  it('shows the shipped boards while the server has nothing, and opens on Home', async () => {
    const d = await opened()
    expect(d.boards.value.map((b) => b.id)).toEqual(BOARD_TEMPLATES.map((t) => t.id))
    expect(d.board.value?.id).toBe(HOME_BOARD_ID)
    expect(d.preset.value).toBe('default')
    expect(service.save).not.toHaveBeenCalled()
  })

  it('still shows the shipped boards, and says why, when the layout cannot be read', async () => {
    service.layout.mockRejectedValue(new Error('down'))
    const d = useDashboard(known)
    await d.load()
    expect(d.error.value).toBe('down')
    expect(d.boards.value.length).toBeGreaterThan(0)
  })

  it('shows an edit at once and saves the whole layout', async () => {
    const d = await opened(filledHome)
    d.resize('host-health', { cols: 6 })
    expect(d.board.value?.cards[0]?.cols).toBe(6)
    expect(d.preset.value).toBe('custom')
    await flushPromises()
    expect(service.save).toHaveBeenCalledTimes(1)
    expect(service.save.mock.calls[0]![0].boards[0]!.cards[0]!.cols).toBe(6)
  })

  it('puts the layout back and says so when a save fails', async () => {
    const d = await opened(filledHome)
    service.save.mockRejectedValue(new Error('no'))
    d.remove('running-now')
    expect(d.board.value?.cards.some((c) => c.id === 'running-now')).toBe(false)
    await flushPromises()
    expect(d.board.value?.cards.some((c) => c.id === 'running-now')).toBe(true)
    expect(useToast().toasts.value.map((t) => t.type)).toContain('error')
  })

  it('applies a preset to the open board and reports it', async () => {
    const d = await opened(filledHome)
    d.setPreset('one-column')
    expect(d.board.value?.cards.every((c) => c.cols === 12 && c.rows === 1)).toBe(true)
    expect(d.preset.value).toBe('one-column')
    d.resetBoard()
    expect(d.preset.value).toBe('default')
  })

  it('pins a card from a stack board to Home', async () => {
    const d = await opened()
    d.open('radio')
    expect(d.pinned('radio-findings')).toBe(false)
    d.pin('radio-findings')
    expect(d.pinned('radio-findings')).toBe(true)
    await flushPromises()
    expect(service.save).toHaveBeenCalled()
  })

  it('adds a board, opens it, and falls back to Home when the open board is removed', async () => {
    const d = await opened()
    d.addBoardFrom()
    expect(d.board.value?.name).toBe('New board')
    const id = d.activeId.value
    d.removeBoardById(id)
    expect(d.board.value?.id).toBe(HOME_BOARD_ID)
  })

  it('opens Home when the remembered board no longer exists', async () => {
    vi.stubGlobal('localStorage', memoryStorage({ 'admin-ui:dashboard-board': JSON.stringify({ version: 1, data: 'gone' }) }))
    const d = await opened()
    expect(d.board.value?.id).toBe(HOME_BOARD_ID)
  })

  it('forgets the stored layout on request', async () => {
    const d = await opened()
    service.reset.mockResolvedValue({ boards: [] })
    service.layout.mockResolvedValue({ boards: [] })
    d.removeBoardById('radio')
    await d.resetAll()
    expect(service.reset).toHaveBeenCalled()
    expect(d.boards.value.some((b) => b.id === 'radio')).toBe(true)
  })
})
