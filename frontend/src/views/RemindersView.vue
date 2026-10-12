<script setup lang="ts">
import { computed, ref } from "vue"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import ReminderEditor from "../components/reminders/ReminderEditor.vue"
import ReminderRow from "../components/reminders/ReminderRow.vue"
import { useReminders } from "../composables/reminders/useReminders"
import { useReminderChannels } from "../composables/reminders/useReminderChannels"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import { browserTimeZone, defaultReminderTime } from "../utils/format/timezone"
import { REMINDER_EXAMPLE, RELOAD_HINT } from "../constants/reminders"
import type { ReminderEdit, ReminderItem } from "../types/reminders"

// Reminders: everything the assistant has been asked to remind you of, and what
// you set here. Upcoming ones can be edited, deleted, and (repeating ones) paused
// or skipped; finished ones (sent, missed, failed) stay in the history until
// deleted, and can be rescheduled.

const NEW = "new" as const

const { reminders, loading, error, actionError, saving, create, save, remove, pause, resume, skip } = useReminders()
const channels = useReminderChannels()
const { confirm } = useConfirm()
const toast = useToast()

// Which row's form is open: a reminder id, or NEW for the "New reminder" form.
const editing = ref<number | typeof NEW | null>(null)
const empty = computed(() => !reminders.value.open.length && !reminders.value.recent.length)

function blankReminder(): ReminderItem {
  const zone = browserTimeZone()
  return {
    id: 0, message: "", fire_at: defaultReminderTime(new Date(), zone), timezone: zone,
    connector: "", channel: "", status: "scheduled",
  }
}

function open(what: number | typeof NEW) {
  actionError.value = ""
  editing.value = what
}

async function submit(target: number | typeof NEW, edit: ReminderEdit) {
  const saved = target === NEW ? await create(edit) : await save(target, edit)
  if (!saved) return
  editing.value = null
  toast.success(target === NEW ? "Reminder created" : "Reminder saved")
}

async function askDelete(r: ReminderItem) {
  const sure = await confirm({
    title: "Delete reminder",
    message: r.status === "done" || r.status === "missed" || r.status === "failed" ? `Remove "${r.message}" from the history.` : `"${r.message}" will not be sent.`,
    type: "warning",
    confirmText: "Delete",
  })
  if (!sure) return
  if (await remove(r.id)) toast.success("Reminder deleted")
  else toast.error(actionError.value)
}

// Pause, resume and skip report a refusal in the page (not a form), where the user is looking.
const act = (run: (id: number) => Promise<boolean>, r: ReminderItem) => {
  actionError.value = ""
  return run(r.id)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Assistant" title="Reminders" subtitle="Ask the assistant to set one, or add one here; edit or delete them here">
      <template #actions>
        <BaseButton size="sm" icon="plus" :disabled="editing === NEW" @click="open(NEW)">New reminder</BaseButton>
      </template>
    </PageHeader>

    <Panel v-if="editing === NEW" title="New reminder">
      <ReminderEditor :reminder="blankReminder()" :channels="channels" :saving="saving" :error="actionError" creating @save="submit(NEW, $event)" @cancel="editing = null" />
    </Panel>

    <LoadingState v-if="loading" label="Loading reminders" :rows="3" />
    <ErrorState v-else-if="error" title="Reminders unavailable" :cause="error" :next="RELOAD_HINT" />
    <EmptyState v-else-if="empty && editing !== NEW" title="No reminders" :body="REMINDER_EXAMPLE" />
    <template v-else>
      <p v-if="actionError && editing === null" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>

      <Panel v-if="reminders.open.length" title="Upcoming">
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Upcoming reminders">
          <li v-for="r in reminders.open" :key="r.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
            <ReminderEditor v-if="editing === r.id" :reminder="r" :channels="channels" :saving="saving" :error="actionError" @save="submit(r.id, $event)" @cancel="editing = null" />
            <ReminderRow
              v-else
              :reminder="r"
              @edit="open(r.id)"
              @delete="askDelete(r)"
              @pause="act(pause, r)"
              @resume="act(resume, r)"
              @skip="act(skip, r)"
            />
          </li>
        </ul>
      </Panel>

      <Panel v-if="reminders.recent.length" title="History">
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Past reminders">
          <li v-for="r in reminders.recent" :key="r.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
            <ReminderEditor v-if="editing === r.id" :reminder="r" :channels="channels" :saving="saving" :error="actionError" reschedule @save="submit(r.id, $event)" @cancel="editing = null" />
            <ReminderRow v-else :reminder="r" @reschedule="open(r.id)" @delete="askDelete(r)" />
          </li>
        </ul>
      </Panel>
    </template>
  </div>
</template>
