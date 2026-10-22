<script setup lang="ts">
import { computed, ref, useId } from "vue"
import { useDismissable } from "../../../composables/ui/useDismissable"
import type { ChoiceOption } from "../../../types/ui"
import Icon from "../../icons/Icon.vue"
import FormField from "../../common/forms/FormField.vue"
import SelectInput from "../../common/forms/SelectInput.vue"
import PopoverPanel from "../../common/layout/PopoverPanel.vue"
import BaseToggle from "../../common/buttons/BaseToggle.vue"
import { HELPERS_FIELD_LABEL, HELPERS_ON_SUFFIX, HELPERS_TOGGLE_LABEL, HELPERS_UNAVAILABLE_HINT, helpersHint } from "../../../constants/labels"

// What the next message runs with, in one control beside the composer's tools: the profile (how the assistant
// behaves) and the model. One button, so the composer row fits a phone; the choices open above it.
// On a phone the sheet is teleported to <body>: the chat box is a stacking context, so a sheet left inside it
// is clipped to the box and the tab bar paints over its lower half (the same reason ComposeSheet teleports).
const props = defineProps<{
  profileOptions: ChoiceOption[]
  modelOptions: ChoiceOption[]
  /** The selected profile's name and what it is for, shown on the button and under the field. */
  profileName: string
  profileSummary: string
  /** The profile names helpers, so the "Use helpers" row is offered at all. */
  helpersShown?: boolean
  /** The server says helpers can run (a cloud helper model is set up); otherwise the row is shown disabled with why. */
  helpersAvailable?: boolean
  /** The helper model, named in the row's help text. */
  helpersModel?: string
  disabled?: boolean
  /** A phone: the choices open as a bottom sheet over the whole screen instead of a popover above the button. */
  sheet?: boolean
}>()

const profile = defineModel<string>("profile", { required: true })
const model = defineModel<string>("model", { default: "" })
/** Helpers are switched on for the next messages of this conversation. */
const helpers = defineModel<boolean>("helpers", { default: false })

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panel = ref<HTMLElement | null>(null)
const panelId = useId()
useDismissable(root, open, panel)

const modelLabel = computed(() => props.modelOptions.find((o) => o.value === model.value)?.label ?? "")
// Helpers count as on only when they will be used: the profile names them and the server can run them.
const helpersOn = computed(() => helpers.value && !!props.helpersShown && !!props.helpersAvailable)
const buttonLabel = computed(() => `Run settings: ${props.profileName} profile${modelLabel.value ? `, ${modelLabel.value}` : ""}${helpersOn.value ? ", helpers on" : ""}. Change`)
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
      :class="{ 'trigger--helpers': helpersOn }"
      @click="open = !open"
    >
      <span class="truncate">{{ profileName }}<template v-if="helpersOn"> · {{ HELPERS_ON_SUFFIX }}</template></span>
      <Icon name="chevron-up" size="xs" class-name="flex-none" />
    </button>

    <Teleport to="body" :disabled="!sheet">
    <div v-if="open" ref="panel" :class="{ 'relative z-50': sheet }">
    <PopoverPanel :id="panelId" title="Run settings" placement="above" align="left" width="sm:w-[340px]" @close="open = false">
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
        <FormField
          v-if="helpersShown"
          :label="HELPERS_FIELD_LABEL"
          :hint="helpersAvailable ? helpersHint(helpersModel ?? '') : HELPERS_UNAVAILABLE_HINT"
        >
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="helpers" :described-by="describedBy" :label="HELPERS_TOGGLE_LABEL" :disabled="!helpersAvailable" />
          </template>
        </FormField>
      </div>
    </PopoverPanel>
    </div>
    </Teleport>
  </div>
</template>

<style scoped>
.trigger {
  @apply inline-flex h-8 max-w-[10rem] items-center gap-1.5 rounded-[var(--radius-md)] px-2 font-mono text-[length:var(--text-small)] text-muted transition-colors hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed disabled:opacity-60;
}
/* "Research · helpers" is longer than a profile name alone: room for the whole word. */
.trigger--helpers { @apply max-w-[14rem]; }
@media (pointer: coarse) { .trigger { @apply h-11; } }
</style>
