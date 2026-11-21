<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import { CARD_LIST_SCROLL } from "../../../constants/dashboard"
import { CARD_FAILING_WORDS, CARD_HEADLINES_EMPTY, CARD_NO_WATCHES, NEWSROOM_CARD_RELOAD, NEWSROOM_LOAD_FAILED } from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { usePolling } from "../../../composables/ui/usePolling"
import { NewsroomService } from "../../../services/newsroom/newsroomService"
import { browserTimeZone } from "../../../utils/format/timezone"
import { formatRelativeTime } from "../../../utils/format/time"
import { failingWatches, latestHeadlines } from "../../../utils/newsroom/latest"
import { safeWebLink } from "../../../utils/newsroom/safeLink"
import type { NewsroomDay, NewsroomWatch } from "../../../types/newsroom"

// Dashboard card · Newsroom: the newest headlines from the watches you follow, and a flag when a watch cannot read a source. It only shows and links;
// following watches, checking and reading the whole feed are on the Newsroom page (ADR-0064). Small is one headline; larger ones a list that scrolls.
const REFRESH_MS = 60000
const SHOWN = 20
const size = useCardSize()
const watches = ref<NewsroomWatch[] | null>(null)
const days = ref<NewsroomDay[]>([])
const error = ref<string | null>(null)

async function load() {
  try {
    const [t, d] = await Promise.all([NewsroomService.watches(), NewsroomService.feed({ watch: null, unreadOnly: false }, browserTimeZone())])
    watches.value = t
    days.value = d
    error.value = null
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : NEWSROOM_LOAD_FAILED
  }
}
onMounted(() => { void load() })
usePolling(() => { void load() }, REFRESH_MS)

const headlines = computed(() => latestHeadlines(days.value, size.value === "xs" ? 1 : SHOWN))
const failing = computed(() => failingWatches(watches.value ?? []))
</script>

<template>
  <div class="flex flex-col gap-3" data-test="newsroom-card">
    <ErrorState v-if="error && !watches" title="The Newsroom could not be read" :cause="error" :next="NEWSROOM_CARD_RELOAD" />
    <LoadingState v-else-if="!watches" label="Reading the Newsroom" :rows="2" />
    <template v-else>
      <p v-if="failing.length" class="m-0 flex flex-wrap items-center gap-2 text-primary" data-test="newsroom-failing">
        <StatusTag state="error" label="Source failing" /> {{ CARD_FAILING_WORDS(failing.map((t) => t.name)) }}
      </p>
      <p v-if="!watches.length" class="m-0 text-muted" data-test="newsroom-empty">{{ CARD_NO_WATCHES }}</p>
      <p v-else-if="!headlines.length" class="m-0 text-muted" data-test="newsroom-empty">{{ CARD_HEADLINES_EMPTY }}</p>
      <ul v-else :class="['m-0 flex list-none flex-col p-0', CARD_LIST_SCROLL[size]]" aria-label="Latest headlines">
        <li v-for="h in headlines" :key="h.entry.id" class="flex flex-col gap-0.5 border-b border-hairline/60 py-2 last:border-b-0" data-test="newsroom-headline">
          <a v-if="safeWebLink(h.entry.link)" :href="safeWebLink(h.entry.link) ?? undefined" target="_blank" rel="noopener noreferrer" class="break-words text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ h.entry.title }}</a>
          <span v-else class="break-words text-primary">{{ h.entry.title }}</span>
          <span class="flex flex-wrap gap-x-2 text-[length:var(--text-small)] text-muted">
            <span class="break-words">{{ h.watch }}</span>
            <time :datetime="h.entry.at">{{ formatRelativeTime(h.entry.at) }}</time>
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>
