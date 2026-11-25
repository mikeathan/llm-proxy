<script setup lang="ts">
import { computed, ref } from "vue"
import { downloadText } from "../../utils/download"
import { opmlOf, parseOpml } from "../../utils/newsroom/opml"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import {
  MY_SOURCES_BODY, MY_SOURCES_EMPTY, MY_SOURCE_ADD_LABEL, MY_SOURCE_LINK_LABEL, MY_SOURCE_LINK_PLACEHOLDER, MY_SOURCE_NAME_LABEL, MY_SOURCE_NAME_PLACEHOLDER, WATCH_NAME_MAX,
  TOPIC_FEEDS_REJECTED, isFeedAddress, OPML_EXPORT_LABEL, OPML_FILE_NAME, OPML_IMPORT_LABEL, OPML_NONE, opmlImportedWords,
} from "../../constants/newsroom"
import type { BriefSource, OpmlFeed } from "../../types/newsroom"

// The owner's own list of websites: add one by name and feed link, remove one. A topic copies what it picks, so removing an entry changes no topic.
// The page does the saving and answers whether it worked (the form clears only then); `error` is its reason when it did not.
// A list from a feed reader (OPML) is read here and added in one go; the owner's own list goes out the same way.
const props = defineProps<{
  sources: readonly BriefSource[]
  saving: boolean
  error: string
  add: (label: string, target: string) => Promise<boolean>
  remove: (id: number) => Promise<boolean>
  importList: (feeds: OpmlFeed[]) => Promise<{ added: number; skipped: number }>
}>()
const imported = ref("")
async function importFile(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = input.files?.[0]
  input.value = ""
  if (!picked) return
  const known = new Set(props.sources.map((s) => s.target))
  const feeds = parseOpml(await picked.text())
  const fresh = feeds.filter((f) => !known.has(f.target))
  if (!feeds.length) {
    imported.value = OPML_NONE
    return
  }
  const done = await props.importList(fresh)
  imported.value = opmlImportedWords(done.added, done.skipped + feeds.length - fresh.length)
}
const exportList = () => downloadText(OPML_FILE_NAME, opmlOf(props.sources), "text/x-opml")
const label = ref("")
const target = ref("")
const failed = ref(false)
const mine = computed(() => props.sources.filter((s) => !s.builtin))
const canAdd = computed(() => !props.saving && label.value.trim() !== "" && isFeedAddress(target.value.trim()))
async function submit() {
  if (!canAdd.value) return
  failed.value = !(await props.add(label.value.trim(), target.value.trim()))
  if (!failed.value) label.value = target.value = ""
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <p class="m-0 text-muted">{{ MY_SOURCES_BODY }}</p>
    <ul v-if="mine.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="My sources">
      <li v-for="s in mine" :key="s.id" class="flex items-center gap-2 py-2" data-test="my-source">
        <span class="min-w-0 flex-1">
          <span class="block break-words text-primary">{{ s.label }}</span>
          <span class="block truncate text-[length:var(--text-small)] text-faint">{{ s.target }}</span>
        </span>
        <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Remove ${s.label}`" :disabled="saving" @click="remove(s.id as number)" />
      </li>
    </ul>
    <p v-else class="m-0 text-[length:var(--text-small)] text-faint">{{ MY_SOURCES_EMPTY }}</p>
    <form class="flex flex-col gap-3" @submit.prevent="submit">
      <div class="grid items-start gap-3 sm:grid-cols-[1fr_2fr]">
      <FormField :label="MY_SOURCE_NAME_LABEL">
        <template #default="{ id }"><input :id="id" v-model="label" type="text" :maxlength="WATCH_NAME_MAX" :placeholder="MY_SOURCE_NAME_PLACEHOLDER" class="form-control" /></template>
      </FormField>
      <FormField :label="MY_SOURCE_LINK_LABEL" :error="failed ? error : target.trim() !== '' && !isFeedAddress(target.trim()) ? TOPIC_FEEDS_REJECTED : ''">
        <template #default="{ id, describedBy, invalid }">
          <input :id="id" v-model="target" type="text" inputmode="url" :aria-describedby="describedBy" :aria-invalid="invalid || undefined" :placeholder="MY_SOURCE_LINK_PLACEHOLDER" class="form-control" />
        </template>
      </FormField>
      </div>
      <div><BaseButton type="submit" size="sm" :disabled="!canAdd">{{ MY_SOURCE_ADD_LABEL }}</BaseButton></div>
    </form>
    <div class="flex flex-wrap items-center gap-2 border-t border-hairline pt-3" data-test="opml">
      <label class="inline-flex min-h-11 cursor-pointer items-center rounded-[var(--radius-sm)] border border-control px-3 text-[length:var(--text-small)] text-primary hover:bg-surface-hover focus-within:ring-2 sm:min-h-8">
        {{ OPML_IMPORT_LABEL }}
        <input type="file" accept=".opml,.xml,text/xml,application/xml,text/x-opml" class="sr-only" :disabled="saving" data-test="opml-import" @change="importFile" />
      </label>
      <BaseButton v-if="mine.length" variant="ghost" size="sm" data-test="opml-export" @click="exportList">{{ OPML_EXPORT_LABEL }}</BaseButton>
      <span v-if="imported" role="status" class="text-[length:var(--text-small)] text-muted" data-test="opml-result">{{ imported }}</span>
    </div>
  </div>
</template>
