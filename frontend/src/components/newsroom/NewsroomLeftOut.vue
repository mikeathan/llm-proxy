<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import NewsroomEntryItem from "./NewsroomEntryItem.vue"
import { dayEntries } from "../../utils/newsroom/dayEntries"
import { KEEP_LABEL, LEFT_OUT_EMPTY, LEFT_OUT_HINT } from "../../constants/newsroom"
import type { NewsroomDay, NewsroomWatch } from "../../types/newsroom"

// What the model left out of the alerts, newest first, each row with the model's reason and one action: "Should have matched" puts it in Alerts. It shows
// and reports; the page does the change.
const props = defineProps<{ days: NewsroomDay[]; watches: NewsroomWatch[]; busy: boolean }>()
const emit = defineEmits<{ (e: "keep", id: number): void }>()
const entries = computed(() => props.days.flatMap(dayEntries))
const manyWatches = computed(() => new Set(entries.value.map((e) => e.watch_id)).size > 1)
const nameOf = (watchId: number): string | undefined => (manyWatches.value ? props.watches.find((t) => t.id === watchId)?.name : undefined)
</script>

<template>
  <div class="flex flex-col gap-2" data-test="left-out">
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ LEFT_OUT_HINT }}</p>
    <p v-if="!entries.length" class="m-0 text-muted">{{ LEFT_OUT_EMPTY }}</p>
    <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
      <li v-for="entry in entries" :key="entry.id" class="flex items-start gap-2">
        <div class="min-w-0 flex-1"><NewsroomEntryItem :entry="{ ...entry, read: true }" :watch-name="nameOf(entry.watch_id)" /></div>
        <BaseButton size="sm" variant="ghost" class-name="mt-2 flex-none" :disabled="busy" :data-test="`keep-${entry.id}`" @click="emit('keep', entry.id)">{{ KEEP_LABEL }}</BaseButton>
      </li>
    </ul>
  </div>
</template>
