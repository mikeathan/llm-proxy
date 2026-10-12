<script setup lang="ts">
import { computed, ref } from "vue"
import FormField from "../common/forms/FormField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SchedulePicker from "../AgentIde/automation/SchedulePicker.vue"
import { fromZonedInput, toZonedInput } from "../../utils/format/timezone"
import { REMINDER_REPEAT_KINDS, REMINDER_UNTIL_TIME } from "../../constants/reminders"
import type { ReminderEdit, ReminderItem } from "../../types/reminders"
import type { ChoiceOption } from "../../types/ui"

// Edit one open reminder: what it says, when (or how often) and where it goes.
// Times are shown and typed in the reminder's own timezone. The owner saves the
// result and shows the server's reason in `error` when it refuses.

const props = defineProps<{ reminder: ReminderItem; channels: ChoiceOption[]; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: ReminderEdit): void; (e: "cancel"): void }>()

const DATE_LENGTH = "YYYY-MM-DD".length
const zone = props.reminder.timezone
const recurring = Boolean(props.reminder.cron)

const message = ref(props.reminder.message)
const when = ref(toZonedInput(props.reminder.fire_at, zone))
const cron = ref(props.reminder.cron ?? "")
const endDate = ref(props.reminder.until ? toZonedInput(props.reminder.until, zone).slice(0, DATE_LENGTH) : "")
const connector = ref(props.reminder.connector)

const canSave = computed(
  () => !props.saving && message.value.trim() !== "" && (recurring ? cron.value !== "" : fromZonedInput(when.value, zone) !== null),
)

function submit() {
  if (!canSave.value) return
  const edit: ReminderEdit = { message: message.value.trim(), connector: connector.value }
  if (recurring) {
    edit.cron = cron.value
    const until = endDate.value ? fromZonedInput(`${endDate.value}T${REMINDER_UNTIL_TIME}`, zone) : null
    if (until) edit.until = until
  } else {
    edit.fire_at = fromZonedInput(when.value, zone) ?? undefined
  }
  emit("save", edit)
}
</script>

<template>
  <form class="flex flex-col gap-3" @submit.prevent="submit">
    <FormField label="Message">
      <template #default="{ id }">
        <textarea :id="id" v-model="message" rows="2" maxlength="2000" class="form-control"></textarea>
      </template>
    </FormField>

    <FormField v-if="!recurring" label="When" :hint="`Times are in ${zone}.`">
      <template #default="{ id, describedBy }">
        <input :id="id" v-model="when" :aria-describedby="describedBy" type="datetime-local" required class="form-control w-fit font-mono" />
      </template>
    </FormField>

    <template v-else>
      <SchedulePicker type="cron" :value="cron" :kinds="[...REMINDER_REPEAT_KINDS]" :preview="false" label="Repeats" @update:trigger="cron = $event.value" />
      <FormField label="Ends" :hint="`Last day it can fire, in ${zone}. Leave empty to repeat until you delete it.`">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="endDate" :aria-describedby="describedBy" type="date" class="form-control w-fit font-mono" />
        </template>
      </FormField>
    </template>

    <FormField label="Channel">
      <template #default="{ id }">
        <SelectInput :id="id" v-model="connector" :options="channels" label="Channel" />
      </template>
    </FormField>

    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>

    <div class="flex flex-wrap gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="secondary" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
