<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import ReminderRow from "./ReminderRow.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import { toReminders } from "../../router/routes"
import { useReminders } from "../../composables/reminders/useReminders"
import { RECENT_REMINDERS_SHOWN, REMINDER_EXAMPLE, RELOAD_HINT } from "../../constants/reminders"

// Overview · Reminders: what is coming up, and what happened to the last few
// (sent, missed while the service was off, or failed to deliver).

const { reminders, loading, error } = useReminders()

// What is coming up, then the last few outcomes.
const shown = computed(() => [...reminders.value.open, ...reminders.value.recent.slice(0, RECENT_REMINDERS_SHOWN)])
const empty = computed(() => !shown.value.length)
</script>

<template>
  <Panel title="Reminders">
    <template #actions>
      <RouterLink :to="toReminders()" class="font-mono text-[length:var(--text-small)] text-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2">Manage reminders →</RouterLink>
    </template>
    <LoadingState v-if="loading" label="Loading reminders" :rows="2" />
    <ErrorState v-else-if="error" title="Reminders unavailable" :cause="error" :next="RELOAD_HINT" />
    <EmptyState v-else-if="empty" title="No reminders" :body="REMINDER_EXAMPLE" />
    <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Reminders">
      <li v-for="r in shown" :key="r.id" class="py-2 first:pt-0 last:pb-0">
        <ReminderRow :reminder="r" readonly />
      </li>
    </ul>
  </Panel>
</template>
