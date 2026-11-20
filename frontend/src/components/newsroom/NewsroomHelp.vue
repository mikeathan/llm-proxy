<script setup lang="ts">
import Panel from "../common/layout/Panel.vue"
import { HELP_SECTIONS, HELP_TITLE } from "../../constants/newsroom"

// The Help tab: what a topic is, where to find sources, what the states mean. Static words from constants, shown as text.
</script>

<template>
  <Panel :title="HELP_TITLE">
    <div class="flex flex-col gap-5" data-test="newsroom-help">
      <section v-for="s in HELP_SECTIONS" :key="s.title" class="flex flex-col gap-2">
        <h3 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ s.title }}</h3>
        <p v-for="line in s.lines" :key="line" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ line }}</p>
      </section>
    </div>
  </Panel>
</template>
