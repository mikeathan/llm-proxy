<script setup lang="ts">
import { computed, ref, useId } from "vue"
import { useDismissable } from "../../../composables/ui/useDismissable"
import type { ChoiceOption } from "../../../types/ui"
import Icon from "../../icons/Icon.vue"
import FormField from "../../common/forms/FormField.vue"
import SelectInput from "../../common/forms/SelectInput.vue"
import PopoverPanel from "../../common/layout/PopoverPanel.vue"

// What the next message runs with, in one control beside the composer's tools: the profile (how the assistant
// behaves) and the model. One button, so the composer row fits a phone; the choices open above it.
const props = defineProps<{
  profileOptions: ChoiceOption[]
  modelOptions: ChoiceOption[]
  /** The selected profile's name and what it is for, shown on the button and under the field. */
  profileName: string
  profileSummary: string
  disabled?: boolean
}>()

const profile = defineModel<string>("profile", { required: true })
const model = defineModel<string>("model", { default: "" })

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panelId = useId()
useDismissable(root, open)

const modelLabel = computed(() => props.modelOptions.find((o) => o.value === model.value)?.label ?? "")
const buttonLabel = computed(() => `Run settings: ${props.profileName} profile${modelLabel.value ? `, ${modelLabel.value}` : ""}. Change`)
</script>

<template>
  <div ref="root" class="relative min-w-0">
    <button
      type="button"
      data-test="run-settings"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      :aria-label="buttonLabel"
      :disabled="disabled"
      class="trigger"
      @click="open = !open"
    >
      <span class="truncate">{{ profileName }}</span>
      <Icon name="chevron-up" size="xs" class-name="flex-none" />
    </button>

    <PopoverPanel v-if="open" :id="panelId" title="Run settings" placement="above" align="left" width="sm:w-[340px]" @close="open = false">
      <div class="flex flex-col gap-3 p-3">
        <FormField label="Profile" :hint="profileSummary">
          <template #default="{ id, describedBy }">
            <SelectInput :id="id" v-model="profile" :options="profileOptions" label="Profile" :aria-describedby="describedBy" />
          </template>
        </FormField>
        <FormField v-if="modelOptions.length > 1" label="Model" hint="Used for the next message.">
          <template #default="{ id, describedBy }">
            <SelectInput :id="id" v-model="model" :options="modelOptions" label="Model" :aria-describedby="describedBy" />
          </template>
        </FormField>
      </div>
    </PopoverPanel>
  </div>
</template>

<style scoped>
.trigger {
  @apply inline-flex h-8 max-w-[10rem] items-center gap-1.5 rounded-[var(--radius-md)] px-2 font-mono text-[length:var(--text-small)] text-muted transition-colors hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed disabled:opacity-60;
}
@media (pointer: coarse) { .trigger { @apply h-11; } }
</style>
