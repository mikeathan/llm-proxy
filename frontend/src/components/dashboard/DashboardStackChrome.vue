<script setup lang="ts">
import { computed } from "vue"
import DashboardStackToolbar from "./DashboardStackToolbar.vue"
import NetauditHelp from "../netaudit/NetauditHelp.vue"
import ReadinessBanner from "../netaudit/ReadinessBanner.vue"
import { stackSourcesOf } from "../../domain/dashboard"
import { HOME_BOARD_ID } from "../../constants/dashboard"
import type { DashboardBoard, DashboardCardDef } from "../../types/dashboard"

// What the cards of a stack share, shown above any board that holds one of them: Simple or Advanced (one remembered choice for Network and Radio), Help,
// and what is missing before a scan can run. It used to sit on the stack's own page, which no longer shows the cards. When the board has sub-tabs
// (`toolbar` false) Simple or Advanced sits at the end of the sub-tab row instead, so no row of controls is spent on it.
const props = withDefaults(defineProps<{ board: DashboardBoard; defs: ReadonlyMap<string, DashboardCardDef>; toolbar?: boolean }>(), { toolbar: true })
const stacks = computed(() => stackSourcesOf(props.board, props.defs))
</script>

<template>
  <div v-if="board.id !== HOME_BOARD_ID && (stacks.network || stacks.radio)" class="flex flex-col gap-3" data-test="stack-chrome">
    <DashboardStackToolbar v-if="toolbar" :board="board" :defs="defs" />
    <ReadinessBanner v-if="stacks.network" />
    <NetauditHelp v-if="stacks.network" />
  </div>
</template>
