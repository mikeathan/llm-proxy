<script setup lang="ts">
import { computed } from "vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { reminderEnds, reminderRepeat, reminderWhen } from "../../utils/reminders/present"
import { REMINDER_OUTCOME_TAG, REMINDER_PAUSED_TAG } from "../../constants/reminders"
import type { ReminderItem } from "../../types/reminders"

// One reminder in a list, with the actions that fit its state: upcoming ones can
// be edited, deleted and (when repeating) paused, resumed or skipped; finished ones
// (sent, missed, failed) can be rescheduled or deleted.

// `readonly` shows the row without actions (the Overview summary).
const props = defineProps<{ reminder: ReminderItem; readonly?: boolean }>()
defineEmits<{
  (e: "edit"): void
  (e: "delete"): void
  (e: "pause"): void
  (e: "resume"): void
  (e: "skip"): void
  (e: "reschedule"): void
}>()

const r = computed(() => props.reminder)
const finished = computed(() => r.value.status in REMINDER_OUTCOME_TAG)
const paused = computed(() => r.value.status === "paused")
const repeating = computed(() => Boolean(r.value.cron))
const tag = computed(() => (paused.value ? REMINDER_PAUSED_TAG : REMINDER_OUTCOME_TAG[r.value.status]))
const label = (what: string) => `${what}: ${r.value.message}`
</script>

<template>
  <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
    <div class="min-w-0">
      <p :class="['m-0 break-words', finished ? 'text-muted' : 'text-primary']">{{ r.message }}</p>
      <p class="m-0 font-mono text-[length:var(--text-small)]" :class="finished || paused ? 'text-faint' : 'text-muted'">
        <template v-if="!finished"><span :title="reminderWhen(r)">{{ formatRelativeTime(r.fire_at) }}</span> · </template>
        {{ reminderWhen(r) }} · {{ r.channel }}
        <template v-if="repeating"> · {{ reminderRepeat(r) }}<template v-if="r.until">, {{ reminderEnds(r) }}</template></template>
      </p>
      <p v-if="r.last_error" class="m-0 font-mono text-[length:var(--text-small)] text-state-error">{{ r.last_error }}</p>
    </div>
    <div class="flex items-center gap-1">
      <StatusTag v-if="tag" v-bind="tag" class="mr-1" />
      <template v-if="readonly"></template>
      <template v-else-if="finished">
        <BaseButton variant="ghost" size="sm" icon="refresh" icon-only :label="label('Reschedule reminder')" @click="$emit('reschedule')" />
      </template>
      <template v-else>
        <BaseButton v-if="repeating && !paused" variant="ghost" size="sm" icon="pause" icon-only :label="label('Pause reminder')" @click="$emit('pause')" />
        <BaseButton v-if="paused" variant="ghost" size="sm" icon="play" icon-only :label="label('Resume reminder')" @click="$emit('resume')" />
        <BaseButton v-if="repeating && !paused" variant="ghost" size="sm" icon="skip" icon-only :label="label('Skip next occurrence')" @click="$emit('skip')" />
        <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="label('Edit reminder')" @click="$emit('edit')" />
      </template>
      <BaseButton v-if="!readonly" variant="ghost" size="sm" icon="trash" icon-only :label="label('Delete reminder')" @click="$emit('delete')" />
    </div>
  </div>
</template>
