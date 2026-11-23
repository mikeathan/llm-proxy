<script setup lang="ts">
import { computed } from "vue"
import { plateKind } from "../../utils/newsroom/plate"
import type { PlateKind } from "../../types/newsroom"

// Bundled line art for a section: drawn with the theme's colours, so it follows light and dark and works offline. No remote image is ever loaded, so
// opening a brief tells no publisher what the owner reads.
const props = withDefaults(defineProps<{ label: string; category?: string; size?: "sm" | "lg"; kind?: PlateKind }>(), { category: "", size: "sm", kind: undefined })
const shape = computed(() => props.kind ?? plateKind(props.category, props.label))
</script>

<template>
  <svg
    :data-plate="shape"
    viewBox="0 0 120 72"
    :class="['flex-none text-accent-brand', size === 'lg' ? 'h-[72px] w-[120px]' : 'h-9 w-[60px]']"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    aria-hidden="true"
  >
    <template v-if="shape === 'world'"><circle cx="60" cy="36" r="26" /><ellipse cx="60" cy="36" rx="12" ry="26" /><path d="M34 36h52M38 24h44M38 48h44" /></template>
    <template v-else-if="shape === 'economy'"><path d="M14 58V40M30 58V30M46 58V44M62 58V22M78 58V34M94 58V16" /><path d="M12 46l20-14 16 10 18-22 22 6" class="stroke-primary" /></template>
    <template v-else-if="shape === 'tech'"><rect x="40" y="22" width="40" height="28" /><path d="M50 22v-8M60 22v-8M70 22v-8M50 50v8M60 50v8M70 50v8M40 32h-8M40 42h-8M80 32h8M80 42h8" /><rect x="52" y="30" width="16" height="12" class="stroke-primary" /></template>
    <template v-else-if="shape === 'politics'"><path d="M20 58h80M26 58V30M42 58V30M58 58V30M74 58V30M90 58V30M18 30h84L60 12z" /></template>
    <template v-else-if="shape === 'science'"><ellipse cx="60" cy="36" rx="38" ry="14" /><ellipse cx="60" cy="36" rx="38" ry="14" transform="rotate(60 60 36)" /><ellipse cx="60" cy="36" rx="38" ry="14" transform="rotate(120 60 36)" /><circle cx="60" cy="36" r="4" class="stroke-primary" /></template>
    <template v-else-if="shape === 'home'"><path d="M26 58V34l34-20 34 20v24z" /><rect x="52" y="40" width="16" height="18" class="stroke-primary" /><path d="M96 20c8 6 8 14 0 20M104 14c13 10 13 26 0 36" /></template>
    <template v-else-if="shape === 'watch'"><path d="M20 52c10-24 20-24 30-8s20 12 50-20" /><circle cx="50" cy="44" r="4" class="stroke-primary" /></template>
    <template v-else><path d="M16 14h88v44H16z" /><path d="M26 26h38M26 36h38M26 46h26" /><rect x="72" y="26" width="22" height="20" class="stroke-primary" /></template>
  </svg>
</template>
