<script setup lang="ts">
import { computed, ref } from "vue"
import { RouterLink } from "vue-router"
import Icon from "../icons/Icon.vue"
import NewsroomPlate from "./NewsroomPlate.vue"
import { toNetwork, toRadio } from "../../router/routes"
import { HOME_BLOCK_LABEL, HOME_COUNTS_ONLY, HOME_FOLD_LABEL, HOME_OPEN_LABEL, homeStateWords } from "../../constants/newsroom"
import { homeLineRoute, splitHome } from "../../utils/newsroom/homeLines"
import type { BriefHomeLine } from "../../types/newsroom"

// What the core knows about the home network and Radio: a count and the core's own sentence for each fact, ranked by the core (ADR-0084). It carries no device name,
// address or network name by design, so the block says so and links to the pages that do. The text is shown as text. A block with nothing to look at folds to one
// line; one with something opens, the facts that need a look first, each a link to the page where it is acted on, and the routine ones below, quietly.
const props = defineProps<{ lines: BriefHomeLine[] }>()
const parts = computed(() => splitHome(props.lines))
const attention = computed(() => [...parts.value.critical, ...parts.value.check])
const state = computed(() => (parts.value.critical.length ? "critical" : parts.value.check.length ? "check" : "quiet"))
// Folded by default only when nothing needs a look; a press overrides, until the lines change.
const picked = ref<boolean | null>(null)
const open = computed(() => picked.value ?? attention.value.length > 0)
const toggle = () => (picked.value = !open.value)
</script>

<template>
  <section
    :class="['flex min-w-0 flex-col rounded-[var(--radius-md)] bg-surface-raised', state === 'critical' ? 'border border-state-error' : '']"
    :aria-label="HOME_BLOCK_LABEL"
    :data-state="state"
    data-test="home-block"
  >
    <button
      type="button"
      class="flex w-full min-w-0 items-center gap-3 rounded-[var(--radius-md)] border-0 bg-transparent p-3 text-left focus-visible:outline-none focus-visible:ring-2"
      :aria-expanded="open"
      :aria-label="open ? HOME_FOLD_LABEL : HOME_OPEN_LABEL"
      data-test="home-toggle"
      @click="toggle"
    >
      <NewsroomPlate :label="HOME_BLOCK_LABEL" kind="home" />
      <span class="flex min-w-0 flex-1 flex-col">
        <span class="text-[length:var(--text-body)] font-medium text-primary">{{ HOME_BLOCK_LABEL }}</span>
        <span :class="['text-[length:var(--text-small)]', state === 'critical' ? 'text-state-error' : 'text-faint']" data-test="home-state">{{ homeStateWords(parts.critical.length, parts.check.length) }}</span>
      </span>
      <Icon name="chevron-down" size="xs" :class-name="['flex-none text-faint transition-transform duration-fast', open ? '' : '-rotate-90'].join(' ')" />
    </button>
    <div v-if="open" class="flex min-w-0 flex-col border-t border-hairline px-3" data-test="home-lines">
      <ul v-if="attention.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="l in attention" :key="l.id">
          <RouterLink
            :to="homeLineRoute(l)"
            class="-mx-2 flex min-w-0 items-start gap-3 rounded-[var(--radius-md)] px-2 py-2 no-underline hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
            :data-level="l.level"
            data-test="home-attention"
          >
            <span v-if="l.count" class="min-w-8 flex-none text-right font-medium tabular-nums text-primary">{{ l.count }}</span>
            <span v-else class="min-w-8 flex-none" aria-hidden="true"></span>
            <span :class="['min-w-0 flex-1 break-words text-[length:var(--text-small)]', l.level === 'critical' ? 'text-state-error' : 'text-secondary']">{{ l.words }}</span>
            <Icon name="chevron-right" size="xs" class-name="mt-1 flex-none text-faint" />
          </RouterLink>
        </li>
      </ul>
      <ul v-if="parts.routine.length" :class="['m-0 flex list-none flex-col p-0', attention.length ? 'border-t border-hairline' : '']" data-test="home-routine">
        <li v-for="l in parts.routine" :key="l.id" class="flex min-w-0 items-baseline gap-3 py-1.5 text-[length:var(--text-small)] text-faint">
          <span v-if="l.count" class="min-w-8 flex-none text-right tabular-nums">{{ l.count }}</span>
          <span v-else class="min-w-8 flex-none" aria-hidden="true"></span>
          <span class="min-w-0 break-words">{{ l.words }}</span>
        </li>
      </ul>
    </div>
    <p v-if="open" class="m-0 flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t border-hairline p-3 text-[length:var(--text-small)] text-faint">
      <span>{{ HOME_COUNTS_ONLY }}</span>
      <span class="flex gap-4">
        <RouterLink :to="toNetwork()" class="text-accent-info-text hover:underline">Open Network</RouterLink>
        <RouterLink :to="toRadio()" class="text-accent-info-text hover:underline">Open Radio</RouterLink>
      </span>
    </p>
    <p v-else class="sr-only">{{ HOME_COUNTS_ONLY }}</p>
  </section>
</template>
