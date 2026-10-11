<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { TriggerType } from "../../../types/automation"
import type { EveryUnit, Schedule, ScheduleKind, Weekday } from "../../../types/schedule"
import { MAX_MONTH_DAY, describeSchedule, fromTrigger, toTrigger } from "../../../utils/automation/schedule"
import { formatRunTime } from "../../../utils/automation/runTime"
import { useSchedulePreview } from "../../../composables/automation/useSchedulePreview"
import FormField from "../../common/forms/FormField.vue"

// When an automation runs, chosen the way a person says it ("every weekday at
// 7:00") instead of written as cron. The picker owns the type/value pair the
// backend stores; a schedule it has no preset for stays as typed under
// "Custom". The next runs come from the server, in the server's own time zone.

const props = defineProps<{ type: TriggerType; value: string }>()
const emit = defineEmits<{
  (e: "update:trigger", trigger: { type: TriggerType; value: string }): void
  // The server's reason when it says the schedule is not valid; empty otherwise, so the owner can hold back a save.
  (e: "update:error", message: string): void
}>()

const DEFAULT_TIME = "07:00"
const DEFAULT_EVERY = 1
const KIND_OPTIONS: { value: ScheduleKind; label: string }[] = [
  { value: "manual", label: "Manual only" },
  { value: "every", label: "Every few minutes or hours" },
  { value: "daily", label: "Every day" },
  { value: "weekdays", label: "Every weekday" },
  { value: "weekly", label: "Certain days of the week" },
  { value: "monthly", label: "Once a month" },
  { value: "custom", label: "Custom (cron)" },
]
const UNIT_OPTIONS: { value: EveryUnit; label: string }[] = [
  { value: "minutes", label: "minutes" },
  { value: "hours", label: "hours" },
]
// Monday first, as a calendar reads; values are cron weekdays.
const WEEK: { day: Weekday; short: string; name: string }[] = [
  { day: 1, short: "Mon", name: "Monday" },
  { day: 2, short: "Tue", name: "Tuesday" },
  { day: 3, short: "Wed", name: "Wednesday" },
  { day: 4, short: "Thu", name: "Thursday" },
  { day: 5, short: "Fri", name: "Friday" },
  { day: 6, short: "Sat", name: "Saturday" },
  { day: 0, short: "Sun", name: "Sunday" },
]
const MONTH_DAYS = Array.from({ length: MAX_MONTH_DAY }, (_, i) => i + 1)
const CRON_FIELD = { label: "Cron expression", hint: "Five fields: minute hour day month weekday, e.g. 0 7 * * 1-5.", placeholder: "0 7 * * 1-5" }
const INTERVAL_FIELD = { label: "Interval", hint: "A duration such as 30s or 1h30m.", placeholder: "30s" }

const schedule = ref<Schedule>(fromTrigger(props.type, props.value))

const trigger = computed(() => toTrigger(schedule.value))

// The owner changes the trigger when another automation is opened for edit.
// An echo of what this picker just wrote is already what it shows.
watch(
  () => [props.type, props.value] as const,
  ([type, value]) => {
    if (trigger.value.type !== type || trigger.value.value !== value) schedule.value = fromTrigger(type, value)
  },
)

function update(next: Schedule) {
  schedule.value = next
  emit("update:trigger", toTrigger(next))
}

const timeOf = (s: Schedule): string => ("time" in s ? s.time : DEFAULT_TIME)

function scheduleOfKind(kind: ScheduleKind): Schedule {
  const time = timeOf(schedule.value)
  switch (kind) {
    case "manual":
      return { kind }
    case "every":
      return { kind, n: DEFAULT_EVERY, unit: "hours" }
    case "daily":
    case "weekdays":
      return { kind, time }
    case "weekly":
      return { kind, days: [1], time }
    case "monthly":
      return { kind, day: 1, time }
    case "custom": {
      // What the current schedule wrote becomes the text to edit.
      const { type, value } = trigger.value
      return { kind, type: type === "interval" ? "interval" : "cron", value }
    }
  }
}

const kind = computed({
  get: () => schedule.value.kind,
  set: (next: ScheduleKind) => update(scheduleOfKind(next)),
})

function patch(change: Partial<Schedule>) {
  update({ ...schedule.value, ...change } as Schedule)
}

const asEvery = computed(() => (schedule.value.kind === "every" ? schedule.value : null))
const asWeekly = computed(() => (schedule.value.kind === "weekly" ? schedule.value : null))
const asMonthly = computed(() => (schedule.value.kind === "monthly" ? schedule.value : null))
const asCustom = computed(() => (schedule.value.kind === "custom" ? schedule.value : null))
const hasTime = computed(() => ["daily", "weekdays", "weekly", "monthly"].includes(schedule.value.kind))

