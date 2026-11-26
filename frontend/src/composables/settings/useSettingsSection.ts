import { computed, watch, type Ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import type { SettingsTab } from "../../types/admin"
import { SETTINGS_SECTIONS } from "../../domain/settings"
import { ROUTE_NAMES } from "../../types/routes"
import { useDestinationRoute } from "../ui/useDestinationRoute"

// The page without a section opens the first one (General's first).
const DEFAULT_SETTINGS_TAB: SettingsTab = SETTINGS_SECTIONS[0]!
const SETTINGS_DESTINATION = "settings"

/**
 * The active Settings section, from `/settings/:section` (plan D18). Read
 * through the Settings route snapshot, so the page never reacts to another
 * destination's `section` param while it transitions out. Once `ready`, an
 * unknown section re-resolves to the in-shell
 * not-found page at the same URL — like any other bad link, never a silent
 * redirect.
 */
export function useSettingsSection(isKnown: (tab: SettingsTab) => boolean, ready: Readonly<Ref<boolean>>) {
  const route = useRoute()
  const router = useRouter()
  const current = useDestinationRoute(SETTINGS_DESTINATION)

  const activeTab = computed<SettingsTab>(() => {
    const section = current.value.params.section
    return typeof section === "string" && section ? (section as SettingsTab) : DEFAULT_SETTINGS_TAB
  })

  watch([activeTab, ready], ([tab, isReady]) => {
    if (!isReady || isKnown(tab) || route.meta.destination !== SETTINGS_DESTINATION) return
    void router.replace({
      name: ROUTE_NAMES.notFound,
      params: { pathMatch: route.path.slice(1).split("/") },
      query: route.query,
    })
  })

  return { activeTab }
}
