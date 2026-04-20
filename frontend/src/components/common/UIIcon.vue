<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  name: 'close' | 'plus' | 'check' | 'chevron-down' | 'chevron-right' | 'trash' | 'settings' | 'play' | 'stop' | 'spinner' | 'document' | 'search';
  size?: 'xs' | 'sm' | 'md' | 'lg';
  className?: string;
}>();

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xs': return 'h-3 w-3';
    case 'sm': return 'h-4 w-4';
    case 'lg': return 'h-6 w-6';
    default: return 'h-5 w-5';
  }
});
</script>

<template>
  <svg 
    v-if="name === 'close'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
  </svg>

  <svg 
    v-else-if="name === 'plus'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
  </svg>

  <svg 
    v-else-if="name === 'check'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
  </svg>

  <svg 
    v-else-if="name === 'chevron-down'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
  </svg>

  <svg 
    v-else-if="name === 'trash'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
  </svg>

  <svg 
    v-else-if="name === 'play'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
  </svg>

  <svg 
    v-else-if="name === 'stop'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <rect x="6" y="6" width="12" height="12" rx="2" stroke-width="2" />
  </svg>

  <div v-else-if="name === 'spinner'" :class="[sizeClass, className, 'animate-spin rounded-full border-b-2 border-current']"></div>

  <svg 
    v-else-if="name === 'document'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
  </svg>

  <svg 
    v-else-if="name === 'search'"
    xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, className]" fill="none" viewBox="0 0 24 24" stroke="currentColor"
  >
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
  </svg>
</template>
