<script setup lang="ts">
import { ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Icon from "../icons/Icon.vue"
import NewsroomSourceArt from "./NewsroomSourceArt.vue"
import NewsroomSourceGuide from "./NewsroomSourceGuide.vue"
import {
  HELP_GOOD_TITLE, HELP_GOOD_TO_KNOW, HELP_INTRO, HELP_SOURCES_INTRO, HELP_SOURCES_TITLE, HELP_STEPS, HELP_STEPS_TITLE, HELP_TITLE, SOURCE_GUIDES,
} from "../../constants/newsroom"
import type { SourceKind } from "../../types/newsroom"

// The Help tab: what a topic is, one card per kind of source (art, what it is for, the steps on demand), and what the states mean.
const KINDS = Object.keys(SOURCE_GUIDES) as SourceKind[]
const opened = ref<SourceKind | null>(null)
const toggle = (k: SourceKind) => { opened.value = opened.value === k ? null : k }
</script>

<template>
  <div class="flex flex-col gap-4" data-test="newsroom-help">
    <Panel :title="HELP_TITLE">
      <div class="flex flex-col gap-4">
        <p class="m-0 text-[length:var(--text-body)] text-secondary">{{ HELP_INTRO }}</p>
        <div class="flex flex-col gap-2">
          <h3 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ HELP_STEPS_TITLE }}</h3>
          <ol class="m-0 flex list-decimal flex-col gap-1 pl-5 text-[length:var(--text-small)] text-secondary">
            <li v-for="s in HELP_STEPS" :key="s">{{ s }}</li>
          </ol>
        </div>
      </div>
    </Panel>

    <Panel :title="HELP_SOURCES_TITLE">
      <p class="m-0 mb-3 text-[length:var(--text-small)] text-muted">{{ HELP_SOURCES_INTRO }}</p>
      <ul class="m-0 flex list-none flex-col gap-2 p-0">
        <li v-for="k in KINDS" :key="k" class="rounded-[var(--radius-md)] border border-hairline bg-canvas" :data-test="'source-card-' + k">
          <button
            type="button"
            :aria-expanded="opened === k"
            class="flex w-full items-center gap-4 rounded-[var(--radius-md)] p-3 text-left hover:bg-surface-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-focus-ring"
            @click="toggle(k)"
          >
            <NewsroomSourceArt :kind="k" />
            <span class="flex min-w-0 flex-1 flex-col gap-0.5">
              <span class="text-[length:var(--text-body)] font-medium text-primary">{{ SOURCE_GUIDES[k].name }}</span>
              <span class="text-[length:var(--text-small)] text-secondary">{{ SOURCE_GUIDES[k].goodFor }}</span>
            </span>
            <Icon name="chevron-right" size="xs" :class="['flex-none text-muted transition-transform duration-[var(--motion-fast)]', opened === k ? 'rotate-90' : '']" />
          </button>
          <div v-show="opened === k" class="border-t border-hairline p-4"><NewsroomSourceGuide :kind="k" /></div>
        </li>
      </ul>
    </Panel>

    <Panel :title="HELP_GOOD_TITLE">
      <dl class="m-0 flex flex-col gap-3">
        <div v-for="s in HELP_GOOD_TO_KNOW" :key="s.title" class="flex flex-col gap-0.5">
          <dt class="text-[length:var(--text-small)] font-medium text-primary">{{ s.title }}</dt>
          <dd v-for="line in s.lines" :key="line" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ line }}</dd>
        </div>
      </dl>
    </Panel>
  </div>
</template>
