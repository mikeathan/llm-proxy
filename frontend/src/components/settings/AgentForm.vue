<script setup lang="ts">
import { computed } from "vue"
import { AGENT_FORM_OTHER_TITLE, AGENT_FORM_SECTIONS } from "../../constants/settings"
import type { ChoiceOption } from "../../types/ui"
import type { ProfileDetail, ProfileField, ProfileInput } from "../../types/profile"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import FormField from "../common/forms/FormField.vue"
import Panel from "../common/layout/Panel.vue"
import UnsavedTag from "../common/display/UnsavedTag.vue"
import AgentFieldControl from "./AgentFieldControl.vue"

// One agent, drawn from the server's description of its settings. A built-in is shown read-only with a way to copy it;
// the user's own can be saved, discarded and deleted. Nothing is validated here: the server's refusal comes back as
// `fieldErrors` / `formError`.
const ABOUT_TITLE = AGENT_FORM_SECTIONS[0]?.title
const NEW_ID_HINT = "Lowercase letters, digits and dashes. It cannot be changed later."
const BUILT_IN_TITLE = "Built-in agent"
const BUILT_IN_BODY = "Built-in agents are read-only, so an update to them reaches everyone. Duplicate it to make your own version."

const props = defineProps<{
  schema: ProfileField[]
  draft: ProfileInput
  detail: ProfileDetail | null
  creating: boolean
  newId: string
  readOnly: boolean
  dirty: boolean
  saving: boolean
  fieldErrors: Record<string, string>
  formError: string
  helperOptions: ChoiceOption[]
}>()
defineEmits<{
  (e: "update", key: keyof ProfileInput, value: ProfileInput[keyof ProfileInput]): void
  (e: "update:newId", id: string): void
  (e: "save"): void
  (e: "discard"): void
  (e: "remove"): void
  (e: "duplicate"): void
  (e: "back"): void
}>()

const panels = computed(() => {
  const byKey = new Map(props.schema.map((field) => [field.key as string, field]))
  const listed = new Set(AGENT_FORM_SECTIONS.flatMap((section) => section.keys))
  const panels = AGENT_FORM_SECTIONS.map((section) => ({
    title: section.title,
    fields: section.keys.map((key) => byKey.get(key)).filter((field): field is ProfileField => !!field),
  }))
  const more = props.schema.filter((field) => !listed.has(field.key))
  return [...panels, { title: AGENT_FORM_OTHER_TITLE, fields: more }].filter((panel) => panel.fields.length > 0)
})
const title = computed(() => (props.creating ? "New agent" : (props.detail?.name ?? "")))
const removeLabel = computed(() => (props.detail?.replaces_builtin ? "Restore built-in" : "Delete"))
// A helper hint only where it applies: the agents this one can pick are every other one.
const canSave = computed(() => props.dirty || (props.creating && props.draft.name.trim() !== ""))
</script>

<template>
  <form class="flex min-w-0 flex-col gap-4" novalidate @submit.prevent="$emit('save')">
    <div class="flex flex-wrap items-center gap-2">
      <BaseButton variant="ghost" size="sm" icon="chevron-left" class-name="lg:hidden" @click="$emit('back')">Agents</BaseButton>
      <h2 class="m-0 min-w-0 flex-1 truncate text-[length:var(--text-heading)] font-semibold text-primary">{{ title }}</h2>
      <UnsavedTag v-if="dirty" />
      <BaseButton v-if="detail && !creating" variant="secondary" size="sm" icon="copy" @click="$emit('duplicate')">Duplicate</BaseButton>
    </div>

    <Callout v-if="readOnly" tone="info" :title="BUILT_IN_TITLE">{{ BUILT_IN_BODY }}</Callout>
    <Callout v-if="formError" tone="error" title="Not saved">{{ formError }}</Callout>

    <Panel v-for="panel in panels" :key="panel.title" :title="panel.title">
      <div class="flex flex-col gap-4">
        <AgentFieldControl
          v-for="field in panel.fields"
          :key="field.key"
          :field="field"
          :draft="draft"
          :error="fieldErrors[field.key]"
          :disabled="readOnly || saving"
          :helper-options="helperOptions"
          @update="(key, value) => $emit('update', key, value)"
        />
        <FormField v-if="creating && panel.title === ABOUT_TITLE" label="ID" :hint="NEW_ID_HINT" :error="fieldErrors.id">
          <template #default="{ id, describedBy, invalid }">
            <input
              :id="id"
              :value="newId"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
              type="text"
              autocomplete="off"
              spellcheck="false"
              class="form-control font-mono"
              @input="$emit('update:newId', ($event.target as HTMLInputElement).value)"
            />
          </template>
        </FormField>
      </div>
    </Panel>

    <div v-if="!readOnly" class="sticky bottom-0 z-10 -mx-1 flex flex-wrap items-center justify-end gap-2 border-t border-hairline bg-canvas px-1 py-3">
      <BaseButton v-if="detail && !creating" variant="danger" :disabled="saving" class-name="mr-auto" @click="$emit('remove')">{{ removeLabel }}</BaseButton>
      <BaseButton variant="ghost" :disabled="!dirty || saving" @click="$emit('discard')">Discard changes</BaseButton>
      <BaseButton variant="primary" type="submit" :loading="saving" :disabled="!canSave">{{ creating ? "Create agent" : "Save agent" }}</BaseButton>
    </div>
  </form>
</template>
