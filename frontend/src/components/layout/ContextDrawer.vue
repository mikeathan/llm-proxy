<script setup lang="ts">
import { nextTick, ref, useId, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"

// Right-hand overlay for detail / monitor content (plan Phase 2): replaces the
// permanent right rail at every width, so the page keeps the full width until
// the user asks for context. Modal while open: Escape and the scrim close it,
// focus moves inside, and returns to whatever opened it.

const props = defineProps<{
  open: boolean
  title: string
  /** Room for detail content (e.g. a run's output) rather than a monitor column. */
  wide?: boolean
}>()

const emit = defineEmits<{ (e: "update:open", open: boolean): void }>()

const titleId = useId()
const closeButton = ref<InstanceType<typeof BaseButton> | null>(null)
let opener: HTMLElement | null = null

const close = () => emit("update:open", false)

watch(
  () => props.open,
  async (open) => {
    if (open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      ;(closeButton.value?.$el as HTMLElement | undefined)?.focus()
    } else {
      opener?.focus()
      opener = null
    }
  },
  { immediate: true },
)
</script>

<template>
  <template v-if="open">
    <div data-test="drawer-scrim" class="fixed inset-0 z-30 bg-scrim/60" aria-hidden="true" @click="close"></div>
    <div
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      :class="['fixed inset-y-0 right-0 z-40 flex w-full flex-col border-l border-hairline bg-canvas', wide ? 'max-w-[720px]' : 'max-w-[360px]']"
      @keydown.escape="close"
    >
      <div class="flex h-12 flex-none items-center justify-between gap-3 border-b border-hairline px-4">
        <h2 :id="titleId" class="font-mono text-[length:var(--text-small)] font-semibold text-primary">{{ title }}</h2>
        <BaseButton ref="closeButton" variant="ghost" icon="close" icon-only :label="`Close ${title}`" @click="close" />
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto p-4">
        <slot />
      </div>
    </div>
  </template>
</template>
