import { computed, effectScope, ref, watch } from "vue"
import { useDispatcher } from "../automation/useDispatcher"
import { useLastWorkspace } from "./useLastWorkspace"

// The workspace the shell is "in" (plan frontend-redesign-v2, V1): the one last used,
// which the Assistant and Workspace entries, the ask panel and the header chip all
// open. This is the one place that keeps it valid — a remembered workspace that no
// longer exists is forgotten, and when none is remembered the first one is used — so
// no page has to. `refresh()` is the app's single start-up call (App.vue); the watcher
// registers once, in a scope of its own (it must outlive whichever component asked
// first), and also covers a workspace deleted from the Workspaces page.
const loaded = ref(false)
const watcherScope = effectScope(true)
let watching = false

export function useActiveWorkspace() {
  const { workspaces, fetchWorkspaces } = useDispatcher()
  const { lastWorkspace, remember, forget } = useLastWorkspace()

  function reconcile() {
    if (!loaded.value) return
    const ids = workspaces.value.map((w) => w.id)
    if (lastWorkspace.value && !ids.includes(lastWorkspace.value)) forget(lastWorkspace.value)
    if (!lastWorkspace.value && ids[0]) remember(ids[0])
  }

  if (!watching) {
    watching = true
    watcherScope.run(() => watch([workspaces, loaded], reconcile, { immediate: true }))
  }

  async function refresh() {
    await fetchWorkspaces()
    loaded.value = true
    reconcile()
  }

  function select(id: string) {
    if (workspaces.value.some((w) => w.id === id)) remember(id)
  }

  return { workspaces, active: computed(() => lastWorkspace.value), loaded, refresh, select }
}
