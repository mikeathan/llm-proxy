<script setup lang="ts">
import { computed } from "vue"
import { toDashboard } from "../../router/routes"
import { BOARD_HINT } from "../../constants/dashboard"
import type { BoardHintKey } from "../../types/dashboard"

// The line on a stack page that says where its views went and links to them: the stack's own board on the dashboard, or one card of it. The stack
// page keeps control and settings; the dashboard keeps the views, and this is the way between them.
const props = defineProps<{ stack: BoardHintKey }>()
const hint = computed(() => BOARD_HINT[props.stack])
</script>

<template>
  <p class="m-0 text-[length:var(--text-small)] text-muted">
    {{ hint.text }}
    <RouterLink :to="toDashboard(hint.board)" class="text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="open-board">{{ hint.link }} →</RouterLink>
  </p>
</template>
