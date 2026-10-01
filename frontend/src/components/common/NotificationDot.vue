<script setup lang="ts">
defineProps<{
  count?: number
}>()
</script>

<template>
  <span class="notif-wrap">
    <span class="notif-dot"></span>
    <span v-if="count && count > 1" class="notif-count">{{ count }}</span>
  </span>
</template>

<style scoped lang="postcss">
.notif-wrap {
  @apply absolute top-0 right-0 flex items-center justify-center;
}

.notif-dot {
  @apply absolute top-0.5 right-0.5 w-1.5 h-1.5 rounded-full;
  background: rgb(var(--state-live));
  box-shadow: 0 0 6px rgb(var(--state-live) / 0.6);
}

.notif-count {
  @apply absolute -top-1 -right-1 min-w-[14px] h-[14px] px-0.5 rounded-full text-[9px] font-bold flex items-center justify-center;
  background: rgb(var(--state-live));
  color: rgb(var(--text-inverse));
  box-shadow: 0 0 6px rgb(var(--state-live) / 0.6);
}
</style>
