<script setup lang="ts">
import { computed } from "vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import NetauditToolbar from "../netaudit/NetauditToolbar.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { stackSourcesOf } from "../../domain/dashboard"
import { HOME_BOARD_ID } from "../../constants/dashboard"
import { RADIO_DETAIL_LABEL } from "../../constants/radio"
import { VIEW_MODE_OPTIONS } from "../../constants/netaudit"
import type { DashboardBoard, DashboardCardDef } from "../../types/dashboard"
import type { NetauditViewMode } from "../../types/netaudit"

// Simple or Advanced (one remembered choice for Network and Radio) and Help, for a board that holds a stack's cards. A stack with sub-tabs
// (Network) shows it at the end of the sub-tab row; Radio shows it above its board (DashboardStackChrome).
const props = defineProps<{ board: DashboardBoard; defs: ReadonlyMap<string, DashboardCardDef> }>()
const { mode, set } = useNetauditViewMode()
const stacks = computed(() => stackSourcesOf(props.board, props.defs))
</script>

<template>
  <div v-if="board.id !== HOME_BOARD_ID && (stacks.network || stacks.radio)" class="flex justify-end" data-test="stack-toolbar">
    <NetauditToolbar v-if="stacks.network" />
    <SegmentedControl v-else :model-value="mode" :options="VIEW_MODE_OPTIONS" :label="RADIO_DETAIL_LABEL" variant="compact" @update:model-value="set($event as NetauditViewMode)" />
  </div>
</template>
