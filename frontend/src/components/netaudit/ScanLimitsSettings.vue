<script setup lang="ts">
import { computed, nextTick, watch } from "vue"
import { useRoute } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import FormField from "../common/forms/FormField.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { ADVANCED_SETTINGS_TITLE, DEVICE_TIME_OPTIONS, SCAN_TIME_ANCHOR, SCAN_UNCONFINED_DETAILS, SCAN_UNCONFINED_HINT } from "../../constants/netaudit"

// Network · Settings · Advanced: how long a scan may spend on one device and whether it may run without file confinement.
// Advanced only; it reads the scope ScanScopeSettings loads, so it draws nothing until that has arrived. Every change saves
// at once. A warning can send you straight to the scan-time control (/network/settings#scan-time).
const scope = useScanScope()
const { advanced } = useNetauditViewMode()
const route = useRoute()

const ready = computed(() => advanced.value && scope.scope.value !== null)

async function scrollToAnchor() {
  if (route.hash !== `#${SCAN_TIME_ANCHOR}`) return
  await nextTick()
  document.getElementById(SCAN_TIME_ANCHOR)?.scrollIntoView({ block: "start" })
}
watch(ready, (isReady) => { if (isReady) void scrollToAnchor() }, { immediate: true })
</script>

<template>
  <section v-if="ready && scope.scope.value" :aria-label="ADVANCED_SETTINGS_TITLE" class="flex flex-col gap-3" data-test="scan-limits">
    <h2 class="m-0 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted">{{ ADVANCED_SETTINGS_TITLE }}</h2>
    <Panel :id="SCAN_TIME_ANCHOR" title="Scan time">
      <FormField label="Time allowed per device" hint="Raise this if a slow device keeps showing as not scanned. A device that runs out of time is tried once more with twice as long. A whole scan may take up to 12 minutes.">
        <template #default="{ id, describedBy }">
          <SelectInput
            :id="id"
            :aria-describedby="describedBy"
            :model-value="String(scope.scope.value.device_timeout_minutes)"
            :options="DEVICE_TIME_OPTIONS"
            label="Time allowed per device"
            @update:model-value="scope.setDeviceTimeout(Number($event))"
          />
        </template>
      </FormField>
    </Panel>

    <Panel title="Confinement">
      <ToggleField
        :model-value="scope.scope.value.allow_unconfined"
        label="Allow scans without file confinement"
        :hint="SCAN_UNCONFINED_HINT"
        :details="SCAN_UNCONFINED_DETAILS"
        @update:model-value="scope.setAllowUnconfined"
      />
    </Panel>
  </section>
</template>
