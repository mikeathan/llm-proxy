<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { CustomTheme, PresetId, ThemeDraft, ThemeInput } from "../../types/theme"
import type { DataTableColumn, StatusState } from "../../types/ui"
import { useTheme } from "../../composables/ui/useTheme"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import { PRESETS } from "../../theme/presets"
import { validateCustomTheme } from "../../theme/validate"
import { draftFromPreset, draftFromTheme } from "../../theme/themeDraft"
import { downloadText } from "../../utils/download"
import Panel from "../common/layout/Panel.vue"
import DataTable from "../common/display/DataTable.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import Callout from "../common/feedback/Callout.vue"
import ThemeEditor from "./ThemeEditor.vue"

// Settings · Appearance (plan D2 / D3): presets apply the moment they are
// picked; custom themes are created by duplicating a theme or importing a file,
// edited in ThemeEditor, exported to keep a copy (they live in this browser
// only) and deleted after confirmation. Every path goes through the one
// validator in useTheme; a rejected file changes nothing. An open editor with
// edits is reported to the page's unsaved-change guard via dirty-change.
const emit = defineEmits<{ (e: "dirty-change", dirty: boolean): void }>()

const theme = useTheme()
const { confirm } = useConfirm()
const toast = useToast()

// A theme file is a few kilobytes; anything far larger is not one.
const IMPORT_MAX_BYTES = 64 * 1024
const JSON_TYPE = "application/json"
const COPY_SUFFIX = " copy"

// ── Theme choice ───────────────────────────────────────────────────────────
const followsSystem = computed({
  get: () => theme.selection.value === null,
  set: (follow: boolean) => (follow ? theme.followSystem() : theme.selectPreset(theme.current.value.presetId)),
})
const selectedPreset = computed(() => (theme.selection.value?.kind === "preset" ? theme.selection.value.id : null))
const activeCustom = computed(() =>
  theme.selection.value?.kind === "custom" ? theme.customThemes.value.find((t) => t.id === theme.selection.value?.id) : undefined,
)
const currentPresetLabel = computed(() => PRESETS[theme.current.value.presetId].label)

// ── Custom themes ──────────────────────────────────────────────────────────
const takenIds = computed(() => theme.customThemes.value.map((t) => t.id))

function contrastStatus(t: CustomTheme): { state: StatusState; label: string } {
  const result = validateCustomTheme(t)
  const failing = result.ok ? result.contrastFailures.length : 0
  if (!failing) return { state: "success", label: "All pairs pass" }
  return { state: "queued", label: `${failing} ${failing === 1 ? "pair" : "pairs"} acknowledged` }
}

const COLUMNS: DataTableColumn<CustomTheme>[] = [
  { key: "name", label: "Theme" },
  { key: "base", label: "Based on", value: (t) => PRESETS[t.base].label },
  { key: "contrast", label: "Contrast" },
  { key: "actions", label: "Actions" },
]

async function remove(t: CustomTheme) {
  const ok = await confirm({
    title: `Delete ${t.label}?`,
    message:
      activeCustom.value?.id === t.id
        ? `It is the theme in use; the page switches to ${PRESETS[t.base].label}. Export it first to keep a copy.`
        : "Custom themes live in this browser only. Export it first to keep a copy.",
    type: "warning",
    confirmText: "Delete theme",
  })
  if (ok) theme.deleteCustomTheme(t.id)
}

function exportTheme(t: CustomTheme) {
  const json = theme.exportTheme(t.id)
  if (json) downloadText(`${t.id}.theme.json`, json, JSON_TYPE)
}

// ── Editor ─────────────────────────────────────────────────────────────────
const editor = ref<{ draft: ThemeDraft; isNew: boolean; lockId: boolean; key: number } | null>(null)
const editorDirty = ref(false)
let editorSeq = 0
watch(editorDirty, (dirty) => emit("dirty-change", dirty))

async function openEditor(draft: ThemeDraft, isNew: boolean, lockId = false) {
  if (editor.value && editorDirty.value && !(await confirmDiscard())) return
  editorDirty.value = false
  editor.value = { draft, isNew, lockId, key: ++editorSeq }
}

