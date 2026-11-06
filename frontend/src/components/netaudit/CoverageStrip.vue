<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useModuleSources } from "../../composables/netaudit/useModuleSources"
import { COVERAGE_ALL_ON, COVERAGE_OFF_INTRO, COVERAGE_OPEN_MODULES, coverageLine } from "../../constants/stacks"
import type { NetworkTab } from "../../types/routes"

// What is watching and what is not, on the first screen: "14 of 16 protections are watching", and the ones that are off, each with
// the one sentence of what it would watch. Everything comes from the same description the Modules tab reads. The page owns
// navigation; this card only says where to go.
const emit = defineEmits<{ (e: "open-tab", tab: NetworkTab): void }>()
const { modules, load } = useModuleSources()
const open = ref(false)
const off = computed(() => modules.value.filter((m) => !m.enabled))
const stopped = computed(() => modules.value.filter((m) => m.enabled && m.health === "stopped"))
const line = computed(() => {
  const parts = [coverageLine(modules.value.length - off.value.length, modules.value.length)]
  if (stopped.value.length) parts.push(`${stopped.value.length} stopped`)
  return parts.join(" · ")
})
onMounted(load)
</script>

<template>
  <Panel v-if="modules.length" data-test="coverage">
    <div class="flex flex-col gap-2">
      <Disclosure :title="line" :open="open" :expandable="off.length > 0 || stopped.length > 0" data-test="coverage-line" @update:open="open = $event">
        <div class="flex flex-col gap-2">
          <ul v-if="stopped.length" class="m-0 flex list-none flex-col gap-1 p-0" data-test="coverage-stopped">
            <li v-for="m in stopped" :key="m.id" class="text-secondary"><span class="font-medium text-primary">{{ m.title }}</span> has stopped. Open the Modules tab to see why.</li>
          </ul>
          <p v-if="off.length" class="m-0 text-muted">{{ COVERAGE_OFF_INTRO }}</p>
          <ul class="m-0 flex list-none flex-col gap-2 p-0" data-test="coverage-off">
            <li v-for="m in off" :key="m.id" class="text-secondary">
              <span class="font-medium text-primary">{{ m.title }}</span>: {{ m.about.watches }}
            </li>
          </ul>
          <div><BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('open-tab', 'modules')">{{ COVERAGE_OPEN_MODULES }}</BaseButton></div>
        </div>
      </Disclosure>
      <p v-if="!off.length && !stopped.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="coverage-all-on">{{ COVERAGE_ALL_ON }}</p>
    </div>
  </Panel>
</template>
