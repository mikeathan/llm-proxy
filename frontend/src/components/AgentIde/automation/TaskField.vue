<script setup lang="ts">
import { computed } from "vue"
import type { TaskMode } from "../../../types/automation"
import type { ChoiceOption } from "../../../types/ui"
import type { TemplateMetadata } from "../../../types/templates"
import FormField from "../../common/forms/FormField.vue"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"

// What the automation does: a task file already in the workspace, or a new task
// written here (optionally started from a playbook) and saved as a file named
// after the automation. Choosing the source is only offered when creating.

const props = defineProps<{
  taskFile: string
  files: string[]
  mode: TaskMode
  content: string
  playbookId: string
  playbooks: TemplateMetadata[]
  playbooksFailed: boolean
  workspaceChosen: boolean
  canChooseMode: boolean
  newFileName: string
  // Changes when the chosen playbook was not applied, so the list goes back to what the box really holds.
  playbookKey: number
}>()

const emit = defineEmits<{
  (e: "update:taskFile", value: string): void
  (e: "update:mode", value: TaskMode): void
  (e: "update:content", value: string): void
  (e: "choose-playbook", id: string): void
}>()

const MODE_OPTIONS: ChoiceOption[] = [
  { value: "existing", label: "Existing file" },
  { value: "new", label: "New task" },
]
const UNCATEGORISED = "Other"

// Playbooks grouped by category, in the order the library lists them.
const groups = computed(() => {
  const byCategory = new Map<string, TemplateMetadata[]>()
  for (const p of props.playbooks) {
    const key = p.category || UNCATEGORISED
    byCategory.set(key, [...(byCategory.get(key) ?? []), p])
  }
  return [...byCategory.entries()]
})

const fileHint = computed(() => (props.workspaceChosen ? "The file whose contents become the task." : "Choose a workspace first."))
const contentHint = computed(() => (props.newFileName ? `Saved in the workspace as ${props.newFileName}.` : "Saved as a file named after the automation. Name it first."))
</script>

<template>
  <div class="flex flex-col gap-3">
    <SegmentedControl
      v-if="canChooseMode"
      class="self-start"
      label="Task from"
      :model-value="mode"
      :options="MODE_OPTIONS"
      @update:model-value="emit('update:mode', $event as TaskMode)"
    />

    <FormField v-if="mode === 'existing'" label="Task file" :hint="fileHint">
      <template #default="{ id, describedBy }">
        <select
          :id="id"
          :value="taskFile"
          :aria-describedby="describedBy"
          :disabled="!workspaceChosen"
          class="form-control font-mono text-[length:var(--text-small)]"
          @change="emit('update:taskFile', ($event.target as HTMLSelectElement).value)"
        >
          <option value="" disabled>Choose a file…</option>
          <option v-for="file in files" :key="file" :value="file">{{ file }}</option>
        </select>
      </template>
    </FormField>

    <template v-else>
      <FormField
        label="Start from a playbook"
        :hint="playbooksFailed ? 'The playbooks could not be loaded. You can still write the task below.' : 'Optional. Fills the box below; change it freely.'"
      >
        <template #default="{ id, describedBy }">
          <select
            :id="id"
            :key="playbookKey"
            :value="playbookId"
            :aria-describedby="describedBy"
            class="form-control max-w-[28rem]"
            @change="emit('choose-playbook', ($event.target as HTMLSelectElement).value)"
          >
            <option value="" disabled>Choose a playbook…</option>
            <optgroup v-for="[category, items] in groups" :key="category" :label="category">
              <option v-for="p in items" :key="p.id" :value="p.id">{{ p.name }}</option>
            </optgroup>
          </select>
        </template>
      </FormField>
      <FormField label="Task" :hint="contentHint">
        <template #default="{ id, describedBy }">
          <textarea
            :id="id"
            :value="content"
            :aria-describedby="describedBy"
            rows="10"
            placeholder="Describe what the automation should do each time it runs."
            class="form-control min-h-[10rem] resize-y font-mono text-[length:var(--text-small)]"
            @input="emit('update:content', ($event.target as HTMLTextAreaElement).value)"
          />
        </template>
      </FormField>
    </template>
  </div>
</template>
