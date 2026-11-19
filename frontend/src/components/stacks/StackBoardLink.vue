<script setup lang="ts">
import { computed } from "vue"
import { toDashboard } from "../../router/routes"
import { BOARD_HINT } from "../../constants/dashboard"

// The line under a stack page's tabs that says where its views went and links to them: the stack's own board on the dashboard. The stack
// page keeps control and settings; the dashboard keeps the views, and this is the way between them.
const props = defineProps<{ stack: keyof typeof BOARD_HINT }>()
const hint = computed(() => BOARD_HINT[props.stack])
</script>

<template>
  <p class="m-0 text-[length:var(--text-small)] text-muted">
    {{ hint.text }}
    <RouterLink :to="toDashboard(stack)" class="text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="open-board">{{ hint.link }} →</RouterLink>
  </p>
</template>
