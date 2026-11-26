import { computed, type Ref } from "vue"
import { useRoute } from "vue-router"
import { useLastWorkspace } from "./useLastWorkspace"
import { useEnabledStacks } from "../stacks/useEnabledStacks"
import { useBriefUnread } from "../newsroom/useBriefUnread"
import { useAlertsUnread } from "../newsroom/useAlertsUnread"
import { CRITICAL_HOME_DOT, unreadAlertsWords, unreadBriefWords } from "../../constants/newsroom"
import { buildNavigation, groupNavigation, homeLocation } from "../../utils/navigation"
import type { NavId } from "../../types/routes"

// The shell navigation for the route the operator is on: one list for the sidebar
// and the phone tab bar, and the home location the brand links to.
export function useShellNavigation(badges: Readonly<Ref<Partial<Record<NavId, number>>>>) {
  const route = useRoute()
  const { lastWorkspace } = useLastWorkspace()
  const stacks = useEnabledStacks()
  // A brief waiting unread is a dot on Dashboard, where the brief is read (ADR-0089), never a run-notification count: it shares the one poller the
  // dashboard's Newsroom tab uses. The Newsroom entry opens the stack's setup, so it carries no dot.
  const { unreadCount, criticalPages } = useBriefUnread()
  // Unread alerts in the feed dot Dashboard too; the dot's words name both when both wait.
  const { unread: unreadAlerts } = useAlertsUnread()
  const newsroomWords = computed(() =>
    [unreadCount.value > 0 ? unreadBriefWords(unreadCount.value) : "", unreadAlerts.value > 0 ? unreadAlertsWords(unreadAlerts.value) : ""].filter(Boolean).join(", "),
  )
  const dots = computed<Partial<Record<NavId, string>>>(() => ({
    ...(newsroomWords.value ? { dashboard: newsroomWords.value } : {}),
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
