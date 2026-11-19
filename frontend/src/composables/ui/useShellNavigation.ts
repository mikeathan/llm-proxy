import { computed, type Ref } from "vue"
import { useRoute } from "vue-router"
import { useLastWorkspace } from "./useLastWorkspace"
import { useEnabledStacks } from "../stacks/useEnabledStacks"
import { buildNavigation, groupNavigation, homeLocation } from "../../utils/navigation"
import type { NavId } from "../../types/routes"

// The shell navigation for the route the operator is on: one list for the sidebar
// and the phone tab bar, and the home location the brand links to.
export function useShellNavigation(badges: Readonly<Ref<Partial<Record<NavId, number>>>>) {
  const route = useRoute()
  const { lastWorkspace } = useLastWorkspace()
  const stacks = useEnabledStacks()

  const items = computed(() =>
    buildNavigation({ stacksOn: stacks.on.value, lastWorkspace: lastWorkspace.value, routeName: route.name as string | null | undefined, destination: route.meta.destination, board: typeof route.query.board === 'string' ? route.query.board : undefined, badges: badges.value }),
  )
  const entries = computed(() => groupNavigation(items.value))
  const home = computed(() => homeLocation())
  return { items, entries, home }
}
