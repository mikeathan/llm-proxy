<script setup lang="ts">
import { onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useDevices } from "../../composables/netaudit/useDevices"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useNetauditSwitch } from "../../composables/netaudit/useNetauditSwitch"
import { NetauditService } from "../../services/netaudit/netauditService"
import {
  DEVICE_SETTINGS_TITLE, IPV6_DETAILS, IPV6_FIX_COMMAND, IPV6_FIX_KEEP_COMMAND, IPV6_FIX_KEEP_NOTE, IPV6_HINT, IPV6_LABEL, IPV6_PROBLEMS,
  NAMES_DETAILS, NAMES_HINT, NAMES_LABEL, NAMES_LOAD_FAILED, NAMES_SAVE_FAILED, PRESENCE_DETAILS, PRESENCE_HINT,
} from "../../constants/netaudit"
import { intervalOptions, presenceLine } from "../../utils/netaudit/devices"

// Network · Settings · Devices: the three switches behind the device list — look for new devices, ask devices their names,
// and find the ones that only speak IPv6. The list itself, and what to do about a device, are on the Devices tab. Simple is
// the switches; Advanced adds how often the check looks.
const names = useNetauditSwitch(() => NetauditService.deviceNames(), (on) => NetauditService.setDeviceNames(on), { load: NAMES_LOAD_FAILED, save: NAMES_SAVE_FAILED }, true)
const { presence, ipv6, loadError, actionError, busy, load, setPresence, setIPv6 } = useDevices()
const { advanced } = useNetauditViewMode()

const toggle = (enabled: boolean) => setPresence(enabled, presence.value?.interval_minutes ?? 0)
const changeInterval = (value: string) => setPresence(presence.value?.enabled ?? false, Number(value))

onMounted(load)
</script>

<template>
  <Panel :title="DEVICE_SETTINGS_TITLE">
    <div class="flex flex-col gap-3" data-test="device-settings">
      <ErrorState v-if="loadError && !presence && !ipv6" title="The device settings could not be read" :cause="loadError" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!presence && !ipv6" label="Reading the device settings" />

      <template v-else>
        <section v-if="presence" class="flex flex-col gap-1" aria-label="New-device check">
          <ToggleField :model-value="presence.enabled" label="Look for new devices" :hint="PRESENCE_HINT" :details="PRESENCE_DETAILS" :disabled="busy" @update:model-value="toggle" />
          <p :class="['m-0 text-[length:var(--text-small)]', presence.problem && presence.enabled ? 'text-state-error' : 'text-muted']" data-test="presence-line">{{ presenceLine(presence) }}</p>
          <SelectInput
            v-if="advanced && presence.enabled"
            :model-value="String(presence.interval_minutes)"
            :options="intervalOptions(presence)"
            label="How often to look"
            @update:model-value="changeInterval"
          />
        </section>

        <section class="flex flex-col gap-1" aria-label="Device names" data-test="device-names">
          <ToggleField :model-value="names.enabled.value" :label="NAMES_LABEL" :hint="NAMES_HINT" :details="NAMES_DETAILS" :disabled="names.busy.value" @update:model-value="names.set" />
          <p v-if="names.error.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ names.error.value }}</p>
        </section>

        <section v-if="ipv6" class="flex flex-col gap-1" aria-label="IPv6 devices" data-test="ipv6-discovery">
          <ToggleField :model-value="ipv6.enabled" :label="IPV6_LABEL" :hint="IPV6_HINT" :details="IPV6_DETAILS" :disabled="busy" @update:model-value="setIPv6" />
          <div v-if="ipv6.enabled && ipv6.problem" role="alert" class="flex flex-col gap-1 text-[length:var(--text-small)] text-state-error" data-test="ipv6-problem">
            <p class="m-0">{{ IPV6_PROBLEMS[ipv6.problem] }}</p>
            <template v-if="ipv6.problem === 'not_permitted'">
              <code class="block break-all rounded-[var(--radius-md)] bg-canvas px-2 py-1 font-mono text-primary">{{ IPV6_FIX_COMMAND }}</code>
              <p class="m-0 text-muted">{{ IPV6_FIX_KEEP_NOTE }}</p>
              <code class="block break-all rounded-[var(--radius-md)] bg-canvas px-2 py-1 font-mono text-primary">{{ IPV6_FIX_KEEP_COMMAND }}</code>
            </template>
          </div>
        </section>

        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
      </template>
    </div>
  </Panel>
</template>