function toggleDay(day: Weekday, on: boolean) {
  const current = asWeekly.value?.days ?? []
  patch({ days: on ? [...current, day] : current.filter((d) => d !== day) } as Partial<Schedule>)
}

const { preview, failed } = useSchedulePreview(() => trigger.value)
const INVALID_FALLBACK = "The schedule is not valid."
watch(
  () => (preview.value && !preview.value.valid ? preview.value.error || INVALID_FALLBACK : ""),
  (message) => emit("update:error", message),
  { immediate: true },
)
const customField = computed(() => (asCustom.value?.type === "interval" ? INTERVAL_FIELD : CRON_FIELD))
const description = computed(() => describeSchedule(schedule.value))
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-wrap items-end gap-3">
      <FormField label="Runs">
        <template #default="{ id }">
          <select :id="id" v-model="kind" data-test="kind" class="form-control min-w-[16rem]">
            <option v-for="opt in KIND_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </template>
      </FormField>

      <template v-if="asEvery">
        <FormField label="Every">
          <template #default="{ id }">
            <input
              :id="id"
              :value="asEvery.n"
              type="number"
              min="1"
              class="form-control w-24"
              @input="patch({ n: ($event.target as HTMLInputElement).valueAsNumber } as Partial<Schedule>)"
            />
          </template>
        </FormField>
        <FormField label="Unit">
          <template #default="{ id }">
            <select :id="id" :value="asEvery.unit" data-test="unit" class="form-control w-32" @change="patch({ unit: ($event.target as HTMLSelectElement).value } as Partial<Schedule>)">
              <option v-for="opt in UNIT_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </template>
        </FormField>
      </template>

      <FormField v-if="asMonthly" label="Day of the month" :hint="`Up to the ${MAX_MONTH_DAY}th, so every month has it.`">
        <template #default="{ id, describedBy }">
          <select :id="id" :value="asMonthly.day" :aria-describedby="describedBy" data-test="month-day" class="form-control w-24" @change="patch({ day: Number(($event.target as HTMLSelectElement).value) } as Partial<Schedule>)">
            <option v-for="d in MONTH_DAYS" :key="d" :value="d">{{ d }}</option>
          </select>
        </template>
      </FormField>

      <FormField v-if="hasTime" label="At">
        <template #default="{ id }">
          <input :id="id" :value="'time' in schedule ? schedule.time : ''" type="time" required class="form-control w-32" @input="patch({ time: ($event.target as HTMLInputElement).value } as Partial<Schedule>)" />
        </template>
      </FormField>
    </div>

    <fieldset v-if="asWeekly" class="m-0 flex min-w-0 flex-wrap gap-2 border-0 p-0">
      <legend class="mb-1.5 p-0 text-[length:var(--text-small)] font-medium text-secondary">On</legend>
      <label
        v-for="d in WEEK"
        :key="d.day"
        class="inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-[var(--radius-md)] border border-control px-2.5 font-mono text-[length:var(--text-small)] text-secondary"
      >
        <input type="checkbox" :aria-label="d.name" :checked="asWeekly.days.includes(d.day)" @change="toggleDay(d.day, ($event.target as HTMLInputElement).checked)" />
        {{ d.short }}
      </label>
    </fieldset>

    <FormField v-if="asCustom" :label="customField.label" :hint="customField.hint">
      <template #default="{ id, describedBy }">
        <input
          :id="id"
          :value="asCustom.value"
          :aria-describedby="describedBy"
          data-test="custom"
            :placeholder="customField.placeholder"
          class="form-control font-mono text-[length:var(--text-small)]"
          @input="patch({ value: ($event.target as HTMLInputElement).value } as Partial<Schedule>)"
        />
      </template>
    </FormField>

    <div data-test="readout" class="flex flex-col gap-1 text-[length:var(--text-small)]">
      <p class="m-0 text-secondary">{{ description }}</p>
      <p v-if="preview && !preview.valid" role="alert" class="m-0 text-state-error">{{ preview.error }}</p>
      <template v-else-if="preview && preview.next_runs.length">
        <p class="m-0 text-muted">Next: {{ preview.next_runs.map(formatRunTime).join(" · ") }}</p>
        <p class="m-0 text-faint">Times are in server time, {{ preview.timezone }}.</p>
      </template>
      <p v-else-if="failed" class="m-0 text-faint">The schedule could not be checked with the server.</p>
    </div>
  </div>
</template>
