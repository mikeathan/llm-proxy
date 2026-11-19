<script setup lang="ts">
import { onMounted, provide } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import Panel from "../common/layout/Panel.vue"
import StackBoardLink from "../stacks/StackBoardLink.vue"
import TrafficRouterSetup from "./TrafficRouterSetup.vue"
import TrafficTablesLine from "./TrafficTablesLine.vue"
import TrafficTraceCard from "./TrafficTraceCard.vue"
import { usePolling } from "../../composables/ui/usePolling"
import { useHostTraffic } from "../../composables/netaudit/useHostTraffic"
import { ROUTER_CONN_INJECTION, useRouterConnections } from "../../composables/netaudit/useRouterConnections"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { formatRelativeTime } from "../../utils/format/time"
import {
  ROUTER_PANEL_TITLE, TRAFFIC_BUTTON, TRAFFIC_OPEN_SETTINGS, TRAFFIC_PASSPHRASE_NOTE, TRAFFIC_RUNNING, TRAFFIC_SUBTITLE, TRAFFIC_TITLE,
  TRAFFIC_FOLLOW_INTERVAL_MS as FOLLOW_INTERVAL_MS, ROUTE_POLL_MS,
} from "../../constants/netaudit"
import type { NetworkTab } from "../../types/routes"

// Network · Traffic: where a look at this computer's traffic is taken (through the privileged helper), where the router is set up to be read, and where a
// route is traced. What the look and the router's table show, by device, is the dashboard's Traffic card; this page says when the last look was.
// The button is there only when the helper can do it; when it cannot, the reason and the one page that fixes it are, and the last look stays readable.
// It makes no finding.
const emit = defineEmits<{ (e: "open-tab", tab: NetworkTab): void }>()
const { availability, summary, loaded, running, error, loadError, load, listen, follow } = useHostTraffic()
// The route traces are kept by the server; one running fills in step by step, so it is read every second while it runs, and not otherwise.
const routes = useRouteTrace()
// One reading of the router, shared by the setup that changes it.
provide(ROUTER_CONN_INJECTION, useRouterConnections())
onMounted(() => { void load(); void routes.load() })
// A look started before a reload runs on the server for up to a minute; check every few seconds whether it has ended.
usePolling(follow, FOLLOW_INTERVAL_MS)
usePolling(() => void routes.refresh(), ROUTE_POLL_MS)

</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-tab">
    <StackBoardLink stack="traffic" />
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
          <p v-if="summary" class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-when">{{ summary.interface }} · {{ formatRelativeTime(summary.at) }}</p>
        </div>
      </Panel>

      <Panel :title="ROUTER_PANEL_TITLE">
        <TrafficRouterSetup />
      </Panel>

      <TrafficTraceCard />

      <!-- The tables that name internet addresses are managed in Settings; here only whether they are on, and a link. -->
      <TrafficTablesLine />
    </template>
  </div>
</template>
