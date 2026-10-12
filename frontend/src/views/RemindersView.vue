<script setup lang="ts">
import { computed, ref } from "vue"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import StatusTag from "../components/common/display/StatusTag.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import ReminderEditor from "../components/reminders/ReminderEditor.vue"
import { useReminders } from "../composables/reminders/useReminders"
import { useReminderChannels } from "../composables/reminders/useReminderChannels"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import { formatRelativeTime } from "../utils/format/time"
import { reminderEnds, reminderRepeat, reminderWhen } from "../utils/reminders/present"
import { REMINDER_EXAMPLE, REMINDER_OUTCOME_TAG, RELOAD_HINT } from "../constants/reminders"
import type { ReminderEdit, ReminderItem } from "../types/reminders"

// Reminders: everything the assistant has been asked to remind you of. Upcoming
// ones can be edited or deleted; finished ones (sent, missed, failed) stay in the
// history until deleted. New reminders come from asking the assistant.

const { reminders, loading, error, actionError, saving, save, remove } = useReminders()
const channels = useReminderChannels()
const { confirm } = useConfirm()
const toast = useToast()

const editingId = ref<number | null>(null)
const empty = computed(() => !reminders.value.open.length && !reminders.value.recent.length)

function startEdit(r: ReminderItem) {
  actionError.value = ""
  editingId.value = r.id
}

async function saveEdit(r: ReminderItem, edit: ReminderEdit) {
  if (await save(r.id, edit)) {
    editingId.value = null
    toast.success("Reminder saved")
  }
}

async function askDelete(r: ReminderItem) {
  const sure = await confirm({
    title: "Delete reminder",
    message: r.status === "scheduled" ? `"${r.message}" will not be sent.` : `Remove "${r.message}" from the history.`,
    type: "warning",
    confirmText: "Delete",
  })
  if (!sure) return
  if (await remove(r.id)) toast.success("Reminder deleted")
  else toast.error(actionError.value)
}

</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Assistant" title="Reminders" subtitle="Ask the assistant to set one; edit or delete them here" />

    <LoadingState v-if="loading" label="Loading reminders" :rows="3" />
    <ErrorState v-else-if="error" title="Reminders unavailable" :cause="error" :next="RELOAD_HINT" />
    <EmptyState v-else-if="empty" title="No reminders" :body="REMINDER_EXAMPLE" />
    <template v-else>
      <Panel v-if="reminders.open.length" title="Upcoming">
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Upcoming reminders">
          <li v-for="r in reminders.open" :key="r.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
            <ReminderEditor
              v-if="editingId === r.id"
              :reminder="r"
              :channels="channels"
              :saving="saving"
              :error="actionError"
              @save="saveEdit(r, $event)"
              @cancel="editingId = null"
            />
            <div v-else class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
              <div class="min-w-0">
                <p class="m-0 break-words text-primary">{{ r.message }}</p>
                <p class="m-0 font-mono text-[length:var(--text-small)] text-muted">
                  <span :title="reminderWhen(r)">{{ formatRelativeTime(r.fire_at) }}</span> · {{ reminderWhen(r) }} · {{ r.channel }}
                  <template v-if="r.cron"> · {{ reminderRepeat(r) }}<template v-if="r.until">, {{ reminderEnds(r) }}</template></template>
                </p>
              </div>
              <div class="flex gap-1">
                <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit reminder: ${r.message}`" @click="startEdit(r)" />
                <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete reminder: ${r.message}`" @click="askDelete(r)" />
              </div>
            </div>
          </li>
        </ul>
      </Panel>

      <Panel v-if="reminders.recent.length" title="History">
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Past reminders">
          <li v-for="r in reminders.recent" :key="r.id" class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 py-3 first:pt-0 last:pb-0">
            <div class="min-w-0">
              <p class="m-0 break-words text-muted">{{ r.message }}</p>
              <p class="m-0 font-mono text-[length:var(--text-small)] text-faint">{{ reminderWhen(r) }} · {{ r.channel }}</p>
              <p v-if="r.last_error" class="m-0 font-mono text-[length:var(--text-small)] text-state-error">{{ r.last_error }}</p>
            </div>
            <div class="flex items-center gap-2">
              <StatusTag v-if="REMINDER_OUTCOME_TAG[r.status]" v-bind="REMINDER_OUTCOME_TAG[r.status]!" />
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete reminder: ${r.message}`" @click="askDelete(r)" />
            </div>
          </li>
        </ul>
      </Panel>
    </template>
  </div>
</template>
