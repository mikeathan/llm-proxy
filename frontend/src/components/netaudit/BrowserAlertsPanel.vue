<script setup lang="ts">
import { ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import { useBrowserAlerts } from "../../composables/netaudit/useBrowserAlerts"
import { BROWSER_ALERTS_HINT, BROWSER_ALERTS_LABEL, BROWSER_ALERTS_TITLE } from "../../constants/netaudit"

// Network · Overview · Notices in this browser: the opt-in for a browser notice when a network alert arrives while the tab is
// hidden. The browser is asked for permission only when the switch is turned on.
const { enabled, problem, supported, enable, disable } = useBrowserAlerts()

// A refused switch is drawn again, so it does not stay on while the setting is off.
const attempt = ref(0)
async function set(on: boolean) {
  if (on) await enable()
  else disable()
  if (enabled.value !== on) attempt.value++
}
</script>

<template>
  <Panel :title="BROWSER_ALERTS_TITLE">
    <div class="flex flex-col gap-2" data-test="browser-alerts">
      <ToggleField :key="attempt" :model-value="enabled" :label="BROWSER_ALERTS_LABEL" :hint="BROWSER_ALERTS_HINT" :disabled="!supported" @update:model-value="set" />
      <p v-if="problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="browser-alerts-problem">{{ problem }}</p>
    </div>
  </Panel>
</template>
