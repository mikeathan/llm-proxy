import { shallowRef, watch } from "vue"
import { useRoute } from "vue-router"
import type { Destination, RouteSnapshot } from "../../types/routes"

/**
 * The current route as seen by one destination's view: it follows routes of
 * that destination and holds the last one while the app is elsewhere. A
 * kept-alive view (plan D19) keeps reacting while in the background, so
 * reading useRoute() directly would re-render it for another page — unmounting
 * the chat or editor state that keep-alive exists to preserve.
 */
export function useDestinationRoute(destination: Destination) {
  const route = useRoute()
  const snapshot = shallowRef<RouteSnapshot>(capture())

  function capture(): RouteSnapshot {
    return { name: route.name, params: { ...route.params }, query: { ...route.query }, fullPath: route.fullPath }
  }

  watch(
    () => route.fullPath,
    () => {
      if (route.meta.destination === destination) snapshot.value = capture()
    },
  )
  return snapshot
}
