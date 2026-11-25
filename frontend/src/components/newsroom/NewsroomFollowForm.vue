<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import {
  FOLLOW_CHANNEL_LABEL, FOLLOW_FEED_LABEL, FOLLOW_HOW_LABEL, FOLLOW_LOOK_FOR_LABEL, FOLLOW_LOOK_FOR_PLACEHOLDER, FOLLOW_MODES, FOLLOW_PREVIEW_HEADING, FOLLOW_PREVIEW_LABEL, FOLLOW_PREVIEW_NONE, FOLLOW_SAVE_LABEL,
  FOLLOW_SECTION_HINT, FOLLOW_SECTION_LABEL, FOLLOW_WHAT_LABEL, FOLLOW_WHAT_PLACEHOLDER, MY_SOURCE_LINK_PLACEHOLDER, TOPIC_FEEDS_REJECTED, WATCH_LOOK_FOR_MAX, WATCH_NAME_MAX, isFeedAddress,
} from "../../constants/newsroom"
import type { ChoiceOption } from "../../types/ui"
import type { BriefFollowEdit, FollowMode, SourcePreview } from "../../types/newsroom"

// One way to follow something (ADR-0086): what to follow, then how to hear about it: written up in the brief, an alert at once, or both. The details each
// way needs show only for that way. A preview shows what a news search finds for the words now, before anything is saved. The page saves; the server
// checks everything again and `error` is its reason when it refuses.
const props = withDefaults(defineProps<{ sections: string[]; channels: ChoiceOption[]; saving: boolean; error: string; preview: SourcePreview | null; initialText?: string; initialMode?: FollowMode }>(), {
  initialText: "",
  initialMode: "brief",
})
const emit = defineEmits<{ (e: "save", edit: BriefFollowEdit): void; (e: "preview", text: string): void; (e: "cancel"): void; (e: "advanced"): void }>()

const text = ref(props.initialText)
const mode = ref<FollowMode>(props.initialMode)
const section = ref("")
const feed = ref("")
const lookFor = ref("")
const channel = ref("")
const MODES: FollowMode[] = ["brief", "alerts", "both"]
const sectionsId = "follow-sections"
onMounted(() => { if (text.value.trim()) emit("preview", text.value) })
watch(() => props.initialText, (t) => (text.value = t))

const feedBad = computed(() => feed.value.trim() !== "" && !isFeedAddress(feed.value.trim()))
const canSave = computed(() => !props.saving && text.value.trim() !== "" && !feedBad.value)
const samples = computed(() => props.preview?.sample.map((s) => s.title).filter(Boolean) ?? [])
function submit() {
  if (!canSave.value) return
  emit("save", { text: text.value, mode: mode.value, section: section.value, feed: feed.value, lookFor: lookFor.value, channel: channel.value })
}
</script>

<template>
  <form class="flex flex-col gap-4" data-test="follow-form" @submit.prevent="submit">
    <FormField :label="FOLLOW_WHAT_LABEL">
      <template #default="{ id }">
        <div class="flex flex-wrap gap-2">
          <input :id="id" v-model="text" type="text" :maxlength="WATCH_NAME_MAX" :placeholder="FOLLOW_WHAT_PLACEHOLDER" required class="form-control min-w-0 flex-1" data-test="follow-text" />
          <BaseButton size="sm" variant="secondary" :disabled="!text.trim()" data-test="follow-preview" @click="emit('preview', text)">{{ FOLLOW_PREVIEW_LABEL }}</BaseButton>
        </div>
      </template>
    </FormField>
    <div v-if="preview" class="flex flex-col gap-1 border border-dashed border-strong p-3 text-[length:var(--text-small)]" data-test="follow-preview-list">
      <span class="font-mono uppercase tracking-wider text-faint">{{ FOLLOW_PREVIEW_HEADING }}</span>
      <ul v-if="samples.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0"><li v-for="s in samples" :key="s" class="break-words py-1 text-primary">{{ s }}</li></ul>
      <p v-else class="m-0 text-muted">{{ preview.problem || FOLLOW_PREVIEW_NONE }}</p>
    </div>
    <fieldset class="m-0 flex min-w-0 flex-col gap-2 border-0 p-0">
      <legend class="mb-1 p-0 text-[length:var(--text-small)] font-medium text-secondary">{{ FOLLOW_HOW_LABEL }}</legend>
      <div role="radiogroup" :aria-label="FOLLOW_HOW_LABEL" class="grid gap-2 sm:grid-cols-3" data-test="follow-modes">
        <button
          v-for="m in MODES"
          :key="m"
          type="button"
          role="radio"
          :aria-checked="mode === m"
          :class="['flex min-h-11 flex-col gap-1 rounded-[var(--radius-sm)] border p-3 text-left focus-visible:outline-none focus-visible:ring-2', mode === m ? 'border-accent-brand bg-surface-active' : 'border-control bg-canvas hover:bg-surface-hover']"
          :data-mode="m"
          @click="mode = m"
        >
          <span class="font-semibold text-primary">{{ FOLLOW_MODES[m].label }}</span>
          <span class="text-[length:var(--text-small)] text-muted">{{ FOLLOW_MODES[m].hint }}</span>
        </button>
      </div>
    </fieldset>
    <div class="grid gap-4 sm:grid-cols-2">
      <FormField v-if="mode !== 'alerts'" :label="FOLLOW_SECTION_LABEL" :hint="FOLLOW_SECTION_HINT">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="section" type="text" :list="sectionsId" :maxlength="WATCH_NAME_MAX" :aria-describedby="describedBy" class="form-control" data-test="follow-section" />
          <datalist :id="sectionsId"><option v-for="s in sections" :key="s" :value="s" /></datalist>
        </template>
      </FormField>
      <FormField :label="FOLLOW_FEED_LABEL" :error="feedBad ? TOPIC_FEEDS_REJECTED : ''">
        <template #default="{ id, describedBy, invalid }">
          <input :id="id" v-model="feed" type="text" inputmode="url" :placeholder="MY_SOURCE_LINK_PLACEHOLDER" :aria-describedby="describedBy" :aria-invalid="invalid || undefined" class="form-control" />
        </template>
      </FormField>
      <FormField v-if="mode !== 'brief'" :label="FOLLOW_LOOK_FOR_LABEL">
        <template #default="{ id }"><input :id="id" v-model="lookFor" type="text" :maxlength="WATCH_LOOK_FOR_MAX" :placeholder="FOLLOW_LOOK_FOR_PLACEHOLDER" class="form-control" data-test="follow-look-for" /></template>
      </FormField>
      <div v-if="mode !== 'brief' && channels.length > 1">
        <SelectInput v-model="channel" :options="channels" :label="FOLLOW_CHANNEL_LABEL" />
      </div>
    </div>
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex flex-wrap items-center gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave" data-test="follow-save">{{ FOLLOW_SAVE_LABEL }}</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
      <button type="button" class="ml-auto border-0 bg-transparent p-0 text-[length:var(--text-small)] text-accent-info-text hover:underline" data-test="follow-advanced" @click="emit('advanced')">Set up a watch with every option →</button>
    </div>
  </form>
</template>
