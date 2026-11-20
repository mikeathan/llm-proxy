<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Panel from "../common/layout/Panel.vue"
import { sourceRows } from "../../utils/newsroom/latest"
import { CHECK_SOURCE_LABEL, SOURCE_KIND_INFO, SOURCE_STATE_TAG, SOURCES_TITLE, readByWords } from "../../constants/newsroom"
import type { NewsroomTopic } from "../../types/newsroom"

// The places the Newsroom reads, each once, with whether the last check of the topics reading it worked. The server keeps a status per topic, so
// "Check now" checks the topics that read the source. All text is from the owner or the open internet and is shown as text.
const props = defineProps<{ topics: readonly NewsroomTopic[]; busy: boolean }>()
const emit = defineEmits<{ (e: "check", topic: NewsroomTopic): void }>()
const rows = computed(() => sourceRows(props.topics))
</script>

<template>
  <Panel :title="SOURCES_TITLE">
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Sources" data-test="newsroom-sources">
      <li v-for="r in rows" :key="r.key" class="flex flex-col gap-1 py-3 first:pt-0 last:pb-0" data-test="newsroom-source">
        <div class="flex flex-wrap items-center gap-2">
          <span class="min-w-0 break-words font-medium text-primary">{{ r.source.label || r.source.target }}</span>
          <StatusTag :state="SOURCE_STATE_TAG[r.state].state" :label="SOURCE_STATE_TAG[r.state].label" />
        </div>
        <p class="m-0 break-words text-[length:var(--text-small)] text-muted">{{ SOURCE_KIND_INFO[r.source.kind].label }} · {{ r.source.target }}</p>
        <p class="m-0 break-words text-[length:var(--text-small)] text-faint">{{ readByWords(r.topics.map((t) => t.name)) }}</p>
        <p v-if="r.detail" class="m-0 break-words text-[length:var(--text-small)] text-state-error">{{ r.detail }}</p>
        <div class="flex flex-wrap gap-2">
          <BaseButton v-for="t in r.topics.filter((x) => x.enabled)" :key="t.id" variant="secondary" size="sm" :disabled="busy" @click="emit('check', t)">
            {{ r.topics.length > 1 ? `${CHECK_SOURCE_LABEL}: ${t.name}` : CHECK_SOURCE_LABEL }}
          </BaseButton>
        </div>
      </li>
    </ul>
  </Panel>
</template>
