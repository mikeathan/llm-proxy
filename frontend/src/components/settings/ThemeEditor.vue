<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { ContrastResult, ThemeDraft, ThemeInput, TokenName } from "../../types/theme"
import { PRESETS } from "../../theme/presets"
import { formatContrastRatio } from "../../utils/format/units"
import { TOKEN_REGISTRY } from "../../theme/tokenRegistry"
import { channelsToHex, parseColour } from "../../theme/colour"
import { validateCustomTheme } from "../../theme/validate"
import { EDITABLE_TOKENS, draftToInput, previewOverrides, tokenErrors, uniqueThemeId } from "../../theme/themeDraft"
import Panel from "../common/layout/Panel.vue"
import FormField from "../common/forms/FormField.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import Callout from "../common/feedback/Callout.vue"

// The custom-theme editor (plan D2): one field per editable token (derived from
// the token registry), checked live by the single validator. A value that is
// not valid for its token blocks Save and never reaches the preview; failing
// contrast pairs are listed and block Save until acknowledged. The preview is a
// scoped sample (its own data-theme + validated overrides) — the page's theme
// changes only when the saved theme is chosen. The page saves (useTheme).
const props = withDefaults(
  defineProps<{
    initial: ThemeDraft
    /** Not saved yet: Save is offered before any change. */
    isNew: boolean
    /** Custom theme ids already in use (a new id avoids them). */
    takenIds: readonly string[]
    /** Keep the draft's id (an imported theme) instead of deriving it from the name. */
    lockId?: boolean
  }>(),
  { lockId: false },
)
const emit = defineEmits<{
  (e: "save", payload: { input: ThemeInput; acknowledgeContrast: boolean }): void
  (e: "cancel"): void
  (e: "dirty-change", dirty: boolean): void
}>()

// A JSON copy: props are reactive proxies, which structuredClone refuses.
const draft = ref<ThemeDraft>(JSON.parse(JSON.stringify(props.initial)) as ThemeDraft)
const acknowledged = ref(false)

const COLOUR_TOKENS = EDITABLE_TOKENS.filter((t) => TOKEN_REGISTRY[t].kind === "colour")
const SHAPE_TOKENS = EDITABLE_TOKENS.filter((t) => TOKEN_REGISTRY[t].kind !== "colour")
const UNIT_HINT: Partial<Record<string, string>> = { length: "px, 0–64", duration: "ms, 0–2000" }

const deriveId = computed(() => props.isNew && !props.lockId)
watch(
  () => draft.value.label,
  (label) => {
    if (deriveId.value) draft.value.id = uniqueThemeId(label, props.takenIds)
  },
)

const input = computed(() => draftToInput(draft.value))
const result = computed(() => validateCustomTheme(input.value))
const errors = computed(() => (result.value.ok ? [] : result.value.errors))
const rowErrors = computed(() => tokenErrors(errors.value))
// Problems that are not about one token (the name, mostly).
const otherErrors = computed(() => errors.value.filter((e) => !Object.values(rowErrors.value).includes(e)))
const failures = computed<ContrastResult[]>(() => (result.value.ok ? result.value.contrastFailures : []))

// A different set of failing pairs needs a fresh acknowledgement.
const failureKey = computed(() => failures.value.map((f) => `${f.fg}/${f.bg}`).join(","))
watch(failureKey, () => {
  acknowledged.value = false
})

const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(props.initial))
watch(dirty, (value) => emit("dirty-change", value), { immediate: true })

const canSave = computed(
  () => result.value.ok && (props.isNew || dirty.value) && (!failures.value.length || acknowledged.value),
)

const previewStyle = computed(() =>
  Object.fromEntries(Object.entries(previewOverrides(input.value)).map(([name, value]) => [`--${name}`, value])),
)

/** The swatch colour of a row: the typed value when it parses, else the base's. */
function swatch(name: TokenName): string {
  const rgb = parseColour(draft.value.values[name] ?? "") ?? parseColour(PRESETS[draft.value.base].tokens[name])
  return rgb ? channelsToHex(rgb) : "transparent"
}

const ratio = (r: ContrastResult) => formatContrastRatio(r.ratio)

function save() {
  if (!canSave.value) return
  emit("save", { input: input.value, acknowledgeContrast: acknowledged.value })
}
</script>

