<script setup lang="ts">
import StatusTag from "../display/StatusTag.vue"

// What failed, the cause, and the next step — never "something went wrong".
defineProps<{ title: string; cause: string; next: string }>()
defineSlots<{ action?(): unknown }>()
</script>

<template>
  <div role="alert" class="flex max-w-[60ch] flex-col items-start gap-2 px-2 py-8">
    <StatusTag state="error" label="Error" />
    <h3 class="m-0 text-[15px] font-semibold text-primary">{{ title }}</h3>
    <p class="m-0 text-muted">{{ cause }}</p>
    <p class="m-0 text-secondary">{{ next }}</p>
    <div v-if="$slots.action" class="mt-1"><slot name="action" /></div>
  </div>
</template>
