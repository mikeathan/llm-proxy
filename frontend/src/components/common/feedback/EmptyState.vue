<script setup lang="ts">
import BrandMark from "../../layout/BrandMark.vue"
import MicroLabel from "../display/MicroLabel.vue"

// One statement of what is missing, why it matters, and the next action.
defineProps<{ title: string; body?: string }>()
defineSlots<{ action?(): unknown }>()
</script>

<template>
  <div class="flex max-w-[60ch] flex-col items-start gap-2 px-2 py-8">
    <BrandMark :size="20" muted />
    <MicroLabel>Empty</MicroLabel>
    <h3 class="m-0 text-[15px] font-semibold text-primary">{{ title }}</h3>
    <p v-if="body" class="m-0 text-muted">{{ body }}</p>
    <div v-if="$slots.action" class="mt-1"><slot name="action" /></div>
  </div>
</template>