<template>
  <Panel :title="isNew ? 'New theme' : `Edit ${initial.label}`" preserve-case>
    <form class="flex flex-col gap-5" @submit.prevent="save">
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
        <FormField label="Name" :hint="`Id: ${draft.id}`">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="draft.label" :aria-describedby="describedBy" type="text" maxlength="60" class="form-control" autocomplete="off" />
          </template>
        </FormField>
        <div class="flex flex-col gap-1.5">
          <MicroLabel>Based on</MicroLabel>
          <span class="font-mono text-[length:var(--text-small)] text-primary">{{ PRESETS[draft.base].label }}</span>
        </div>
      </div>

      <Callout v-if="otherErrors.length" tone="error" title="This theme cannot be saved yet">
        <ul class="m-0 pl-4"><li v-for="message in otherErrors" :key="message">{{ message }}</li></ul>
      </Callout>

      <div class="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_260px]">
        <div class="flex flex-col gap-4">
          <MicroLabel>Colours · hex, rgb() or hsl(), opaque</MicroLabel>
          <div class="token-grid">
            <FormField v-for="name in COLOUR_TOKENS" :key="name" :label="`--${name}`" :error="rowErrors[name]">
              <template #default="{ id, describedBy, invalid }">
                <div class="flex items-center gap-2">
                  <span aria-hidden="true" class="h-6 w-6 flex-none border border-control" :style="{ background: swatch(name) }"></span>
                  <input
                    :id="id"
                    v-model="draft.values[name]"
                    :data-token="name"
                    :aria-describedby="describedBy"
                    :aria-invalid="invalid"
                    type="text"
                    spellcheck="false"
                    autocomplete="off"
                    class="form-control font-mono"
                  />
                </div>
              </template>
            </FormField>
          </div>
          <MicroLabel>Shape and motion</MicroLabel>
          <div class="token-grid">
            <FormField v-for="name in SHAPE_TOKENS" :key="name" :label="`--${name}`" :hint="UNIT_HINT[TOKEN_REGISTRY[name].kind]" :error="rowErrors[name]">
              <template #default="{ id, describedBy, invalid }">
                <input
                  :id="id"
                  v-model="draft.values[name]"
                  :data-token="name"
                  :aria-describedby="describedBy"
                  :aria-invalid="invalid"
                  type="text"
                  spellcheck="false"
                  autocomplete="off"
                  class="form-control font-mono"
                />
              </template>
            </FormField>
          </div>
        </div>

        <!-- Scoped sample: its own preset block plus validated overrides only. -->
        <div
          data-test="theme-preview"
          :data-theme="draft.base"
          :style="previewStyle"
          class="flex flex-col gap-3 border border-hairline bg-canvas p-3 text-primary lg:sticky lg:top-4"
          aria-label="Theme preview"
          role="img"
        >
          <div class="flex flex-col gap-2 rounded-[var(--radius-sm)] border border-hairline bg-surface p-3">
            <span class="font-mono text-[length:var(--text-small)] uppercase tracking-[0.04em]"><span class="text-accent-brand">01</span> ── Preview</span>
            <span class="text-primary">Primary text</span>
            <span class="text-secondary">Secondary text</span>
            <span class="text-muted">Muted text</span>
            <span class="text-faint">Faint text</span>
            <span class="flex flex-wrap gap-1.5">
              <StatusTag state="success" label="Done" />
              <StatusTag state="running" label="Running" />
              <StatusTag state="error" label="Failed" />
              <StatusTag state="info" label="Info" />
            </span>
            <span class="rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-1 font-mono text-[length:var(--text-small)] text-muted">input</span>
            <span class="offset-brand w-fit rounded-[var(--radius-sm)] border border-text-primary bg-text-primary px-3 py-1 font-mono text-[length:var(--text-small)] text-inverse">Primary</span>
          </div>
        </div>
      </div>

      <div v-if="failures.length" data-test="contrast-warning" role="alert" class="flex flex-col gap-2">
        <Callout tone="error" :title="`${failures.length} contrast ${failures.length === 1 ? 'pair fails' : 'pairs fail'}`">
          <ul class="m-0 flex flex-col gap-0.5 pl-4">
            <li v-for="f in failures" :key="`${f.fg}/${f.bg}`">
              <span class="font-mono">--{{ f.fg }}</span> on <span class="font-mono">--{{ f.bg }}</span> is
              <span class="font-mono tabular-nums">{{ ratio(f) }}</span>; {{ f.role }} needs <span class="font-mono tabular-nums">{{ f.min }}:1</span>.
            </li>
          </ul>
          <template #actions>
            <BaseToggle v-model="acknowledged" label="I understand some text may be hard to read — save anyway" />
          </template>
        </Callout>
      </div>

      <div class="flex flex-wrap justify-end gap-2">
        <BaseButton variant="ghost" @click="$emit('cancel')">Cancel</BaseButton>
        <!-- A plain button: a submit button would fire save twice (click + submit). -->
        <BaseButton variant="primary" icon="check" :disabled="!canSave" @click="save">Save theme</BaseButton>
      </div>
    </form>
  </Panel>
</template>

<style scoped lang="postcss">
.token-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-3;
}
</style>
