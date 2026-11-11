<script setup lang="ts">
import { computed } from "vue"
import { ownerLine } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_OPEN_PLACE, TRAFFIC_OTHERS, TRAFFIC_TALK_DIRECTION, TRAFFIC_TALK_DIRECTION_LABEL, TRAFFIC_TALK_GROUPS, TRAFFIC_TALK_NOTHING, TRAFFIC_TALK_SHARE,
  TRAFFIC_TALK_SHARED_NOTE,
} from "../../constants/netaudit"
import type { TalkGroup, TalkRow } from "../../types/netaudit"

// Who the computer talked to, one row per place, in three groups: the internet, devices on this network, and broadcast chatter (closed until
// opened: it is mostly discovery noise). A row says what it was and which way the bytes went, and opens the detail of the place. Names are text a
// peer chose, so they are shown as text only.
const props = defineProps<{ groups: readonly TalkGroup[]; total: number; openAddress: string | null; otherFlows: number; otherBytes: number }>()
const emit = defineEmits<{ (e: "open", address: string): void }>()

const visible = computed(() => props.groups.filter((g) => g.rows.length > 0))
const percent = (bytes: number) => (props.total > 0 ? Math.round((bytes / props.total) * 100) : 0)
const biggest = (g: TalkGroup) => Math.max(1, ...g.rows.map((r) => r.bytes))
const barWidth = (bytes: number, max: number) => `${Math.max(2, Math.round((bytes / max) * 100))}%`
// The second line: what it was, then who runs it and where (internet) or the address (a device shown by its name).
const detail = (r: TalkRow): string =>
  [r.services.slice(0, 3).join(", "), r.group === "internet" ? ownerLine(r.owner) : r.named ? r.address : ""].filter(Boolean).join(" · ")
const hasDirection = (r: TalkRow) => r.sent + r.received > 0
</script>

<template>
  <p v-if="!visible.length" class="m-0 text-secondary" data-test="talk-nothing">{{ TRAFFIC_TALK_NOTHING }}</p>
  <div v-else class="flex flex-col gap-5" data-test="talk">
    <component
      :is="g.id === 'shared' ? 'details' : 'section'"
      v-for="g in visible"
      :key="g.id"
      :aria-label="TRAFFIC_TALK_GROUPS[g.id].title"
      class="group"
      :data-test="`talk-${g.id}`"
    >
      <component :is="g.id === 'shared' ? 'summary' : 'div'" class="mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5" :class="g.id === 'shared' ? 'cursor-pointer list-none max-sm:min-h-11' : ''">
        <span v-if="g.id === 'shared'" class="mr-1 inline-block text-muted transition-transform group-open:rotate-90" aria-hidden="true">›</span>
        <h3 class="m-0 inline font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ TRAFFIC_TALK_GROUPS[g.id].title }}</h3>
        <span class="text-[length:var(--text-small)] text-muted" data-test="talk-share">{{ TRAFFIC_TALK_SHARE(formatBytes(g.bytes), percent(g.bytes)) }}</span>
      </component>
      <p v-if="g.id === 'shared'" class="m-0 mb-2 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_TALK_SHARED_NOTE }}</p>
      <ul class="m-0 flex list-none flex-col gap-3 p-0">
        <li v-for="r in g.rows" :key="r.address" data-test="talk-row">
          <button
            type="button"
            class="flex w-full flex-col gap-1 bg-transparent p-0 text-left max-sm:min-h-11"
            :aria-label="TRAFFIC_OPEN_PLACE(r.label)"
            :aria-expanded="openAddress === r.address"
            data-test="talk-open"
            @click="emit('open', r.address)"
          >
            <span class="flex w-full items-baseline justify-between gap-3">
              <span class="min-w-0 break-all" :class="openAddress === r.address ? 'font-medium text-primary' : 'text-primary'" data-test="talk-name">{{ r.label }}</span>
              <span class="flex-none text-[length:var(--text-small)] text-muted">{{ formatBytes(r.bytes) }}</span>
            </span>
            <span v-if="detail(r)" class="min-w-0 break-words text-[length:var(--text-small)] text-muted" data-test="talk-detail">{{ detail(r) }}</span>
            <span class="flex items-center gap-3">
              <span class="block h-1.5 flex-1 bg-surface-hover" aria-hidden="true"><span class="block h-1.5 bg-accent-info" :style="{ width: barWidth(r.bytes, biggest(g)) }"></span></span>
              <span
                v-if="hasDirection(r)"
                class="flex-none whitespace-pre font-mono text-[length:var(--text-micro)] text-muted"
                :aria-label="TRAFFIC_TALK_DIRECTION_LABEL(formatBytes(r.sent), formatBytes(r.received))"
                data-test="talk-direction"
              >{{ TRAFFIC_TALK_DIRECTION(formatBytes(r.sent), formatBytes(r.received)) }}</span>
            </span>
          </button>
        </li>
      </ul>
    </component>
    <p v-if="otherFlows" class="m-0 text-[length:var(--text-small)] text-muted" data-test="talk-other">{{ TRAFFIC_OTHERS(otherFlows) }} · {{ formatBytes(otherBytes) }}</p>
  </div>
</template>
