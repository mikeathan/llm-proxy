<script setup lang="ts">
import { computed, onUnmounted, ref, useId, watch } from "vue"
import type { NotificationItem, NotificationTone, RunNotification } from "../../../types/notifications"
import { describeNotification, summarizeBatch } from "../../../utils/notifications/present"
import Icon from "../../icons/Icon.vue"
import BaseButton from "../buttons/BaseButton.vue"

// The top-strip bell (plan Phase 4): runs that ended and reminders that fired
// while the user was elsewhere, one entry per poll tick. Presentation only — the
// owner supplies the notifications and handles open / dismiss / clear. On a phone the panel spans the screen under the
// header (the bell is not at the right edge, so anchoring to it cut the panel off); from `sm` it hangs from the bell.

const props = defineProps<{ notifications: RunNotification[] }>()

const emit = defineEmits<{
  (e: "open", item: NotificationItem): void
  (e: "dismiss", batchId: string): void
  (e: "clear"): void
}>()

// Outcome is always spelled out; colour only reinforces it.
const TONE_CLASS: Record<NotificationTone, string> = {
  error: "text-state-error",
  warning: "text-state-running",
  success: "text-state-success",
  muted: "text-muted",
}

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panelId = useId()

const unread = computed(() => props.notifications.reduce((n, batch) => n + batch.items.length, 0))
const bellLabel = computed(() => {
  if (!unread.value) return "No unread notifications"
  return `${unread.value} unread notification${unread.value === 1 ? "" : "s"}`
})

function onOpen(item: NotificationItem) {
  open.value = false
  emit("open", item)
}

// Outside click and Escape close the panel; listeners live only while open.
function onDocumentClick(event: MouseEvent) {
  if (root.value && event.target instanceof Node && !root.value.contains(event.target)) open.value = false
}
function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") open.value = false
}
function removeListeners() {
  document.removeEventListener("click", onDocumentClick)
  document.removeEventListener("keydown", onDocumentKeydown)
}
watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener("click", onDocumentClick)
    document.addEventListener("keydown", onDocumentKeydown)
  } else {
    removeListeners()
  }
})
onUnmounted(removeListeners)
</script>

<template>
  <div ref="root" class="relative flex-none">
    <button
      type="button"
      aria-haspopup="true"
      :aria-expanded="open"
      :aria-controls="panelId"
      :aria-label="bellLabel"
      class="relative inline-flex h-[30px] w-[30px] items-center justify-center rounded-[var(--radius-md)] text-muted hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2"
      @click="open = !open"
    >
      <Icon name="bell" size="sm" />
      <span
        v-if="unread"
        aria-hidden="true"
        class="absolute right-0 top-0.5 min-w-[15px] rounded-full bg-accent-info px-[3px] text-center font-mono text-[9px] font-semibold leading-[15px] text-primary"
      >{{ unread }}</span>
    </button>

    <div
      v-if="open"
      :id="panelId"
      role="region"
      aria-label="Notifications"
      class="fixed inset-x-3 top-14 z-30 flex max-h-[70vh] flex-col overflow-hidden rounded-[var(--radius-sm)] border border-hairline bg-canvas sm:absolute sm:inset-x-auto sm:right-0 sm:top-full sm:mt-2 sm:w-[340px]"
    >
      <div class="flex flex-none items-center justify-between gap-2 border-b border-hairline px-3 py-2">
        <h2 class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Notifications</h2>
        <BaseButton v-if="notifications.length" variant="ghost" size="sm" @click="emit('clear')">Clear all</BaseButton>
      </div>

      <p v-if="!notifications.length" class="px-3 py-4 text-[length:var(--text-small)] text-muted">
        Nothing new since you looked.
      </p>

      <ul v-else class="min-h-0 flex-1 overflow-y-auto">
        <li v-for="batch in [...notifications].reverse()" :key="batch.id" class="border-b border-hairline px-3 py-2 last:border-b-0">
          <div class="flex items-start justify-between gap-2">
            <p class="text-[length:var(--text-small)] font-semibold text-primary">{{ summarizeBatch(batch.items) }}</p>
            <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Dismiss notification" @click="emit('dismiss', batch.id)" />
          </div>
          <ul class="mt-1 flex flex-col gap-1">
            <li v-for="item in batch.items" :key="item.id" data-test="notification-item" class="flex flex-col gap-0.5">
              <RouterLink
                :to="item.target"
                class="flex min-w-0 items-baseline gap-2 rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] text-secondary hover:text-primary focus-visible:outline-none focus-visible:ring-2"
                @click="onOpen(item)"
              >
                <span :class="['flex-none', TONE_CLASS[describeNotification(item).tone]]">{{ describeNotification(item).label }}</span>
                <span class="min-w-0 truncate">{{ describeNotification(item).text }}</span>
              </RouterLink>
              <p v-if="describeNotification(item).detail" class="line-clamp-2 break-words text-[length:var(--text-micro)] text-muted">{{ describeNotification(item).detail }}</p>
            </li>
          </ul>
        </li>
      </ul>
    </div>
  </div>
</template>
