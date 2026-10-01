<script setup lang="ts">
import { computed, onUnmounted, ref, useId, watch } from "vue"
import type { RunEndItem, RunEndOutcome, RunNotification } from "../../../types/notifications"
import { summarize } from "../../../composables/assistant/useRunNotifications"
import Icon from "../../icons/Icon.vue"
import BaseButton from "../buttons/BaseButton.vue"

// The top-strip bell (plan Phase 4): runs that ended while the user was
// elsewhere, one entry per poll tick. Presentation only — the owner supplies
// the notifications and handles open / dismiss / clear.

const props = defineProps<{ notifications: RunNotification[] }>()

const emit = defineEmits<{
  (e: "open", item: RunEndItem): void
  (e: "dismiss", batchId: string): void
  (e: "clear"): void
}>()

// Outcome is always spelled out; colour only reinforces it.
const OUTCOME_CLASS: Record<RunEndOutcome, string> = {
  failed: "text-state-error",
  completed: "text-state-success",
  ended: "text-muted",
}

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panelId = useId()

const unread = computed(() => props.notifications.reduce((n, batch) => n + batch.items.length, 0))
const bellLabel = computed(() => {
  if (!unread.value) return "No unread run notifications"
  return `${unread.value} unread run notification${unread.value === 1 ? "" : "s"}`
})

function onOpen(item: RunEndItem) {
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
      aria-label="Run notifications"
      class="absolute right-0 top-full z-30 mt-2 flex max-h-[70vh] w-[min(340px,calc(100vw-24px))] flex-col overflow-hidden rounded-[var(--radius-sm)] border border-hairline bg-canvas"
    >
      <div class="flex flex-none items-center justify-between gap-2 border-b border-hairline px-3 py-2">
        <h2 class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Runs ended</h2>
        <BaseButton v-if="notifications.length" variant="ghost" size="sm" @click="emit('clear')">Clear all</BaseButton>
      </div>

      <p v-if="!notifications.length" class="px-3 py-4 text-[length:var(--text-small)] text-muted">
        No runs have ended since you looked.
      </p>

      <ul v-else class="min-h-0 flex-1 overflow-y-auto">
        <li v-for="batch in [...notifications].reverse()" :key="batch.id" class="border-b border-hairline px-3 py-2 last:border-b-0">
          <div class="flex items-start justify-between gap-2">
            <p class="text-[length:var(--text-small)] font-semibold text-primary">{{ summarize(batch.items) }}</p>
            <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Dismiss notification" @click="emit('dismiss', batch.id)" />
          </div>
          <ul class="mt-1 flex flex-col gap-1">
            <li v-for="item in batch.items" :key="item.id" data-test="run-item" class="flex flex-col gap-0.5">
              <RouterLink
                :to="item.target"
                class="flex min-w-0 items-baseline gap-2 rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] text-secondary hover:text-primary focus-visible:outline-none focus-visible:ring-2"
                @click="onOpen(item)"
              >
                <span :class="['flex-none', OUTCOME_CLASS[item.outcome]]">{{ item.outcome }}</span>
                <span class="min-w-0 truncate">{{ item.kind === "assistant" ? `assistant · ${item.workspace}` : item.label }}</span>
              </RouterLink>
              <p v-if="item.error" class="line-clamp-2 break-words text-[length:var(--text-micro)] text-muted">{{ item.error }}</p>
            </li>
          </ul>
        </li>
      </ul>
    </div>
  </div>
</template>