const confirmDiscard = () =>
  confirm({ title: "Discard theme edits?", message: "Your changes to this theme are not saved.", type: "warning", confirmText: "Discard", cancelText: "Keep editing" })

function closeEditor() {
  editorDirty.value = false
  editor.value = null
}

async function cancelEditor() {
  if (editorDirty.value && !(await confirmDiscard())) return
  closeEditor()
}

const duplicatePreset = (id: PresetId) => openEditor(draftFromPreset(id, `${PRESETS[id].label}${COPY_SUFFIX}`, takenIds.value), true)
// A copy keeps the theme's values under a new name and id.
const duplicateCustom = (t: CustomTheme) =>
  openEditor({ ...draftFromPreset(t.base, `${t.label}${COPY_SUFFIX}`, takenIds.value), values: draftFromTheme(t).values }, true)
const editCustom = (t: CustomTheme) => openEditor(draftFromTheme(t), false)

function saveFromEditor({ input, acknowledgeContrast }: { input: ThemeInput; acknowledgeContrast: boolean }) {
  const { result, saved } = theme.saveCustomTheme(input, { acknowledgeContrast })
  if (!saved) {
    toast.error(`Could not save the theme: ${result.ok ? "acknowledge the contrast failures first" : result.errors.join("; ")}`)
    return
  }
  toast.success(`Saved ${input.label}`)
  closeEditor()
}

// ── Import ─────────────────────────────────────────────────────────────────
const fileInput = ref<HTMLInputElement | null>(null)
const importError = ref<{ file: string; reasons: string[] } | null>(null)

async function onFileChosen(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ""
  if (file) await importFile(file)
}

