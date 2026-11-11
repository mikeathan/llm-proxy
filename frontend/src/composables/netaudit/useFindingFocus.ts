import { inject, nextTick, onScopeDispose, ref, watch, type Ref } from 'vue'
import { routeLocationKey } from 'vue-router'
import { motionScroll } from '../../utils/motion'

/** How long a finding stays marked after a link brought the person to it. */
const FLASH_MS = 3000
const FLASH_ATTRIBUTE = 'data-flash'
const KEY_ATTRIBUTE = 'data-key'
const DEVICE_PREFIX_END = '|'

const text = (value: unknown): string => (typeof value === 'string' ? value : '')

/**
 * Brings the person to the finding a link named: the route's `finding` (one key) or `device` (every finding of an address). Once the report is
 * drawn, the first match is scrolled to the middle and all matches are marked for a moment. `missing` is true when the report has no such
 * finding any more (it was fixed, or marked as fine and folded away), so the screen can say so instead of showing nothing happening.
 */
export function useFindingFocus(root: Ref<HTMLElement | null>, ready: Ref<boolean>) {
  // The route is read without the warning `useRoute` gives where no router is installed (a card mounted alone): there is then no link to follow.
  const route = inject(routeLocationKey, null)
  const missing = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let marked: HTMLElement[] = []

  const unmark = () => {
    clearTimeout(timer)
    for (const el of marked) el.removeAttribute(FLASH_ATTRIBUTE)
    marked = []
  }

  async function focus(): Promise<void> {
    const finding = text(route?.query.finding)
    const device = text(route?.query.device)
    unmark()
    missing.value = false
    if (!ready.value || (!finding && !device)) return
    await nextTick()
    const rows = [...(root.value?.querySelectorAll<HTMLElement>(`[${KEY_ATTRIBUTE}]`) ?? [])]
    const found = rows.filter((el) => {
      const key = el.getAttribute(KEY_ATTRIBUTE) ?? ''
      return finding ? key === finding : key.startsWith(device + DEVICE_PREFIX_END)
    })
    missing.value = found.length === 0
    if (!found.length) return
    found[0]!.scrollIntoView?.({ block: 'center', behavior: motionScroll('smooth') })
    for (const el of found) el.setAttribute(FLASH_ATTRIBUTE, '')
    marked = found
    timer = setTimeout(unmark, FLASH_MS)
  }

  watch([() => route?.query.finding, () => route?.query.device, ready], focus, { immediate: true, flush: 'post' })
  onScopeDispose(unmark)
  return { missing }
}
