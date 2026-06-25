<script setup lang="ts">
import { computed } from 'vue';
import Icon from '../icons/Icon.vue';

interface Props {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'glass';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
  disabled?: boolean;
  icon?: string;
  iconOnly?: boolean;
  type?: 'button' | 'submit' | 'reset';
  className?: string;
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  type: 'button',
  className: ''
});

const baseClasses = "inline-flex items-center justify-center font-bold uppercase tracking-wider transition-all duration-200 active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed";

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary': return "bg-blue-600 hover:bg-blue-500 text-white border border-blue-400/30 shadow-lg shadow-blue-500/20";
    case 'secondary': return "bg-gray-700 hover:bg-gray-600 text-gray-200 border border-white/5";
    case 'danger': return "bg-red-600 hover:bg-red-500 text-white border border-red-400/30 shadow-lg shadow-red-500/20";
    case 'ghost': return "bg-transparent hover:bg-white/5 text-gray-400 hover:text-white border border-transparent";
    case 'glass': return "bg-white/5 hover:bg-white/10 backdrop-blur-md text-white border border-white/10";
    default: return "";
  }
});

const sizeClasses = computed(() => {
  if (props.iconOnly) {
    switch (props.size) {
      case 'sm': return "p-1.5 rounded-full";
      case 'lg': return "p-3 rounded-full";
      default: return "p-2 rounded-full";
    }
  }
  switch (props.size) {
    case 'sm': return "px-3 py-1.5 text-[9px] rounded";
    case 'lg': return "px-6 py-3 text-sm rounded-lg";
    default: return "px-4 py-2 text-[10px] rounded-md";
  }
});
</script>

<template>
  <button 
    :type="type"
    :disabled="disabled || loading"
    :class="[baseClasses, variantClasses, sizeClasses, className]"
  >
    <Icon v-if="loading" name="spinner" size="sm" class="mr-2" />
    <template v-else>
      <Icon v-if="icon" :name="(icon as any)" :size="size === 'lg' ? 'md' : 'sm'" :class="{ 'mr-2': !iconOnly }" />
      <slot v-if="!iconOnly"></slot>
    </template>
  </button>
</template>
