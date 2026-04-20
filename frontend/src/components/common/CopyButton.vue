<script setup lang="ts">
import { ref, computed } from 'vue';

const props = defineProps<{
  text: any; // Text or object to copy
  title?: string; // HTML title attribute
  iconSize?: 'sm' | 'md' | 'lg'; // Size of the SVG icon
}>();

const isCopied = ref(false);

const sizeClass = computed(() => {
  return {
    'sm': 'w-3 h-3',
    'md': 'w-4 h-4',
    'lg': 'w-5 h-5',
  }[props.iconSize || 'md'];
});

const handleCopy = async (event: Event) => {
  event.stopPropagation();
  event.preventDefault();

  try {
    const stringToCopy = typeof props.text === 'string' ? props.text : JSON.stringify(props.text, null, 2);
    await navigator.clipboard.writeText(stringToCopy);
    
    isCopied.value = true;
    setTimeout(() => {
      isCopied.value = false;
    }, 2000);
  } catch (err) {
    console.error('Failed to copy text', err);
  }
};
</script>

<template>
  <button 
    @click="handleCopy"
    :title="title || 'Copy to clipboard'"
    class="copy-btn"
  >
    <!-- Checkmark SVG -->
    <svg v-if="isCopied" xmlns="http://www.w3.org/2000/svg" :class="[sizeClass, 'text-green-400']" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
    </svg>
    <!-- Copy / Duplicate SVG -->
    <svg v-else xmlns="http://www.w3.org/2000/svg" :class="sizeClass" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
    </svg>
  </button>
</template>

<style scoped lang="postcss">
/* Base styling that can be extended by parents via class attributes */
.copy-btn {
  @apply text-gray-500 hover:text-white transition-colors flex items-center justify-center;
}
</style>
