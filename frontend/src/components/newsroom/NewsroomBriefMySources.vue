<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import {
  MY_SOURCES_BODY, MY_SOURCES_EMPTY, MY_SOURCE_ADD_LABEL, MY_SOURCE_LINK_LABEL, MY_SOURCE_LINK_PLACEHOLDER, MY_SOURCE_NAME_LABEL, MY_SOURCE_NAME_PLACEHOLDER, WATCH_NAME_MAX,
  TOPIC_FEEDS_REJECTED, isFeedAddress,
} from "../../constants/newsroom"
import type { BriefSource } from "../../types/newsroom"

// The owner's own list of websites: add one by name and feed link, remove one. A topic copies what it picks, so removing an entry changes no topic.
// The page does the saving and answers whether it worked (the form clears only then); `error` is its reason when it did not.
const props = defineProps<{ sources: readonly BriefSource[]; saving: boolean; error: string; add: (label: string, target: string) => Promise<boolean>; remove: (id: number) => Promise<boolean> }>()
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
  </div>
</template>
