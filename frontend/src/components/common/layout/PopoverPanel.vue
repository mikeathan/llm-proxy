<script setup lang="ts">
import BaseButton from "../buttons/BaseButton.vue"
import MicroLabel from "../display/MicroLabel.vue"

// The panel under a header control (run pill, host stats): a popover from `sm`
// up, a bottom sheet over a scrim below it. The owner decides when it is open
// (v-if) and dismisses it through useDismissable; this only draws it.
const props = withDefaults(defineProps<{ id: string; title: string; width?: string }>(), { width: "sm:w-[360px]" })

defineEmits<{ (e: "close"): void }>()
defineSlots<{ default(): unknown }>()
</script>

<template>
  <div aria-hidden="true" class="fixed inset-0 z-20 bg-scrim/50 sm:hidden" @click="$emit('close')"></div>
  <div
    :id="id"
    role="dialog"
    :aria-label="title"
    :class="[
      'z-30 flex flex-col border border-strong bg-surface max-sm:fixed max-sm:inset-x-0 max-sm:bottom-0 max-sm:max-h-[80vh] max-sm:overflow-y-auto max-sm:border-x-0 max-sm:border-b-0 sm:absolute sm:right-0 sm:top-full sm:mt-2 sm:rounded-[var(--radius-sm)]',
      props.width,
    ]"
  >
    <div class="flex items-center justify-between gap-2 border-b border-hairline px-3 py-2">
      <MicroLabel>{{ title }}</MicroLabel>
      <BaseButton variant="ghost" size="sm" icon="close" icon-only :label="`Close ${title.toLowerCase()}`" @click="$emit('close')" />
    </div>
    <slot />
  </div>
</template>
