import { ref } from 'vue'
import type { AppBannerMessage } from '../../types/ui'

// Generic, domain-agnostic banner bus. It is a pure sink: any part of the UI
// may `show` a fully-formed message and the bus exposes the latest one as
// `active` for AppBanner.vue to render. Precedence between messages (e.g. a
// transient error overriding a persistent warning) is coordinated by the
// emitting components, not by this composable.
const active = ref<AppBannerMessage | null>(null)

export function useAppBanner() {
  const show = (msg: AppBannerMessage) => {
    active.value = msg
  }
  const clear = () => {
    active.value = null
  }
  return { active, show, clear }
}
