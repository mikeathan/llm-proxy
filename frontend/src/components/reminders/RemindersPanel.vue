<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import StatusTag from "../common/display/StatusTag.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import { useReminders } from "../../composables/reminders/useReminders"
import { usePolling } from "../../composables/ui/usePolling"
import { formatAbsoluteTime, formatRelativeTime } from "../../utils/format/time"
import { RECENT_REMINDERS_SHOWN, REMINDERS_POLL_MS, REMINDER_EXAMPLE, REMINDER_OUTCOME_TAG, RELOAD_HINT } from "../../constants/reminders"

// Overview · Reminders: what is coming up, and what happened to the last few
// (sent, missed while the service was off, or failed to deliver).

const { reminders, loading, error, load } = useReminders()
onMounted(load)
usePolling(load, REMINDERS_POLL_MS)

const recent = computed(() => reminders.value.recent.slice(0, RECENT_REMINDERS_SHOWN))
const empty = computed(() => !reminders.value.open.length && !recent.value.length)
</script>

<template>
  <Panel title="Reminders">
    <LoadingState v-if="loading" label="Loading reminders" :rows="2" />
    <ErrorState v-else-if="error" title="Reminders unavailable" :cause="error" :next="RELOAD_HINT" />
    <EmptyState v-else-if="empty" title="No reminders" :body="REMINDER_EXAMPLE" />
    <div v-else class="flex flex-col gap-4">
      <ul v-if="reminders.open.length" class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Upcoming reminders">
        <li v-for="r in reminders.open" :key="r.id" class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <span class="min-w-0 break-words text-primary">{{ r.message }}</span>
          <span class="font-mono text-[length:var(--text-small)] text-muted" :title="formatAbsoluteTime(r.fire_at, undefined, r.timezone)">
            {{ formatRelativeTime(r.fire_at) }} · {{ r.channel }}<template v-if="r.cron"> · repeats</template>
          </span>
        </li>
      </ul>
      <ul v-if="recent.length" class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Recent reminders">
        <li v-for="r in recent" :key="r.id" class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <span class="min-w-0 break-words text-muted">{{ r.message }}</span>
          <span class="flex items-baseline gap-2">
            <span v-if="r.last_error" class="font-mono text-[length:var(--text-small)] text-state-error">{{ r.last_error }}</span>
            <StatusTag v-if="REMINDER_OUTCOME_TAG[r.status]" v-bind="REMINDER_OUTCOME_TAG[r.status]!" />
          </span>
        </li>
      </ul>
    </div>
  </Panel>
</template>
