import { onMounted, onUnmounted, type Ref } from 'vue'

const TYPING = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

/**
 * Reading with the keyboard: j and k move to the next and previous story inside `root`, o opens the story in focus. Keys typed in a field, or with a modifier,
 * are left alone. A story is any element marked data-story; its first link takes the focus (a headline-only story without one is skipped).
 */
export function useStoryKeys(root: Ref<HTMLElement | null>) {
  function onKey(e: KeyboardEvent) {
    if (e.metaKey || e.ctrlKey || e.altKey || !root.value) return
    const target = e.target as HTMLElement | null
    if (target && (TYPING.has(target.tagName) || target.isContentEditable)) return
    if (e.key !== 'j' && e.key !== 'k' && e.key !== 'o') return
    const links = [...root.value.querySelectorAll<HTMLAnchorElement>('[data-story] a[href]')]
    if (!links.length) return
    const at = links.findIndex((a) => a === document.activeElement)
    if (e.key === 'o') {
      if (at >= 0) links[at]?.click()
      return
    }
    const next = e.key === 'j' ? Math.min(at + 1, links.length - 1) : Math.max(at - 1, 0)
    e.preventDefault()
    links[next]?.focus()
    links[next]?.closest('[data-story]')?.scrollIntoView({ block: 'nearest' })
  }
  onMounted(() => document.addEventListener('keydown', onKey))
  onUnmounted(() => document.removeEventListener('keydown', onKey))
}