async function importFile(file: File) {
  importError.value = null
  const reject = (reasons: string[]) => (importError.value = { file: file.name, reasons })
  if (file.size > IMPORT_MAX_BYTES) return reject([`the file is larger than ${IMPORT_MAX_BYTES / 1024} KB, so it is not a theme file`])

  let parsed: unknown
  try {
    parsed = JSON.parse(await file.text())
  } catch {
    return reject(["the file is not valid JSON"])
  }
  const result = validateCustomTheme(parsed)
  if (!result.ok) return reject(result.errors)

  const existing = theme.customThemes.value.find((t) => t.id === result.theme.id)
  if (existing) {
    const ok = await confirm({
      title: `Replace ${existing.label}?`,
      message: `The file's theme has the same id (${existing.id}) as one you already have. Importing replaces it.`,
      type: "warning",
      confirmText: "Replace",
    })
    if (!ok) return
  }
  // Contrast failures need an explicit acknowledgement: review them in the editor.
  if (result.contrastFailures.length) return openEditor(draftFromTheme(result.theme), true, true)
  theme.saveCustomTheme(result.theme)
  toast.success(`Imported ${result.theme.label}`)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Theme">
      <fieldset class="m-0 grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-3 border-0 p-0">
        <legend class="sr-only">Theme</legend>
        <label v-for="preset in theme.presets" :key="preset.id" class="relative block cursor-pointer">
          <input
            type="radio"
            name="theme"
            class="peer sr-only"
            :value="preset.id"
            :checked="selectedPreset === preset.id"
            @change="theme.selectPreset(preset.id)"
          />
          <span
            class="flex h-full flex-col gap-2 rounded-[var(--radius-md)] border border-control bg-canvas p-2.5 peer-checked:border-accent-brand peer-checked:shadow-[inset_0_0_0_1px_rgb(var(--accent-brand))] peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-focus-ring"
          >
            <!-- Each card renders in its own preset block, whatever the page theme is. -->
            <span aria-hidden="true" :data-theme="preset.id" class="flex h-16 gap-1.5 border border-hairline bg-canvas p-1.5">
              <span class="w-3 bg-surface"></span>
              <span class="flex flex-1 flex-col gap-1 border border-hairline bg-surface p-1.5">
                <i class="h-1 w-3/4 bg-text-primary"></i>
                <i class="h-1 w-1/2 bg-text-muted"></i>
                <span class="mt-auto flex gap-1">
                  <i class="h-1.5 w-1.5 bg-accent-brand"></i><i class="h-1.5 w-1.5 bg-state-success"></i><i class="h-1.5 w-1.5 bg-state-running"></i><i class="h-1.5 w-1.5 bg-accent-info"></i>
                </span>
              </span>
            </span>
            <span class="text-[length:var(--text-small)] font-medium text-primary">{{ preset.label }}</span>
            <span class="font-mono text-[length:var(--text-micro)] text-muted">{{ preset.id }} · {{ preset.scheme }}</span>
          </span>
        </label>
      </fieldset>
      <div class="mt-4 flex flex-col gap-2">
        <BaseToggle v-model="followsSystem" label="Follow the system light/dark setting until I choose a theme" />
        <p v-if="activeCustom" class="m-0 text-[length:var(--text-small)] text-muted">
          In use: custom theme <span class="font-mono text-primary">{{ activeCustom.label }}</span> (based on {{ PRESETS[activeCustom.base].label }}).
        </p>
      </div>
    </Panel>

    <Panel title="Custom themes" :flush="!!theme.customThemes.value.length">
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="plus" @click="duplicatePreset(theme.current.value.presetId)">Duplicate {{ currentPresetLabel }}</BaseButton>
        <BaseButton variant="secondary" size="sm" icon="arrow-up" @click="fileInput?.click()">Import</BaseButton>
        <input ref="fileInput" type="file" accept=".json,application/json" class="sr-only" tabindex="-1" aria-hidden="true" @change="onFileChosen" />
      </template>

      <div v-if="importError" data-test="import-error" :class="theme.customThemes.value.length ? 'p-4 pb-0' : 'mb-4'">
        <Callout tone="error" :title="`${importError.file} was rejected — nothing was imported`">
          <ul class="m-0 pl-4"><li v-for="reason in importError.reasons" :key="reason">{{ reason }}</li></ul>
          <p class="mb-0 mt-1">Use hex, rgb() or hsl() colours and token names from this page, then import again.</p>
          <template #actions><BaseButton variant="ghost" size="sm" @click="importError = null">Dismiss</BaseButton></template>
        </Callout>
      </div>

      <EmptyState
        v-if="!theme.customThemes.value.length"
        title="No custom themes"
        body="Duplicate a theme to make your own, or import a theme file. Custom themes live in this browser only — export them to keep a copy."
      />
      <DataTable v-else :columns="COLUMNS" :rows="theme.customThemes.value" :row-key="(t) => t.id" caption="Custom themes">
        <template #cell-name="{ row }">
          <span class="text-primary">{{ row.label }}</span>
          <span class="ml-2 font-mono text-[length:var(--text-micro)] text-faint">{{ row.id }}</span>
          <StatusTag v-if="activeCustom?.id === row.id" class="ml-2" state="info" label="In use" />
        </template>
        <template #cell-contrast="{ row }"><StatusTag v-bind="contrastStatus(row)" /></template>
        <template #cell-actions="{ row }">
          <span class="inline-flex flex-wrap items-center gap-1">
            <BaseButton v-if="activeCustom?.id !== row.id" variant="ghost" size="sm" icon="check" icon-only :label="`Use ${row.label}`" @click="theme.selectCustom(row.id)" />
            <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${row.label}`" @click="editCustom(row)" />
            <BaseButton variant="ghost" size="sm" icon="plus" icon-only :label="`Duplicate ${row.label}`" @click="duplicateCustom(row)" />
            <BaseButton variant="ghost" size="sm" icon="arrow-down" icon-only :label="`Export ${row.label}`" @click="exportTheme(row)" />
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${row.label}`" @click="remove(row)" />
          </span>
        </template>
      </DataTable>
    </Panel>

    <ThemeEditor
      v-if="editor"
      :key="editor.key"
      :initial="editor.draft"
      :is-new="editor.isNew"
      :lock-id="editor.lockId"
      :taken-ids="takenIds"
      @save="saveFromEditor"
      @cancel="cancelEditor"
      @dirty-change="editorDirty = $event"
    />
  </div>
</template>
