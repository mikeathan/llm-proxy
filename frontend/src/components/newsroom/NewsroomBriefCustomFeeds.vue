<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { NewsroomService } from "../../services/newsroom/newsroomService"
import { FEED_CHECKING, FEED_CHECK_NOTE, FEED_NOT_A_FEED, FEED_OFFERS, FEED_SAVE_LABEL, LOOKUP_FAILED, WATCH_NAME_MAX, feedFoundWords } from "../../constants/newsroom"
import type { SourceChoice } from "../../types/newsroom"

// The feed links of a topic that are in neither list (pasted by the owner): each is read once with the watch form's look-up, so the owner sees what it
// gives before saving, and can keep it in "My sources". The check is a hint and never blocks the topic. A page that is not a feed offers the feeds it
// advertises, and a pick replaces the link. Everything shown came from the internet, so it is shown as text.
type Check = { state: "checking" } | { state: "found"; title: string; count: number } | { state: "problem"; problem: string; choices: SourceChoice[] }
const props = defineProps<{ feeds: readonly string[]; saving: boolean; save?: (label: string, target: string) => Promise<boolean> }>()
const emit = defineEmits<{ (e: "replace", from: string, to: string): void }>()

const checks = ref<Record<string, Check>>({})
const controllers = new Map<string, AbortController>()

async function check(url: string) {
  const ctl = new AbortController()
  controllers.set(url, ctl)
  checks.value = { ...checks.value, [url]: { state: "checking" } }
  let next: Check
  try {
    const got = await NewsroomService.lookup({ kind: "feed", target: url }, ctl.signal)
    next = got.found ? { state: "found", title: got.title ?? "", count: got.sample.length } : { state: "problem", problem: got.problem || FEED_NOT_A_FEED, choices: got.choices ?? [] }
  } catch (e) {
    if (ctl.signal.aborted) return
    next = { state: "problem", problem: e instanceof Error && e.message ? e.message : LOOKUP_FAILED, choices: [] }
  }
  if (!ctl.signal.aborted) checks.value = { ...checks.value, [url]: next }
}

// A link is checked once while it stays in the list; one taken out is forgotten, so putting it back checks it again.
watch(
  () => props.feeds,
  (now) => {
    for (const url of now) if (!(url in checks.value)) void check(url)
    for (const url of Object.keys(checks.value)) {
      if (now.includes(url)) continue
      controllers.get(url)?.abort()
      controllers.delete(url)
      const { [url]: _gone, ...rest } = checks.value
      checks.value = rest
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => controllers.forEach((c) => c.abort()))

const hostOf = (url: string): string => {
  try {
    return new URL(url).hostname
  } catch {
    return url
  }
}
const nameFor = (url: string, c: Check): string => (c.state === "found" && c.title.trim() ? c.title.trim() : hostOf(url)).slice(0, WATCH_NAME_MAX)
const asFound = (c: Check | undefined) => (c?.state === "found" ? c : null)
const asProblem = (c: Check | undefined) => (c?.state === "problem" ? c : null)
</script>

<template>
  <ul class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Links to check" data-test="custom-feeds">
    <li v-for="url in feeds" :key="url" class="flex flex-col gap-1 rounded-[var(--radius-md)] border border-hairline p-2 text-[length:var(--text-small)]" data-test="custom-feed">
      <span class="break-all text-secondary">{{ url }}</span>
      <span v-if="!checks[url] || checks[url]!.state === 'checking'" class="text-faint" role="status">{{ FEED_CHECKING }}</span>
      <template v-else-if="asFound(checks[url])">
        <span class="text-state-success" role="status">{{ feedFoundWords(asFound(checks[url])!.title, asFound(checks[url])!.count) }}</span>
        <div v-if="save"><BaseButton variant="secondary" size="sm" :disabled="saving" @click="save(nameFor(url, checks[url]!), url)">{{ FEED_SAVE_LABEL }}</BaseButton></div>
      </template>
      <template v-else-if="asProblem(checks[url])">
        <span class="break-words text-state-error" role="status">{{ asProblem(checks[url])!.problem }}</span>
        <div v-if="asProblem(checks[url])!.choices.length" class="flex flex-col gap-1">
          <span class="text-secondary">{{ FEED_OFFERS }}</span>
          <div class="flex flex-wrap gap-1.5">
            <BaseButton v-for="c in asProblem(checks[url])!.choices" :key="c.value" variant="secondary" size="sm" :title="c.value" @click="emit('replace', url, c.value)">{{ c.label }}</BaseButton>
          </div>
        </div>
      </template>
      <span class="text-faint">{{ FEED_CHECK_NOTE }}</span>
    </li>
  </ul>
</template>
