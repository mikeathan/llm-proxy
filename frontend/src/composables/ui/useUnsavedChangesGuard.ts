import { onMounted, onUnmounted, type Ref } from "vue"
import { onBeforeRouteLeave, onBeforeRouteUpdate, type RouteLocationNormalized } from "vue-router"
import { useConfirm } from "./useConfirm"

const LEAVE_PROMPT = {
  title: "Unsaved changes",
  message: "You have changes that are not saved. Leave without saving them?",
  type: "warning",
  confirmText: "Leave",
  cancelText: "Stay",
} as const

/**
 * The single unsaved-change guard (plan D19). While `isDirty` is true, a route
 * change away from the component — including a param change on the same
 * route, such as opening another file — asks for confirmation, and closing or
 * reloading the tab triggers the browser's own warning. Everything is
 * registered by the owning component and removed when it unmounts.
 *
 * `discards` narrows the prompt to navigations that actually lose the
 * changes, for owners whose buffer outlives some route changes (a kept-alive
 * view). `confirmLeave` asks the same question for in-page actions, such as a
 * close button.
 */
export function useUnsavedChangesGuard(
  isDirty: Ref<boolean>,
  options: { discards?: (to: RouteLocationNormalized) => boolean } = {},
) {
  const { confirm } = useConfirm()
  const discards = options.discards ?? (() => true)

  const confirmLeave = async () => !isDirty.value || confirm(LEAVE_PROMPT)
  const allowNavigation = (to: RouteLocationNormalized) => !discards(to) || confirmLeave()
  onBeforeRouteLeave(allowNavigation)
  onBeforeRouteUpdate(allowNavigation)

  function warnBeforeUnload(event: BeforeUnloadEvent) {
    if (!isDirty.value) return
    event.preventDefault()
    // Older browsers show the prompt only when returnValue is set.
    event.returnValue = ""
  }
  onMounted(() => window.addEventListener("beforeunload", warnBeforeUnload))
  onUnmounted(() => window.removeEventListener("beforeunload", warnBeforeUnload))

  return { confirmLeave }
}
