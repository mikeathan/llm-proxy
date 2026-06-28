<script setup lang="ts">
interface Props {
  modelValue: boolean;
  label?: string;
  disabled?: boolean;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
}>();

const toggle = () => {
  if (props.disabled) return;
  emit('update:modelValue', !props.modelValue);
};
</script>

<template>
  <div class="flex items-center gap-3 cursor-pointer group" @click="toggle">
    <div 
      class="relative w-10 h-5 rounded-full transition-all duration-300 border"
      :class="[
        modelValue ? 'bg-blue-600/20 border-blue-500/50' : 'bg-gray-800 border-white/5',
        disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'
      ]"
    >
      <div 
        class="absolute top-1 left-1 w-3 h-3 rounded-full transition-all duration-300 shadow-sm"
        :class="modelValue ? 'translate-x-5 bg-blue-400' : 'translate-x-0 bg-gray-500'"
      ></div>
    </div>
    <span v-if="label" class="text-[10px] font-bold uppercase tracking-wider text-gray-500 group-hover:text-gray-300 transition-colors">
      {{ label }}
    </span>
  </div>
</template>
