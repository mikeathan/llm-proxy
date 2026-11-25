<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useSourceLookup } from "../../composables/newsroom/useSourceLookup"
import { sourceProblem } from "../../utils/newsroom/sourceCheck"
import { safeWebLink } from "../../utils/newsroom/safeLink"
import { LOOKUP_BUSY, LOOKUP_HINTS, LOOKUP_LABEL, LOOKUP_LATEST_LABEL, LOOKUP_NOTHING_YET, LOOKUP_PICK_LABEL, lookupSavedAs, matchingLabel, trendingLabel } from "../../constants/newsroom"
import type { NewsroomSource } from "../../types/newsroom"

// Under one source of the watch form: "Look up" shows what the source would read (its name, its latest items) before it is saved, and offers what the
// owner can pick (the feeds a page advertises, a Mastodon server's tags). Everything shown is text from the open internet, so it is shown as text and a
// link opens only if it is http or https. A look-up never blocks saving; the server still checks the source when the watch is saved.
const props = defineProps<{ source: NewsroomSource }>()
const emit = defineEmits<{ (e: "pick", value: { target: string; instance?: string }): void }>()

const { preview, loading, choices, suggestProblem, lookUp, suggest, clearPreview, clearSuggestions } = useSourceLookup()

const ready = computed(() => props.source.target.trim() !== "" && sourceProblem(props.source) === "")
const mastodonHost = computed(() => (props.source.instance ?? "").trim().toLowerCase().replace(/^https?:\/\//, "").split(/[/?#]/)[0] ?? "")
const typedTag = computed(() => props.source.target.trim().replace(/^#/, ""))
const suggestLabel = computed(() => (typedTag.value ? matchingLabel(typedTag.value) : trendingLabel(mastodonHost.value)))
const showSuggestions = computed(() => props.source.kind === "mastodon" && (choices.value.length > 0 || suggestProblem.value !== ""))

async function run() {
  if (!ready.value) return
  const got = await lookUp({ ...props.source })
  // A found source is kept in its cleaned-up form, so a pasted link becomes the bare value the watch will keep.
  if (got?.found && got.target && (got.target !== props.source.target || (got.instance && got.instance !== props.source.instance))) {
    emit("pick", { target: got.target, ...(got.instance ? { instance: got.instance } : {}) })
  }
}

function pick(value: string) {
  emit("pick", { target: value })
  clearPreview()
}

// What was looked up is no longer what is typed: drop the preview. A Mastodon server's tags follow the typing, after a pause.
let timer: ReturnType<typeof setTimeout> | null = null
watch(
  () => [props.source.kind, props.source.target, props.source.instance] as const,
  () => {
    const p = preview.value
    if (p && (!p.found || p.target !== props.source.target || (p.instance ?? "") !== (props.source.instance ?? ""))) clearPreview()
    if (timer) clearTimeout(timer)
    if (props.source.kind !== "mastodon") return clearSuggestions()
    const host = mastodonHost.value
    if (!host.includes(".")) return clearSuggestions()
    timer = setTimeout(() => void suggest({ ...props.source }), 450)
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
})

const when = (iso?: string): string => (iso ? new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" }) : "")
</script>

<template>
  <div class="flex flex-col gap-2" data-test="source-lookup">
    <div class="flex flex-wrap items-center gap-2">
      <BaseButton variant="secondary" size="sm" icon="search" :disabled="!ready || loading" data-test="lookup-run" @click="run">{{ loading ? LOOKUP_BUSY : LOOKUP_LABEL }}</BaseButton>
      <span class="text-[length:var(--text-small)] text-faint">{{ LOOKUP_HINTS[source.kind] }}</span>
    </div>

    <div v-if="showSuggestions" class="flex flex-col gap-1" data-test="lookup-suggestions">
      <span class="text-[length:var(--text-small)] text-secondary">{{ suggestProblem || suggestLabel }}</span>
      <div v-if="choices.length" class="flex flex-wrap gap-2">
        <BaseButton v-for="c in choices" :key="c.value" variant="secondary" size="sm" :title="c.note" data-test="lookup-suggestion" @click="pick(c.value)">{{ c.label }}</BaseButton>
      </div>
    </div>

    <div aria-live="polite" class="flex flex-col gap-2">
      <div v-if="preview?.found" class="flex flex-col gap-1 border border-hairline p-2" data-test="lookup-found">
        <span class="font-medium text-primary [overflow-wrap:anywhere]">{{ preview.title }}</span>
        <span v-if="preview.target && preview.target !== source.target" class="text-[length:var(--text-small)] text-faint">{{ lookupSavedAs(preview.target) }}</span>
        <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ LOOKUP_LATEST_LABEL }}</span>
        <ul v-if="preview.sample.length" class="m-0 flex list-none flex-col gap-1 p-0">
          <li v-for="(s, i) in preview.sample" :key="i" class="text-[length:var(--text-small)] text-secondary [overflow-wrap:anywhere]" data-test="lookup-sample">
            <a v-if="safeWebLink(s.link)" :href="safeWebLink(s.link) ?? undefined" target="_blank" rel="noopener noreferrer" class="hover:underline">{{ s.title }}</a>
            <template v-else>{{ s.title }}</template>
            <span v-if="s.at" class="text-faint"> · {{ when(s.at) }}</span>
            <p v-if="s.text" class="m-0 text-faint">{{ s.text }}</p>
          </li>
        </ul>
        <span v-else class="text-[length:var(--text-small)] text-faint">{{ LOOKUP_NOTHING_YET }}</span>
      </div>

      <div v-else-if="preview" class="flex flex-col gap-1" data-test="lookup-problem">
        <p role="status" class="m-0 text-[length:var(--text-small)] text-state-running">{{ preview.problem }}</p>
        <template v-if="preview.choices.length">
          <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ LOOKUP_PICK_LABEL }}</span>
          <div class="flex flex-col items-start gap-1">
            <BaseButton v-for="c in preview.choices" :key="c.value" variant="secondary" size="sm" data-test="lookup-choice" @click="pick(c.value)">{{ c.label === c.value ? c.value : `${c.label} (${c.value})` }}</BaseButton>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
