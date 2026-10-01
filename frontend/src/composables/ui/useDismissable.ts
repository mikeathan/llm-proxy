import { onUnmounted, type Ref } from 'vue'

/**
 * Closes an open popover on a pointer press outside `root` or on Escape.
 * Listeners live on the document for the owner's lifetime and do nothing
 * while `open` is false.
 */
export function useDismissable(root: Ref<HTMLElement | null>, open: Ref<boolean>) {
  function onPointerDown(event: PointerEvent) {
    if (!open.value) return
    if (root.value && !root.value.contains(event.target as Node)) open.value = false
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
