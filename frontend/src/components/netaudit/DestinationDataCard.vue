<script setup lang="ts">
import { computed, onMounted } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import { useDestinationData } from "../../composables/netaudit/useDestinationData"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"
import {
  DESTINATION_DATA_DOWNLOAD, DESTINATION_DATA_HELD, DESTINATION_DATA_OLD, DESTINATION_DATA_OLD_DAYS, DESTINATION_DATA_REFRESH, DESTINATION_DATA_REMOVE, DESTINATION_TABLE_COPY,
} from "../../constants/netaudit"
import type { DestinationTable } from "../../types/netaudit"

// Network · Settings → Downloaded data: whether one table the traffic look names destinations with is held (who runs each address, or the optional table of cities), the one
// button that downloads it (the server asks for the passphrase, because it makes this computer ask an outside service) and one to remove it. The
// lookups never leave this computer.
const props = withDefaults(defineProps<{ table?: DestinationTable }>(), { table: "owners" })
const emit = defineEmits<{ (e: "changed"): void }>()
const copy = DESTINATION_TABLE_COPY[props.table]
const { state, error, busy, load, download, remove } = useDestinationData(props.table)
const MS_PER_DAY = 86_400_000
const DAYS_PER_MONTH = 30
// How old the held table is in months, once it is past the age that is worth a word; null for a fresh, missing or unrecorded one.
const oldMonths = computed<number | null>(() => {
  const at = state.value?.loaded_at
  if (!state.value || state.value.ranges === 0 || !at || !isRecorded(at)) return null
  const days = (Date.now() - new Date(at).getTime()) / MS_PER_DAY
  return days > DESTINATION_DATA_OLD_DAYS ? Math.round(days / DAYS_PER_MONTH) : null
})
const held = (ranges: number) => new Intl.NumberFormat("en").format(ranges)
async function press(action: () => Promise<boolean>) {
  if (await action()) emit("changed")
}
onMounted(load)
</script>

<template>
  <Panel v-if="state || error" :title="copy.title">
    <div class="flex flex-col gap-3" :data-test="copy.prefix">
      <p class="m-0 text-secondary">{{ copy.intro }}</p>
      <template v-if="state">
        <p class="m-0 text-primary" :data-test="`${copy.prefix}-line`">
          {{ state.ranges > 0 ? DESTINATION_DATA_HELD(held(state.ranges), state.loaded_at && isRecorded(state.loaded_at) ? formatRelativeTime(state.loaded_at) : "earlier") : copy.needs }}
        </p>
        <p v-if="state.ranges === 0" class="m-0 text-[length:var(--text-small)] text-muted" :data-test="`${copy.prefix}-source`">{{ copy.source(state.source) }}</p>
        <div class="flex flex-wrap items-center gap-2">
          <BaseButton :variant="state.ranges > 0 ? 'secondary' : 'primary'" size="sm" class-name="max-sm:!h-11" :loading="busy" :disabled="busy" :data-test="`${copy.prefix}-download`" @click="press(download)">{{ state.ranges > 0 ? DESTINATION_DATA_REFRESH : DESTINATION_DATA_DOWNLOAD }}</BaseButton>
          <BaseButton v-if="state.ranges > 0" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" :data-test="`${copy.prefix}-remove`" @click="press(remove)">{{ DESTINATION_DATA_REMOVE }}</BaseButton>
        </div>
        <p v-if="oldMonths" class="m-0 text-[length:var(--text-small)] text-state-running" :data-test="`${copy.prefix}-old`">{{ DESTINATION_DATA_OLD(oldMonths) }}</p>
        <p v-if="state.ranges > 0" class="m-0 text-[length:var(--text-small)] text-muted">{{ copy.accuracy }}</p>
        <p v-if="copy.credit" class="m-0 text-[length:var(--text-small)] text-muted" :data-test="`${copy.prefix}-credit`">{{ copy.credit }}</p>
      </template>
      <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" :data-test="`${copy.prefix}-error`">{{ error }}</p>
    </div>
  </Panel>
</template>
