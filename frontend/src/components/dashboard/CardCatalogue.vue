<script setup lang="ts">
import { computed } from "vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { ADD_CARD_LABEL, ALL_CARDS_ADDED, SOURCE_LABELS, SOURCE_ORDER } from "../../constants/dashboard"
import type { DashboardCardDef, DashboardSource } from "../../types/dashboard"

// Every card the screen can draw, grouped by where it comes from. A card already on the open board says so. Adding puts the card at the
// end of the board at its own default size.
const props = defineProps<{ defs: readonly DashboardCardDef[]; onBoard: ReadonlySet<string> }>()
const emit = defineEmits<{ (e: "add", def: DashboardCardDef): void; (e: "close"): void }>()

const groups = computed(() =>
  SOURCE_ORDER.map((source: DashboardSource) => ({ source, defs: props.defs.filter((d) => d.source === source) })).filter((g) => g.defs.length),
)
const allOnBoard = computed(() => props.defs.every((d) => props.onBoard.has(d.id)))
</script>

<template>
  <PopoverPanel id="card-catalogue" :title="ADD_CARD_LABEL" width="sm:w-[380px]" @close="emit('close')">
    <div class="flex max-h-[60vh] flex-col gap-3 overflow-y-auto p-3">
      <p v-if="allOnBoard" class="m-0 text-muted" data-test="catalogue-all">{{ ALL_CARDS_ADDED }}</p>
      <section v-for="g in groups" :key="g.source" :aria-label="SOURCE_LABELS[g.source]" class="flex flex-col gap-1">
        <MicroLabel>{{ SOURCE_LABELS[g.source] }}</MicroLabel>
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="d in g.defs" :key="d.id" class="flex items-center justify-between gap-2 py-1" :data-test="`catalogue-${d.id}`">
            <span class="min-w-0 flex-1 text-primary">{{ d.title }}</span>
            <span v-if="onBoard.has(d.id)" class="text-[length:var(--text-small)] text-muted">On this board</span>
            <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('add', d)">Add</BaseButton>
          </li>
        </ul>
      </section>
    </div>
  </PopoverPanel>
</template>
