import { computed, ref, toRaw } from 'vue'
import { DashboardService } from '../../services/dashboard/dashboardService'
import { useToast } from '../useToast'
import { usePersistedState } from '../ui/usePersistedState'
import { addBoard, addCard, applyPreset, boardsToOffer, isOnBoard, moveCard, pinToHome, reconcile, removeBoard, removeCard, renameBoard, resizeCard, shippedLayout } from '../../domain/dashboard'
import { HOME_BOARD_ID, LAYOUT_LOAD_FAILED, LAYOUT_SAVE_FAILED } from '../../constants/dashboard'
import { errorMessage } from '../../utils/errors'
import type { DashboardBoard, DashboardBoardTemplate, DashboardCols, DashboardLayout, DashboardPlacement, DashboardPreset, DashboardRows } from '../../types/dashboard'

const STORAGE_VERSION = 1

/**
 * The dashboard's state: the layout the server keeps for the owner, the board that is open in this browser, and the edits.
 * An edit shows at once and is saved after the one before it; if the save fails the layout goes back to what was last saved,
 * and a toast says so. `knownCards` are the cards the screen can draw, so a card it cannot draw never reaches the grid.
 */
export function useDashboard(knownCards: ReadonlySet<string>) {
  const toast = useToast()
  const layout = ref<DashboardLayout | null>(null)
  const loading = ref(true)
  const error = ref('')
  const arrange = ref(false)
  const activeId = usePersistedState<string>('dashboard-board', { version: STORAGE_VERSION, parse: (d) => (typeof d === 'string' ? d : null), fallback: HOME_BOARD_ID })

  let confirmed: DashboardLayout | null = null
  let queue: Promise<void> = Promise.resolve()

  const boards = computed<DashboardBoard[]>(() => layout.value?.boards ?? [])
  // A board removed in another browser must not leave this one on a page that does not exist: it falls back to Home.
  const board = computed<DashboardBoard | null>(() => boards.value.find((b) => b.id === activeId.value) ?? boards.value.find((b) => b.id === HOME_BOARD_ID) ?? null)
  const offeredBoards = computed<DashboardBoardTemplate[]>(() => boardsToOffer({ boards: boards.value }))

  /** Which preset the open board is in now, or 'custom' once a card has been moved or resized by hand. */
  const preset = computed<DashboardPreset | 'custom'>(() => {
    const current = board.value
    if (!current || !layout.value) return 'custom'
    const same = (a: DashboardPlacement[], b: DashboardPlacement[]) => a.length === b.length && a.every((c, i) => c.id === b[i]!.id && c.cols === b[i]!.cols && c.rows === b[i]!.rows)
    const presets: DashboardPreset[] = ['default', 'compact', 'two-columns', 'one-column']
    return presets.find((p) => same(current.cards, applyPreset(layout.value!, current.id, p).boards.find((b) => b.id === current.id)?.cards ?? [])) ?? 'custom'
  })

  async function load(): Promise<void> {
    loading.value = true
    try {
      const stored = await DashboardService.layout()
      const shown = reconcile({ boards: stored.boards ?? [] }, knownCards)
      layout.value = shown
      confirmed = shown
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LAYOUT_LOAD_FAILED)
      layout.value ??= reconcile(shippedLayout(), knownCards)
      confirmed ??= layout.value
    } finally {
      loading.value = false
    }
  }

  function change(next: DashboardLayout): void {
    if (!layout.value || next === toRaw(layout.value)) return
    layout.value = next
    queue = queue.then(async () => {
      try {
        await DashboardService.save(next)
        confirmed = next
      } catch {
        // Only the newest edit goes back: an older failure under a newer edit is replaced by that edit's own save.
        if (layout.value && toRaw(layout.value) === next && confirmed) layout.value = confirmed
        toast.error(LAYOUT_SAVE_FAILED)
      }
    })
  }

  const edit = (apply: (l: DashboardLayout, boardId: string) => DashboardLayout) => {
    const current = board.value
    if (layout.value && current) change(apply(layout.value, current.id))
  }

  const resize = (cardId: string, size: { cols?: DashboardCols; rows?: DashboardRows }) => edit((l, b) => resizeCard(l, b, cardId, size))
  const move = (from: number, to: number) => edit((l, b) => moveCard(l, b, from, to))
  const remove = (cardId: string) => edit((l, b) => removeCard(l, b, cardId))
  const add = (card: DashboardPlacement) => edit((l, b) => addCard(l, b, card))
  const pin = (cardId: string) => layout.value && change(pinToHome(layout.value, cardId))
  const pinned = (cardId: string): boolean => (layout.value ? isOnBoard(layout.value, HOME_BOARD_ID, cardId) : false)
  const setPreset = (p: DashboardPreset) => edit((l, b) => applyPreset(l, b, p))
  /** Puts the open board back to how it shipped. A board that did not ship has no shipped arrangement, so nothing changes. */
  const resetBoard = () => setPreset('default')

  function open(id: string): void {
    if (boards.value.some((b) => b.id === id)) activeId.value = id
  }
  function addBoardFrom(template?: DashboardBoardTemplate): void {
    if (!layout.value) return
    const out = addBoard(layout.value, template)
    if (out.id === null) return
    change(out.layout)
    activeId.value = out.id
  }
  function removeBoardById(id: string): void {
    if (!layout.value) return
    change(removeBoard(layout.value, id))
    if (activeId.value === id) activeId.value = HOME_BOARD_ID
  }
  const rename = (id: string, name: string) => layout.value && change(renameBoard(layout.value, id, name))

  /** Forgets the stored layout, so the shipped boards show again. */
  async function resetAll(): Promise<void> {
    try {
      await DashboardService.reset()
      await load()
      activeId.value = HOME_BOARD_ID
    } catch (err) {
      toast.error(errorMessage(err, LAYOUT_SAVE_FAILED))
    }
  }

  return { layout, boards, board, preset, offeredBoards, loading, error, arrange, activeId, load, open, resize, move, remove, add, pin, pinned, setPreset, resetBoard, addBoardFrom, removeBoardById, rename, resetAll }
}
