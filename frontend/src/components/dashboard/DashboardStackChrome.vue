<script setup lang="ts">
import { computed } from "vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import NetauditToolbar from "../netaudit/NetauditToolbar.vue"
import NetauditHelp from "../netaudit/NetauditHelp.vue"
import ReadinessBanner from "../netaudit/ReadinessBanner.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { HOME_BOARD_ID } from "../../constants/dashboard"
import { RADIO_DETAIL_LABEL } from "../../constants/radio"
import { VIEW_MODE_OPTIONS } from "../../constants/netaudit"
import type { DashboardBoard, DashboardCardDef } from "../../types/dashboard"
import type { NetauditViewMode } from "../../types/netaudit"

// What the cards of a stack share, shown above any board that holds one of them: Simple or Advanced (one remembered choice for Network and Radio), Help,
// and what is missing before a scan can run. It used to sit on the stack's own page, which no longer shows the cards.
const props = defineProps<{ board: DashboardBoard; defs: ReadonlyMap<string, DashboardCardDef> }>()
const { mode, set } = useNetauditViewMode()
const sources = computed(() => new Set(props.board.cards.map((c) => props.defs.get(c.id)?.source)))
const network = computed(() => sources.value.has("network"))
const radio = computed(() => sources.value.has("radio"))
</script>

<template>
  <div v-if="board.id !== HOME_BOARD_ID && (network || radio)" class="flex flex-col gap-3" data-test="stack-chrome">
    <div class="flex justify-end">
      <NetauditToolbar v-if="network" />
      <SegmentedControl v-else :model-value="mode" :options="VIEW_MODE_OPTIONS" :label="RADIO_DETAIL_LABEL" variant="compact" @update:model-value="set($event as NetauditViewMode)" />
    </div>
    <ReadinessBanner v-if="network" />
    <NetauditHelp v-if="network" />
  </div>
</template>
