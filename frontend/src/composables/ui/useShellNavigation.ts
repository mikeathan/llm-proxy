import { computed, type Ref } from "vue"
import { useRoute } from "vue-router"
import { useLastWorkspace } from "./useLastWorkspace"
import { useEnabledStacks } from "../stacks/useEnabledStacks"
import { useBriefUnread } from "../newsroom/useBriefUnread"
import { CRITICAL_HOME_DOT, unreadBriefWords } from "../../constants/newsroom"
import { buildNavigation, groupNavigation, homeLocation } from "../../utils/navigation"
import type { NavId } from "../../types/routes"

// The shell navigation for the route the operator is on: one list for the sidebar
// and the phone tab bar, and the home location the brand links to.
export function useShellNavigation(badges: Readonly<Ref<Partial<Record<NavId, number>>>>) {
  const route = useRoute()
  const { lastWorkspace } = useLastWorkspace()
  const stacks = useEnabledStacks()
  // A brief waiting unread is a dot on Newsroom, never a run-notification count: it shares the one poller the Briefs tab and the dashboard use.
  const { unreadCount, criticalPages } = useBriefUnread()
  const dots = computed<Partial<Record<NavId, string>>>(() => ({
    ...(unreadCount.value > 0 ? { newsroom: unreadBriefWords(unreadCount.value) } : {}),
    // A critical fact in the latest unread brief dots the page it belongs to (ADR-0084): the brief says it, the page is where to look.
    ...Object.fromEntries(criticalPages.value.map((p) => [p, CRITICAL_HOME_DOT])),
  }))

  const items = computed(() =>
    buildNavigation({ stacksOn: stacks.on.value, lastWorkspace: lastWorkspace.value, routeName: route.name as string | null | undefined, destination: route.meta.destination, badges: badges.value, dots: dots.value }),
  )
  const entries = computed(() => groupNavigation(items.value))
  const home = computed(() => homeLocation())
  return { items, entries, home }
}
