<script setup lang="ts">
import { computed, ref } from "vue"
import Icon from "../icons/Icon.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { safeWebLink } from "../../utils/newsroom/safeLink"
import { textBeyondTitle } from "../../utils/newsroom/entryText"
import { entryHeadline } from "../../utils/newsroom/entryHeadline"
import { ALSO_SEEN_LABEL, CHIP_CHANGED, FIRST_CHECK_LABEL, HIDE_DETAILS_LABEL, NOT_FOR_ME_LABEL, SHOW_DETAILS_LABEL, UNREAD_LABEL, leftOutWords, notForMeAria, originalWords, postedWords, whyWords } from "../../constants/newsroom"
import type { NewsroomEntry } from "../../types/newsroom"

// One finding as a compact row: a dot while unread, where it came from and when, the headline (two lines at most), and a chevron that opens the reason,
// the rest of the text and the other reports of the same story. Every piece of text is from the open internet, so it is shown as text and a link is
// opened only if it is a real http or https address (utils/newsroom/safeLink). Opening the link or the details counts as reading it. An item the model left
// out says why on the row itself: that reason is what the owner checks.
// canSkip shows "Not for me" on the row: the owner's verdict that the alert is not what the watch is for (it teaches the watch's model).
const props = defineProps<{ entry: NewsroomEntry; watchName?: string; selectable?: boolean; selected?: boolean; canSkip?: boolean }>()
const emit = defineEmits<{ (e: "toggle-select"): void; (e: "open"): void; (e: "skip"): void }>()
const link = computed(() => safeWebLink(props.entry.link))
const changed = computed(() => Boolean(props.entry.was && props.entry.now))
// The model's headline stands in for the original title in the row; the original is kept and shown when the entry is opened.
const headline = computed(() => entryHeadline(props.entry))
const hasHeadline = computed(() => headline.value !== props.entry.title)
const extraText = computed(() => textBeyondTitle(props.entry.title, props.entry.text))
const whyInDetails = computed(() => Boolean(props.entry.why && !props.entry.left_out))
const hasDetails = computed(() => Boolean(changed.value || props.entry.summary || hasHeadline.value || whyInDetails.value || extraText.value || props.entry.also_seen?.length))
const open = ref(false)
function toggle() {
  open.value = !open.value
  if (open.value) emit("open")
}
</script>

<template>
  <article class="flex min-w-0 items-start gap-2 py-3" :class="selected ? 'bg-surface-hover' : ''" :data-unread="entry.read ? undefined : 'true'">
    <input
      v-if="selectable"
      type="checkbox"
      :checked="selected"
      :aria-label="`Select ${headline}`"
      class="mt-0.5 h-4 w-4 flex-none cursor-pointer focus-visible:outline-none focus-visible:ring-2 sm:h-3.5 sm:w-3.5"
      @change="emit('toggle-select')"
    />
    <span class="mt-[7px] h-1.5 w-1.5 flex-none" :class="entry.read ? '' : 'bg-accent-brand'" aria-hidden="true"></span>
    <span v-if="!entry.read" class="sr-only">{{ UNREAD_LABEL }}</span>
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-[length:var(--text-small)] text-faint">
        <span class="truncate">{{ entry.source }}</span>
        <span v-if="watchName" class="truncate">· {{ watchName }}</span>
        <time :datetime="entry.posted ?? entry.at">{{ entry.posted ? postedWords(formatRelativeTime(entry.posted)) : formatRelativeTime(entry.at) }}</time>
        <StatusTag v-if="entry.first" state="neutral" :label="FIRST_CHECK_LABEL" />
        <StatusTag v-else-if="changed" state="warning" :label="CHIP_CHANGED" />
      </div>
      <a
        v-if="link"
        :href="link"
        target="_blank"
        rel="noopener noreferrer"
        class="line-clamp-2 min-w-0 break-words font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2"
        :class="entry.read ? 'font-normal text-secondary' : ''"
        @click="emit('open')"
        @auxclick="emit('open')"
      >{{ headline }}</a>
      <p v-else class="m-0 line-clamp-2 min-w-0 break-words font-medium text-primary" :class="entry.read ? 'font-normal text-secondary' : ''">{{ headline }}</p>
      <p v-if="entry.left_out && entry.why" class="m-0 break-words text-[length:var(--text-small)] text-secondary" data-test="left-out-why">{{ leftOutWords(entry.why) }}</p>
      <div v-if="open" class="flex flex-col gap-1 border-l border-hairline pl-3">
        <p v-if="changed" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ entry.was }} → {{ entry.now }}</p>
        <p v-if="entry.summary" class="m-0 break-words text-[length:var(--text-small)] text-primary">{{ entry.summary }}</p>
        <p v-if="whyInDetails" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ whyWords(entry.why ?? "") }}</p>
        <p v-if="hasHeadline" class="m-0 break-words text-[length:var(--text-small)] text-faint">{{ originalWords(entry.title) }}</p>
        <p v-if="extraText" class="m-0 whitespace-pre-line break-words text-[length:var(--text-small)] text-muted">{{ extraText }}</p>
        <p v-if="entry.also_seen?.length" class="m-0 break-words text-[length:var(--text-small)] text-faint">
          {{ ALSO_SEEN_LABEL }}:
          <template v-for="(seen, i) in entry.also_seen" :key="i">
            <a v-if="safeWebLink(seen.link)" :href="safeWebLink(seen.link) ?? undefined" target="_blank" rel="noopener noreferrer" class="hover:underline">{{ seen.source }}</a>
            <span v-else>{{ seen.source }}</span
            ><span v-if="i < (entry.also_seen?.length ?? 0) - 1">, </span>
          </template>
        </p>
      </div>
    </div>
    <!-- Said in words on every alert, so the way to teach the watch is seen, not known; its own 44 px box beside the chevron, clear of the headline link. -->
    <button
      v-if="canSkip"
      type="button"
      class="flex min-h-11 flex-none items-center self-center border-0 bg-transparent px-2 text-[length:var(--text-small)] text-faint hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2"
      :aria-label="notForMeAria(headline)"
      data-test="not-for-me"
      @click="emit('skip')"
    >{{ NOT_FOR_ME_LABEL }}</button>
    <button
      v-if="hasDetails"
      type="button"
      class="mt-0.5 flex-none border-0 bg-transparent p-1 text-faint hover:text-primary focus-visible:outline-none focus-visible:ring-2"
      :aria-expanded="open"
      :aria-label="open ? HIDE_DETAILS_LABEL : SHOW_DETAILS_LABEL"
      @click="toggle"
    >
      <Icon :name="open ? 'chevron-down' : 'chevron-right'" size="sm" />
    </button>
  </article>
</template>
