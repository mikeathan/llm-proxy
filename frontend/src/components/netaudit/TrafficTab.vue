<script setup lang="ts">
import { onMounted } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import Panel from "../common/layout/Panel.vue"
import TrafficOtherDevices from "./TrafficOtherDevices.vue"
import TrafficReport from "./TrafficReport.vue"
import TrafficTablesLine from "./TrafficTablesLine.vue"
import TrafficTraceCard from "./TrafficTraceCard.vue"
import { usePolling } from "../../composables/ui/usePolling"
import { useHostTraffic } from "../../composables/netaudit/useHostTraffic"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { formatRelativeTime } from "../../utils/format/time"
import {
  TRAFFIC_BUTTON, TRAFFIC_NEVER_BODY, TRAFFIC_NEVER_TITLE, TRAFFIC_OPEN_SETTINGS, TRAFFIC_PASSPHRASE_NOTE, TRAFFIC_RUNNING, TRAFFIC_SUBTITLE, TRAFFIC_TITLE,
  TRAFFIC_FOLLOW_INTERVAL_MS as FOLLOW_INTERVAL_MS, ROUTE_POLL_MS,
} from "../../constants/netaudit"
import type { NetworkTab } from "../../types/routes"

// Network · Traffic: a look at this computer's own traffic for up to a minute, through the privileged helper. The button is there only when
// the helper can do it; when it cannot, the reason and the one page that fixes it are, and the last look stays readable. It makes no finding.
const emit = defineEmits<{ (e: "open-tab", tab: NetworkTab): void }>()
const { availability, summary, loaded, running, error, loadError, load, listen, follow } = useHostTraffic()
// The route traces are kept by the server; one running fills in step by step, so it is read every second while it runs, and not otherwise.
const routes = useRouteTrace()
onMounted(() => { void load(); void routes.load() })
// A look started before a reload runs on the server for up to a minute; check every few seconds whether it has ended.
usePolling(follow, FOLLOW_INTERVAL_MS)
usePolling(() => void routes.refresh(), ROUTE_POLL_MS)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-tab">
    <LoadingState v-if="!loaded" label="Reading the traffic look" :rows="3" data-test="traffic-loading" />
    <template v-else>
      <Panel :title="TRAFFIC_TITLE">
        <div class="flex flex-col gap-3">
          <p class="m-0 text-secondary">{{ TRAFFIC_SUBTITLE }}</p>
          <div v-if="availability.ok" class="flex flex-wrap items-center gap-3">
            <BaseButton :loading="running" :disabled="running" class-name="max-sm:!h-11" data-test="traffic-listen" @click="listen">{{ running ? TRAFFIC_RUNNING : TRAFFIC_BUTTON }}</BaseButton>
            <span class="text-[length:var(--text-small)] text-muted">{{ TRAFFIC_PASSPHRASE_NOTE }}</span>
          </div>
          <div v-else class="flex flex-wrap items-center gap-3" data-test="traffic-unavailable">
            <p class="m-0 min-w-0 break-words text-secondary">{{ availability.reason }}</p>
            <BaseButton v-if="availability.fixTab" variant="secondary" class-name="max-sm:!h-11" data-test="traffic-fix" @click="emit('open-tab', availability.fixTab)">{{ TRAFFIC_OPEN_SETTINGS }}</BaseButton>
          </div>
          <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-error">{{ error }}</p>
          <p v-if="loadError" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-load-error">{{ loadError }}</p>
        </div>
      </Panel>

      <TrafficTraceCard />

      <template v-if="summary">
        <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-when">{{ summary.interface }} · {{ formatRelativeTime(summary.at) }}</p>
        <TrafficReport :traffic="summary" />
      </template>
      <EmptyState v-else-if="!loadError" :title="TRAFFIC_NEVER_TITLE" :body="TRAFFIC_NEVER_BODY" data-test="traffic-never" />

      <TrafficOtherDevices />

      <!-- The tables that name internet addresses are managed in Settings; here only whether they are on, and a link. -->
      <TrafficTablesLine />
    </template>
  </div>
</template>
