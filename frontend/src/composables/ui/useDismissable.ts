import { onUnmounted, type Ref } from 'vue'

/**
 * Closes an open popover on a pointer press outside `root` (and any `also` element, for a panel
 * teleported out of it) or on Escape.
 * Listeners live on the document for the owner's lifetime and do nothing
 * while `open` is false.
 */
export function useDismissable(root: Ref<HTMLElement | null>, open: Ref<boolean>, also?: Ref<HTMLElement | null>) {
  function onPointerDown(event: PointerEvent) {
    if (!open.value) return
    const target = event.target as Node
    if (also?.value?.contains(target)) return
    if (root.value && !root.value.contains(target)) open.value = false
  }

  function onKeydown(event: KeyboardEvent) {
    if (open.value && event.key === 'Escape') open.value = false
  }

  document.addEventListener('pointerdown', onPointerDown)
  document.addEventListener('keydown', onKeydown)
  onUnmounted(() => {
    document.removeEventListener('pointerdown', onPointerDown)
    document.removeEventListener('keydown', onKeydown)
  })
}
