<script setup lang="ts">
import { onMounted } from "vue"
import { useRouter } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import Panel from "../common/layout/Panel.vue"
import { usePolling } from "../../composables/ui/usePolling"
import { useHostTraffic } from "../../composables/netaudit/useHostTraffic"
import { formatRelativeTime } from "../../utils/format/time"
import { toNetworkSettings } from "../../router/routes"
import { TRAFFIC_BUTTON, TRAFFIC_FOLLOW_INTERVAL_MS as FOLLOW_INTERVAL_MS, TRAFFIC_OPEN_SETTINGS, TRAFFIC_PASSPHRASE_NOTE, TRAFFIC_RUNNING, TRAFFIC_SUBTITLE, TRAFFIC_TITLE } from "../../constants/netaudit"

// Traffic · the look: where a look at this computer's traffic is taken (through the privileged helper). The button is there only when the helper can do
// it; when it cannot, the reason and the one page that fixes it are, and the last look stays readable. It makes no finding. What the look shows, by
// device, is the Traffic by device card below it.
const router = useRouter()
const { availability, summary, loaded, running, error, loadError, load, listen, follow } = useHostTraffic()
onMounted(() => void load())
// A look started before a reload runs on the server for up to a minute; check every few seconds whether it has ended.
usePolling(follow, FOLLOW_INTERVAL_MS)
</script>

<template>
  <LoadingState v-if="!loaded" label="Reading the traffic look" :rows="2" data-test="traffic-loading" />
  <Panel v-else :title="TRAFFIC_TITLE">
    <div class="flex flex-col gap-3">
      <p class="m-0 text-secondary">{{ TRAFFIC_SUBTITLE }}</p>
      <div v-if="availability.ok" class="flex flex-wrap items-center gap-3">
        <BaseButton :loading="running" :disabled="running" class-name="max-sm:!h-11" data-test="traffic-listen" @click="listen">{{ running ? TRAFFIC_RUNNING : TRAFFIC_BUTTON }}</BaseButton>
        <span class="text-[length:var(--text-small)] text-muted">{{ TRAFFIC_PASSPHRASE_NOTE }}</span>
      </div>
      <div v-else class="flex flex-wrap items-center gap-3" data-test="traffic-unavailable">
        <p class="m-0 min-w-0 break-words text-secondary">{{ availability.reason }}</p>
        <BaseButton v-if="availability.fixTab" variant="secondary" class-name="max-sm:!h-11" data-test="traffic-fix" @click="router.push(toNetworkSettings('helper'))">{{ TRAFFIC_OPEN_SETTINGS }}</BaseButton>
      </div>
      <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-error">{{ error }}</p>
      <p v-if="loadError" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-load-error">{{ loadError }}</p>
      <p v-if="summary" class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-when">{{ summary.interface }} · {{ formatRelativeTime(summary.at) }}</p>
    </div>
  </Panel>
</template>
