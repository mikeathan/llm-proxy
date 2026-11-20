<script setup lang="ts">
import { computed } from "vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { safeWebLink } from "../../utils/newsroom/safeLink"
import { ALSO_SEEN_LABEL, CHIP_CHANGED, CHIP_NEW, UNREAD_LABEL } from "../../constants/newsroom"
import type { NewsroomEntry } from "../../types/newsroom"

// One finding: where it came from, when, its headline and one line of text, and chips for what kind of finding it is. Every piece of text is from the
// open internet, so it is shown as text and a link is opened only if it is a real http or https address (utils/newsroom/safeLink).
const props = defineProps<{ entry: NewsroomEntry }>()
const link = computed(() => safeWebLink(props.entry.link))
const changed = computed(() => Boolean(props.entry.was && props.entry.now))
</script>

<template>
  <article class="flex min-w-0 flex-col gap-1" :data-unread="entry.read ? undefined : 'true'">
    <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-[length:var(--text-small)] text-faint">
      <span v-if="!entry.read" class="sr-only">{{ UNREAD_LABEL }}</span>
      <span v-if="!entry.read" aria-hidden="true" class="h-1.5 w-1.5 flex-none bg-accent-brand"></span>
      <span class="truncate">{{ entry.source }}</span>
      <time :datetime="entry.at">{{ formatRelativeTime(entry.at) }}</time>
      <StatusTag v-if="changed" state="warning" :label="CHIP_CHANGED" />
      <StatusTag v-else state="info" :label="CHIP_NEW" />
    </div>
    <a v-if="link" :href="link" target="_blank" rel="noopener noreferrer" class="min-w-0 break-words font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ entry.title }}</a>
    <p v-else class="m-0 min-w-0 break-words font-medium text-primary">{{ entry.title }}</p>
    <p v-if="changed" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ entry.was }} → {{ entry.now }}</p>
    <p v-if="entry.text" class="m-0 break-words text-[length:var(--text-small)] text-muted">{{ entry.text }}</p>
    <p v-if="entry.also_seen?.length" class="m-0 break-words text-[length:var(--text-small)] text-faint">
      {{ ALSO_SEEN_LABEL }}:
      <template v-for="(seen, i) in entry.also_seen" :key="i">
        <a v-if="safeWebLink(seen.link)" :href="safeWebLink(seen.link) ?? undefined" target="_blank" rel="noopener noreferrer" class="hover:underline">{{ seen.source }}</a>
        <span v-else>{{ seen.source }}</span
        ><span v-if="i < (entry.also_seen?.length ?? 0) - 1">, </span>
      </template>
    </p>
  </article>
</template>
