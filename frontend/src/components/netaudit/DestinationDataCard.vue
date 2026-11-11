<script setup lang="ts">
import { onMounted } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import { useDestinationData } from "../../composables/netaudit/useDestinationData"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"
import {
  DESTINATION_DATA_ACCURACY, DESTINATION_DATA_DOWNLOAD, DESTINATION_DATA_HELD, DESTINATION_DATA_INTRO, DESTINATION_DATA_NEEDS, DESTINATION_DATA_REFRESH,
  DESTINATION_DATA_REMOVE, DESTINATION_DATA_SOURCE, DESTINATION_DATA_TITLE,
} from "../../constants/netaudit"

// Network · Traffic · Who runs these servers: whether the table of address owners is held, the one button that downloads it (the server asks for
// the passphrase, because it makes this computer ask an outside service) and one to remove it. The lookups never leave this computer.
const emit = defineEmits<{ (e: "changed"): void }>()
const { state, error, busy, load, download, remove } = useDestinationData()
const held = (ranges: number) => new Intl.NumberFormat("en").format(ranges)
async function press(action: () => Promise<boolean>) {
  if (await action()) emit("changed")
}
onMounted(load)
</script>

<template>
  <Panel v-if="state || error" :title="DESTINATION_DATA_TITLE">
    <div class="flex flex-col gap-3" data-test="destination-data">
      <p class="m-0 text-secondary">{{ DESTINATION_DATA_INTRO }}</p>
      <template v-if="state">
        <p class="m-0 text-primary" data-test="destination-data-line">
          {{ state.ranges > 0 ? DESTINATION_DATA_HELD(held(state.ranges), state.loaded_at && isRecorded(state.loaded_at) ? formatRelativeTime(state.loaded_at) : "earlier") : DESTINATION_DATA_NEEDS }}
        </p>
        <p v-if="state.ranges === 0" class="m-0 text-[length:var(--text-small)] text-muted" data-test="destination-data-source">{{ DESTINATION_DATA_SOURCE(state.source) }}</p>
        <div class="flex flex-wrap items-center gap-2">
          <BaseButton :variant="state.ranges > 0 ? 'secondary' : 'primary'" size="sm" class-name="max-sm:!h-11" :loading="busy" :disabled="busy" data-test="destination-data-download" @click="press(download)">{{ state.ranges > 0 ? DESTINATION_DATA_REFRESH : DESTINATION_DATA_DOWNLOAD }}</BaseButton>
          <BaseButton v-if="state.ranges > 0" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="destination-data-remove" @click="press(remove)">{{ DESTINATION_DATA_REMOVE }}</BaseButton>
        </div>
        <p v-if="state.ranges > 0" class="m-0 text-[length:var(--text-small)] text-muted">{{ DESTINATION_DATA_ACCURACY }}</p>
      </template>
      <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="destination-data-error">{{ error }}</p>
    </div>
  </Panel>
</template>
