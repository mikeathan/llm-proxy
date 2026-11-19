<script setup lang="ts">
import { computed, onMounted } from "vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useAgentActivity } from "../../../composables/security/useAgentActivity"
import { usePolling } from "../../../composables/ui/usePolling"
import { agentEventSource, agentEventText } from "../../../utils/security/agentWatch"
import { formatRelativeTime } from "../../../utils/format/time"
import { AGENT_ACTIVITY_EMPTY, AGENTS_CARD_LOCKDOWN_OFF, LOCKDOWN_ON_TAG } from "../../../constants/security"

// Dashboard card · Agent activity: whether lock down is on, and what the agents did lately, newest first. It only reads; the lock down switch is
// on the Agents page. Small is the lock down state and the latest event; medium a short list; large the same list with where each came from.
const SHOWN = { xs: 1, sm: 4, lg: 10 } as const
const REFRESH_MS = 30000
const size = useCardSize()
const a = useAgentActivity()
const events = computed(() => (a.record.value?.events ?? []).slice(0, SHOWN[size.value]))
onMounted(() => { void a.load() })
usePolling(() => { void a.load() }, REFRESH_MS)
</script>

<template>
  <div class="flex flex-col gap-3" data-test="agents-activity-card">
    <ErrorState v-if="a.error.value && !a.record.value" title="The agent activity could not be read" :cause="a.error.value" next="It is read again in a minute." />
    <LoadingState v-else-if="!a.record.value" label="Reading the agent activity" :rows="2" />
    <template v-else>
      <p class="m-0 flex flex-wrap items-center gap-2 text-primary" data-test="agents-lockdown">
        <template v-if="a.lockdown.value?.on">Lock down <StatusTag state="error" :label="LOCKDOWN_ON_TAG" /></template>
        <template v-else>{{ AGENTS_CARD_LOCKDOWN_OFF }}</template>
      </p>
      <p v-if="!events.length" class="m-0 text-muted" data-test="agents-empty">{{ AGENT_ACTIVITY_EMPTY }}</p>
      <ul v-else class="m-0 flex list-none flex-col p-0" aria-label="Recent agent activity">
        <li v-for="(e, i) in events" :key="`${e.at}-${i}`" class="flex flex-col gap-0.5 border-b border-hairline/60 py-2 last:border-b-0" data-test="agents-event">
          <span class="break-words text-primary">{{ agentEventText(e) }}</span>
          <span class="flex flex-wrap gap-x-2 text-[length:var(--text-small)] text-muted">
            <span v-if="size === 'lg'" class="break-words">{{ agentEventSource(e) }}</span>
            <time :datetime="e.at">{{ formatRelativeTime(e.at) }}</time>
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>
