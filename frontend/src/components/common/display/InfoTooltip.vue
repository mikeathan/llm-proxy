<script setup lang="ts">
defineProps<{
  text: string
}>()
</script>

<template>
  <span class="tooltip-wrap">
    <svg class="info-icon" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1ZM7 5.5a1 1 0 1 1 2 0 1 1 0 0 1-2 0ZM9 11a1 1 0 0 1-2 0V7a1 1 0 0 1 2 0v4Z"/>
    </svg>
    <span class="tooltip-text">{{ text }}</span>
  </span>
</template>

<style scoped>
.tooltip-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  margin-left: 4px;
  cursor: help;
  vertical-align: middle;
}
.info-icon {
  width: 14px;
  height: 14px;
  color: #6b7280;
  transition: color 0.15s;
}
.tooltip-wrap:hover .info-icon {
  color: #9ca3af;
}
.tooltip-text {
  visibility: hidden;
  opacity: 0;
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  background: #1f2937;
  color: #d1d5db;
  font-size: 11px;
  padding: 6px 10px;
  border-radius: 6px;
  white-space: nowrap;
  max-width: 280px;
  border: 1px solid #374151;
  pointer-events: none;
  transition: opacity 0.15s;
  z-index: 20;
  line-height: 1.4;
  text-align: center;
}
.tooltip-wrap:hover .tooltip-text {
  visibility: visible;
  opacity: 1;
}
</style>
