<script setup lang="ts">
import Panel from "../common/layout/Panel.vue"
import { toFinding } from "../../router/routes"
import { ATTACK_PATHS_FIX_LABEL, FINDING_LINK_LABEL, ATTACK_PATHS_MARK_HINT, ATTACK_PATHS_NONE, ATTACK_PATHS_TITLE } from "../../constants/netaudit"
import { breaksLine, prizeText, stepText } from "../../utils/netaudit/attackPaths"
import type { AttackPath } from "../../types/netaudit"

// The Overview's ways in: for each path where it starts, the real findings it passes through, what it ends at and the one fix.
// A path of findings the person accepted stays but is quieter. It adds no scanning; the server builds it from the last audit.
defineProps<{ paths: AttackPath[] }>()
</script>

<template>
  <Panel :title="ATTACK_PATHS_TITLE">
    <div class="flex flex-col gap-3" data-test="attack-paths">
      <template v-if="paths.length">
        <ol class="m-0 flex list-none flex-col gap-4 p-0">
          <li v-for="p in paths" :key="p.id" :class="['flex flex-col gap-1', p.all_accepted ? 'opacity-70' : '']" :data-test="`path-${p.id}`">
            <span class="text-primary">{{ p.entry.text }}</span>
            <ul class="m-0 flex list-none flex-col gap-0.5 p-0 pl-4">
              <li v-for="s in p.steps" :key="s.key" class="text-[length:var(--text-small)] text-secondary" data-test="path-step"><span aria-hidden="true" class="mr-1.5 text-muted">&rarr;</span><RouterLink :to="toFinding(s.key)" :aria-label="FINDING_LINK_LABEL(s.title)" class="text-secondary underline decoration-hairline underline-offset-2 hover:text-accent-info-text" data-test="path-step-link">{{ stepText(s) }}</RouterLink></li>
              <li class="text-[length:var(--text-small)] font-medium text-primary" data-test="path-prize"><span aria-hidden="true" class="mr-1.5 text-muted">&rarr;</span>{{ prizeText(p.prize) }}</li>
            </ul>
            <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="path-fix"><span class="font-medium text-secondary">{{ ATTACK_PATHS_FIX_LABEL }}:</span> {{ p.fix.text }} {{ breaksLine(p.breaks) }}</p>
          </li>
        </ol>
        <p class="m-0 text-[length:var(--text-small)] text-faint">{{ ATTACK_PATHS_MARK_HINT }}</p>
      </template>
      <p v-else class="m-0 text-muted" data-test="attack-paths-none">{{ ATTACK_PATHS_NONE }} <span class="text-faint">{{ ATTACK_PATHS_MARK_HINT }}</span></p>
    </div>
  </Panel>
</template>
