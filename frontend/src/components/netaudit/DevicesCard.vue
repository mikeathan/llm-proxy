<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import Callout from "../common/feedback/Callout.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useDevices } from "../../composables/netaudit/useDevices"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  DEVICES_BASELINE_NOTE,
  DEVICES_EMPTY,
  DEVICES_TITLE,
  DEVICE_NAME_MAX,
  FORGET_CONFIRM,
  GATEWAY_CHANGED_BODY,
  GATEWAY_CHANGED_TITLE,
  PRESENCE_HINT,
} from "../../constants/netaudit"
import { deviceDetail, deviceLabel, intervalOptions, presenceLine } from "../../utils/netaudit/devices"
import type { NetworkDevice } from "../../types/netaudit"

// Settings · Network scan · Devices: who has been seen on the network, and the check that looks for a new one every few
// minutes. A device nobody approved shows first. Naming a device approves it. Simple shows names and one decision per
// device; Advanced adds the addresses and how often the check runs.
const { inventory, presence, loadError, actionError, busy, load, setPresence, name, approve, forget, confirmGateway } = useDevices()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

const editing = ref<string | null>(null)
const draft = ref("")

function startNaming(device: NetworkDevice) {
  editing.value = device.mac
  draft.value = device.name ?? ""
}

async function saveName(device: NetworkDevice) {
  if (await name(device.mac, draft.value)) editing.value = null
}

async function remove(device: NetworkDevice) {
  const ok = await confirm({ ...FORGET_CONFIRM, type: "warning" })
  if (ok) await forget(device.mac)
}

const toggle = (enabled: boolean) => setPresence(enabled, presence.value?.interval_minutes ?? 0)
const changeInterval = (value: string) => setPresence(presence.value?.enabled ?? false, Number(value))

onMounted(load)
</script>

<template>
  <Panel :title="DEVICES_TITLE">
    <div class="flex flex-col gap-3" data-test="devices">
      <ErrorState v-if="loadError && !inventory && !presence" title="The devices could not be read" :cause="loadError" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!inventory && !presence" label="Reading the devices" />

      <template v-else>
        <section v-if="presence" class="flex flex-col gap-1" aria-label="New-device check">
          <ToggleField :model-value="presence.enabled" label="Look for new devices" :hint="PRESENCE_HINT" :disabled="busy" @update:model-value="toggle" />
          <p :class="['m-0 text-[length:var(--text-small)]', presence.problem && presence.enabled ? 'text-state-error' : 'text-muted']" data-test="presence-line">{{ presenceLine(presence) }}</p>
          <SelectInput
            v-if="advanced && presence.enabled"
            :model-value="String(presence.interval_minutes)"
            :options="intervalOptions(presence)"
            label="How often to look"
            @update:model-value="changeInterval"
          />
        </section>

        <Callout v-if="inventory?.gateway.changed" tone="error" :title="GATEWAY_CHANGED_TITLE" data-test="gateway-changed">
          {{ GATEWAY_CHANGED_BODY }}
          <template #actions>
            <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="busy" @click="confirmGateway">I know why: confirm</BaseButton>
          </template>
        </Callout>

        <p v-if="inventory && !inventory.baselined" class="m-0 text-[length:var(--text-small)] text-muted">{{ DEVICES_BASELINE_NOTE }}</p>
        <p v-if="inventory && !inventory.devices.length" class="m-0 text-secondary">{{ DEVICES_EMPTY }}</p>

        <ul v-if="inventory?.devices.length" class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Devices">
          <li v-for="device in inventory.devices" :key="device.mac" class="flex flex-col gap-1 border-b border-hairline pb-2 last:border-b-0" :data-test="`device-${device.mac}`">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="flex min-w-0 flex-wrap items-center gap-2">
                <span class="min-w-0 break-words text-primary">{{ deviceLabel(device) }}</span>
                <StatusTag v-if="device.new" state="running" label="New" />
              </span>
              <span v-if="editing !== device.mac" class="flex flex-wrap gap-2">
                <BaseButton v-if="!device.approved" variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="approve(device.mac)">It's mine</BaseButton>
                <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="startNaming(device)">{{ device.name ? "Rename" : "Name it" }}</BaseButton>
                <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="remove(device)">Remove</BaseButton>
              </span>
            </div>
            <p class="m-0 text-[length:var(--text-small)] text-muted">
              {{ deviceDetail(device) }}
              <span v-if="advanced" class="font-mono"> · {{ device.mac }}</span>
              <span v-if="advanced && device.ipv6?.length" class="break-all font-mono"> · {{ device.ipv6.join(", ") }}</span>
            </p>
            <form v-if="editing === device.mac" class="flex flex-wrap items-center gap-2" @submit.prevent="saveName(device)">
              <input
                v-model="draft"
                type="text"
                :maxlength="DEVICE_NAME_MAX"
                autocomplete="off"
                aria-label="Device name"
                class="h-8 min-w-0 flex-1 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"
              />
              <BaseButton type="submit" size="sm" class-name="max-sm:!h-11" :loading="busy">Save</BaseButton>
              <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="editing = null">Cancel</BaseButton>
            </form>
          </li>
        </ul>

        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
        <p v-else-if="loadError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ loadError }}</p>
      </template>
    </div>
  </Panel>
</template>
