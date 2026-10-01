<script setup lang="ts">
import { computed, ref, useId, watch } from "vue"
import cronstrue from "cronstrue"
import FormField from "../../common/forms/FormField.vue"

// A cron schedule: a simple "every N minutes/hours/days" builder, or a custom
// expression, always with a plain-English reading of the result.

const props = defineProps<{
  modelValue: string
  triggerType: string
}>()

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void
}>()

type CronMode = "every" | "custom"
type CronUnit = "minutes" | "hours" | "days"
const EXPRESSION: Record<CronUnit, (n: number) => string> = {
  minutes: (n) => `*/${n} * * * *`,
  hours: (n) => `0 */${n} * * *`,
  days: (n) => `0 0 */${n} * *`,
}

const cronType = ref<CronMode>("custom")
const cronEvery = ref(1)
const cronUnit = ref<CronUnit>("hours")
const modeId = useId()

watch([cronType, cronEvery, cronUnit], () => {
  if (cronType.value === "custom") return
  emit("update:modelValue", EXPRESSION[cronUnit.value](cronEvery.value))
})

watch(
  () => props.triggerType,
  (type) => {
    if (type === "cron") cronType.value = "custom"
  },
)

const description = computed(() => {
  if (props.triggerType !== "cron" || !props.modelValue) return ""
  try {
    return cronstrue.toString(props.modelValue)
  } catch {
    return "Not a valid cron expression"
  }
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-wrap items-end gap-2">
      <label :for="modeId" class="sr-only">Schedule style</label>
      <select :id="modeId" v-model="cronType" class="form-control max-w-[16rem]">
        <option value="every">Simple frequency</option>
        <option value="custom">Custom expression</option>
      </select>
      <template v-if="cronType === 'every'">
        <span class="font-mono text-[length:var(--text-small)] text-muted">every</span>
        <input v-model.number="cronEvery" type="number" min="1" aria-label="Every how many" class="form-control w-20" />
        <select v-model="cronUnit" aria-label="Unit" class="form-control w-32">
          <option value="minutes">minutes</option>
          <option value="hours">hours</option>
          <option value="days">days</option>
        </select>
      </template>
    </div>
    <FormField label="Cron expression" :hint="description || 'Five fields: minute hour day month weekday, e.g. 0 7 * * *'">
      <template #default="{ id, describedBy }">
        <input
          :id="id"
          :value="modelValue"
          :aria-describedby="describedBy"
          :readonly="cronType !== 'custom'"
          placeholder="0 7 * * *"
          class="form-control font-mono text-[length:var(--text-small)]"
          @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        />
      </template>
    </FormField>
  </div>
</template>
