<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, type RouteLocationNormalizedLoaded } from 'vue-router'
import SkipLink from './components/layout/SkipLink.vue'
import AppSidebar from './components/layout/AppSidebar.vue'
import AdminHeader from './components/layout/AdminHeader.vue'
import { useModels } from './composables/models/useModels'
import { useMcpServers } from './composables/system/useMcpServers'
import { useModelBanner } from './composables/models/useModelBanner'
import { useResponsiveLayout } from './composables/ui/useResponsiveLayout'
import Toast from './components/ui/Toast.vue'
import ConfirmDialog from './components/ui/ConfirmDialog.vue'
import AppBanner from './components/ui/AppBanner.vue'
import { useConfirm } from './composables/ui/useConfirm'
import { useRunNotifications } from './composables/assistant/useRunNotifications'
import RunNotifications from './components/common/display/RunNotifications.vue'
import type { Destination } from './types/routes'
import type { RunEndItem } from './types/notifications'

const { isOpen, options, handleConfirm, handleCancel } = useConfirm()
const { recompute: recomputeModelBanner } = useModelBanner()

const route = useRoute()

// Run notifications (plan Phase 4): the bell lives in the top strip and each
// destination owning unread runs gets a badge. Opening a run clears it.
const runNotifications = useRunNotifications()
const destinationBadges = computed(() => {
  const counts: Partial<Record<Destination, number>> = {}
  for (const batch of runNotifications.notifications.value) {
    for (const item of batch.items) counts[item.destination] = (counts[item.destination] ?? 0) + 1
  }
  return counts
})
const openRun = (item: RunEndItem) => runNotifications.dismissItem(item.id)
const { isMobile } = useResponsiveLayout()
const drawerOpen = ref(false)
watch(isMobile, (mobile) => {
  if (!mobile) drawerOpen.value = false
})

// Keep-alive is opt-in by route meta (plan D19): once a `keepAlive` route has
// resolved its lazy component, that component's name joins the include list.
// Bounded by the number of keep-alive destinations, so the cache cannot grow.
const keepAliveNames = ref<string[]>([])
function componentName(r: RouteLocationNormalizedLoaded): string | undefined {
  const component = r.matched[r.matched.length - 1]?.components?.default as { name?: string; __name?: string } | undefined
  return component?.name ?? component?.__name
}
watch(
  () => route.fullPath,
  () => {
    const name = route.meta.keepAlive ? componentName(route) : undefined
    if (name && !keepAliveNames.value.includes(name)) keepAliveNames.value = [...keepAliveNames.value, name]
  },
  { immediate: true },
)
const contentClass = computed(() =>
  route.meta.fullWidth ? 'w-full px-4 py-4 md:px-6 [counter-reset:section]' : 'mx-auto w-full max-w-[1120px] px-4 py-4 md:px-6 md:py-8 [counter-reset:section]',
)

const { state, refresh: refreshModels } = useModels()
const { refresh: refreshMcp } = useMcpServers()

// The model warning logic lives in useModelBanner, which registers its
// state/transient watchers once at module load (no per-component startup
// wiring). The only trigger we keep here is the centralized post-refresh
// recompute so the warning appears/disappears reactively after a Settings save
// or model add without a page reload.
onMounted(() => {
  Promise.all([refreshModels(), refreshMcp()]).then(recomputeModelBanner)
})
</script>

<template>
  <RouterView v-if="route.meta.bare" />
  <div v-else class="flex min-h-screen bg-canvas font-sans text-secondary">
    <SkipLink target="content" />
    <AppSidebar v-model:open="drawerOpen" :mobile="isMobile" :badges="destinationBadges" />

    <div class="flex min-w-0 flex-1 flex-col">
      <AdminHeader :mobile="isMobile" :drawer-open="drawerOpen" @toggle-drawer="drawerOpen = !drawerOpen">
        <template #actions>
          <RunNotifications
            :notifications="runNotifications.notifications.value"
            @open="openRun"
            @dismiss="runNotifications.dismiss"
            @clear="runNotifications.clear"
          />
        </template>
      </AdminHeader>

      <main id="content" tabindex="-1" :class="[contentClass, 'focus:outline-none']">
        <div v-if="!state" class="flex items-center justify-center py-20" role="status" aria-live="polite">
          <span class="font-mono text-[length:var(--text-small)] text-muted">loading…</span>
        </div>

        <template v-else>
          <AppBanner class="mb-4" />
          <RouterView v-slot="{ Component }">
            <Transition name="route" mode="out-in">
              <KeepAlive :include="keepAliveNames" :max="keepAliveNames.length || 1">
                <component :is="Component" />
              </KeepAlive>
            </Transition>
          </RouterView>
        </template>
      </main>
    </div>
  </div>

  <Toast />
  <ConfirmDialog
    v-model="isOpen"
    v-bind="options"
    @confirm="handleConfirm"
    @cancel="handleCancel"
  />
</template>

<style>
/* Route transitions use the motion tokens (plan D17); reduced motion zeroes them. */
.route-enter-active,
.route-leave-active {
  transition: opacity var(--motion-fast) var(--ease-standard);
}
.route-enter-from,
.route-leave-to {
  opacity: 0;
}
</style>
