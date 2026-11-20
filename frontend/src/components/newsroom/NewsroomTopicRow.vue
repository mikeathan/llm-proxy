<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { compactInterval } from "../../utils/newsroom/interval"
import {
  FEED_ONLY_WORDS, INTERVAL_OPTIONS, lastCheckedWords, NEVER_CHECKED, PAUSED_LABEL, RUN_NOW_LABEL, sendsToWords, TOPIC_STATUS_TAG,
} from "../../constants/newsroom"
import type { NewsroomTopic } from "../../types/newsroom"

// One followed topic: its name, where it reads, how often, where it tells you, and what its last check did.
const props = defineProps<{ topic: NewsroomTopic; busy: boolean }>()
const emit = defineEmits<{ (e: "run"): void; (e: "edit"): void; (e: "delete"): void }>()

const sourceNames = computed(() => props.topic.sources.map((s) => s.label || s.target).join(", "))
const every = computed(() => {
  const short = compactInterval(props.topic.interval)
  return INTERVAL_OPTIONS.find((o) => o.value === short)?.label ?? short
})
const tag = computed(() => (props.topic.last_status ? TOPIC_STATUS_TAG[props.topic.last_status] : null))
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2">
    <div class="flex flex-wrap items-center gap-2">
      <h3 class="m-0 min-w-0 break-words text-[length:var(--text-body)] font-medium text-primary">{{ topic.name }}</h3>
      <StatusTag v-if="!topic.enabled" state="neutral" :label="PAUSED_LABEL" />
      <StatusTag v-else-if="tag" :state="tag.state" :label="tag.label" />
    </div>
    <p class="m-0 break-words text-[length:var(--text-small)] text-muted">{{ sourceNames }}</p>
    <p class="m-0 text-[length:var(--text-small)] text-faint">
      {{ every }} · {{ topic.channel ? sendsToWords(topic.channel) : FEED_ONLY_WORDS }} ·
      {{ topic.last_run ? lastCheckedWords(formatRelativeTime(topic.last_run)) : NEVER_CHECKED }}
    </p>
    <p v-if="topic.last_detail" class="m-0 break-words text-[length:var(--text-small)] text-faint">{{ topic.last_detail }}</p>
    <div class="flex flex-wrap gap-2">
      <BaseButton variant="secondary" size="sm" :disabled="busy || !topic.enabled" @click="emit('run')">{{ RUN_NOW_LABEL }}</BaseButton>
      <BaseButton variant="secondary" size="sm" :disabled="busy" @click="emit('edit')">Edit</BaseButton>
      <BaseButton variant="danger" size="sm" :disabled="busy" @click="emit('delete')">Delete</BaseButton>
    </div>
  </div>
</template>
