import { computed, type Ref } from "vue"
import { useRoute } from "vue-router"
import { useLastWorkspace } from "./useLastWorkspace"
import { buildNavigation, groupNavigation, homeLocation } from "../../utils/navigation"
import type { NavId } from "../../types/routes"

// The shell navigation for the route the operator is on: one list for the sidebar
// and the phone tab bar, and the home location the brand links to.
export function useShellNavigation(badges: Readonly<Ref<Partial<Record<NavId, number>>>>) {
  const route = useRoute()
  const { lastWorkspace } = useLastWorkspace()

  const items = computed(() =>
    buildNavigation({ lastWorkspace: lastWorkspace.value, routeName: route.name as string | null | undefined, destination: route.meta.destination, badges: badges.value }),
  )
  const entries = computed(() => groupNavigation(items.value))
  const home = computed(() => homeLocation(lastWorkspace.value))
  return { items, entries, home }
}
