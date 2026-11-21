<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { compactInterval } from "../../utils/newsroom/interval"
import {
  FEED_ONLY_WORDS, INTERVAL_OPTIONS, lastCheckedWords, lookingForWords, NEVER_CHECKED, PAUSED_LABEL, RUN_NOW_LABEL, RUNNING_LABEL, RUNNING_WORDS, sendsToWords, WATCH_STATUS_TAG,
} from "../../constants/newsroom"
import type { NewsroomWatch } from "../../types/newsroom"

// One followed watch: its name, where it reads, how often, where it tells you, and what its last check did.
const props = defineProps<{ watch: NewsroomWatch; busy: boolean; running?: boolean }>()
const emit = defineEmits<{ (e: "run"): void; (e: "edit"): void; (e: "delete"): void }>()

const sourceNames = computed(() => props.watch.sources.map((s) => s.label || s.target).join(", "))
const every = computed(() => {
  const short = compactInterval(props.watch.interval)
  return INTERVAL_OPTIONS.find((o) => o.value === short)?.label ?? short
})
const tag = computed(() => (props.watch.last_status ? WATCH_STATUS_TAG[props.watch.last_status] : null))
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2">
    <div class="flex flex-wrap items-center gap-2">
      <h3 class="m-0 min-w-0 break-words text-[length:var(--text-body)] font-medium text-primary">{{ watch.name }}</h3>
      <StatusTag v-if="!watch.enabled" state="neutral" :label="PAUSED_LABEL" />
      <StatusTag v-else-if="tag" :state="tag.state" :label="tag.label" />
    </div>
    <p class="m-0 break-words text-[length:var(--text-small)] text-muted">{{ sourceNames }}</p>
    <p v-if="watch.look_for" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ lookingForWords(watch.look_for) }}</p>
    <p class="m-0 text-[length:var(--text-small)] text-faint">
      {{ every }} · {{ watch.channel ? sendsToWords(watch.channel) : FEED_ONLY_WORDS }} ·
      {{ watch.last_run ? lastCheckedWords(formatRelativeTime(watch.last_run)) : NEVER_CHECKED }}
    </p>
    <p v-if="watch.last_detail" class="m-0 break-words text-[length:var(--text-small)] text-faint">{{ watch.last_detail }}</p>
    <p v-if="running" role="status" class="m-0 text-[length:var(--text-small)] text-secondary">{{ RUNNING_WORDS }}</p>
    <div class="flex flex-wrap gap-2">
      <BaseButton variant="secondary" size="sm" :loading="running" :disabled="busy || !watch.enabled" @click="emit('run')">{{ running ? RUNNING_LABEL : RUN_NOW_LABEL }}</BaseButton>
      <BaseButton variant="secondary" size="sm" :disabled="busy" @click="emit('edit')">Edit</BaseButton>
      <BaseButton variant="danger" size="sm" :disabled="busy" @click="emit('delete')">Delete</BaseButton>
    </div>
  </div>
</template>
