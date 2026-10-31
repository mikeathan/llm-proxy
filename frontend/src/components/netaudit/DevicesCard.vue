<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import Callout from "../common/feedback/Callout.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useDevices } from "../../composables/netaudit/useDevices"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useNetauditSwitch } from "../../composables/netaudit/useNetauditSwitch"
import { NetauditService } from "../../services/netaudit/netauditService"
import {
  DEVICES_BASELINE_NOTE,
  DEVICES_EMPTY,
  DEVICES_EMPTY_IPV4,
  DEVICES_EMPTY_IPV6_OFF,
  DEVICES_EMPTY_IPV6_ON,
  DEVICES_LIST_TITLE,
  FAMILY_TABS_LABEL,
  DEVICES_TITLE,
  DEVICE_NAME_MAX,
  FORGET_CONFIRM,
  GATEWAY_CHANGED_BODY,
  GATEWAY_CHANGED_TITLE,
  IPV6_FIX_COMMAND,
  IPV6_FIX_KEEP_COMMAND,
  IPV6_FIX_KEEP_NOTE,
  IPV6_HINT,
  IPV6_LABEL,
  IPV6_PROBLEMS,
  PRESENCE_HINT,
  NAMES_HINT,
  NAMES_LABEL,
  NAMES_LOAD_FAILED,
  NAMES_SAVE_FAILED,
  SUGGESTION_ACCEPT,
  SUGGESTION_DISMISS,
  UNNAMED_HINT,
  UNNAMED_SHOW,
  UNNAMED_SHOW_ALL,
  unnamedPrompt,
  SUGGESTION_APPROVED,
  SUGGESTION_NEW_DEVICE,
} from "../../constants/netaudit"
import { deviceDetail, deviceLabel, devicesIn, deviceSummary, familyOptions, intervalOptions, ipv6Detail, nameableDevices, portsLine, presenceLine } from "../../utils/netaudit/devices"
import type { DeviceFamily, NetworkDevice } from "../../types/netaudit"

// Settings · Network scan · Devices: who has been seen on the network, and the check that looks for a new one every few
// minutes. A device nobody approved shows first. Naming a device approves it. Simple shows names and one decision per
// device; Advanced adds the addresses and how often the check runs.
const names = useNetauditSwitch(() => NetauditService.deviceNames(), (on) => NetauditService.setDeviceNames(on), { load: NAMES_LOAD_FAILED, save: NAMES_SAVE_FAILED }, true)
const { inventory, presence, ipv6, loadError, actionError, busy, load, setPresence, setIPv6, name, approve, forget, dismissSuggestion, confirmGateway } = useDevices()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

const family = ref<DeviceFamily>("ipv4")
const devices = computed(() => inventory.value?.devices ?? [])
// The naming list: the unnamed devices naming can help, shown alone so each can be told apart and named in turn.
const unnamed = computed(() => nameableDevices(devices.value))
const namingList = ref(false)
watch(unnamed, (list) => { if (!list.length) namingList.value = false })
const shown = computed(() => (namingList.value ? unnamed.value : devicesIn(devices.value, family.value)))
const tabs = computed(() => familyOptions(devices.value))
const emptyText = computed(() => {
  if (devices.value.length === 0) return DEVICES_EMPTY
  if (family.value === "ipv4") return DEVICES_EMPTY_IPV4
  return ipv6.value?.enabled ? DEVICES_EMPTY_IPV6_ON : DEVICES_EMPTY_IPV6_OFF
})
const detail = (device: NetworkDevice) => (family.value === "ipv6" ? ipv6Detail(device, advanced.value) : deviceDetail(device))

// The list starts folded to one line, unless a device is new: that needs a decision and is never hidden behind a count. The
// choice is made once, when the list first arrives, so approving the last new device does not fold the list under the person.
const open = ref<boolean | null>(null)
watch(inventory, (inv) => { if (open.value === null && inv) open.value = inv.devices.some((d) => d.new) }, { immediate: true })

// Showing the naming list opens the device list, which starts folded when no device is new: the list is what was asked for.
function toggleNamingList() {
  namingList.value = !namingList.value
  if (namingList.value) open.value = true
}

const editing = ref<string | null>(null)
const draft = ref("")

function startNaming(device: NetworkDevice) {
  editing.value = device.mac
  draft.value = device.name ?? device.suggested_name ?? ""
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

        <section class="flex flex-col gap-1" aria-label="Device names" data-test="device-names">
          <ToggleField :model-value="names.enabled.value" :label="NAMES_LABEL" :hint="NAMES_HINT" :disabled="names.busy.value" @update:model-value="names.set" />
          <p v-if="names.error.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ names.error.value }}</p>
        </section>

        <section v-if="ipv6" class="flex flex-col gap-1" aria-label="IPv6 devices" data-test="ipv6-discovery">
          <ToggleField :model-value="ipv6.enabled" :label="IPV6_LABEL" :hint="IPV6_HINT" :disabled="busy" @update:model-value="setIPv6" />
          <div v-if="ipv6.enabled && ipv6.problem" role="alert" class="flex flex-col gap-1 text-[length:var(--text-small)] text-state-error" data-test="ipv6-problem">
            <p class="m-0">{{ IPV6_PROBLEMS[ipv6.problem] }}</p>
            <template v-if="ipv6.problem === 'not_permitted'">
              <code class="block break-all rounded-[var(--radius-md)] bg-canvas px-2 py-1 font-mono text-primary">{{ IPV6_FIX_COMMAND }}</code>
              <p class="m-0 text-muted">{{ IPV6_FIX_KEEP_NOTE }}</p>
              <code class="block break-all rounded-[var(--radius-md)] bg-canvas px-2 py-1 font-mono text-primary">{{ IPV6_FIX_KEEP_COMMAND }}</code>
            </template>
          </div>
        </section>

        <Callout v-if="inventory?.gateway.changed" tone="error" :title="GATEWAY_CHANGED_TITLE" data-test="gateway-changed">
          {{ GATEWAY_CHANGED_BODY }}
          <template #actions>
            <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="busy" @click="confirmGateway">I know why: confirm</BaseButton>
          </template>
        </Callout>

        <p v-if="inventory && !inventory.baselined" class="m-0 text-[length:var(--text-small)] text-muted">{{ DEVICES_BASELINE_NOTE }}</p>

        <div v-if="unnamed.length" class="flex flex-col gap-1" data-test="unnamed-prompt">
          <p class="m-0 flex flex-wrap items-center gap-2 text-secondary">
            <span>{{ unnamedPrompt(unnamed.length) }}</span>
            <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="unnamed-show" @click="toggleNamingList">{{ namingList ? UNNAMED_SHOW_ALL : UNNAMED_SHOW }}</BaseButton>
          </p>
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ UNNAMED_HINT }}</p>
        </div>

        <Disclosure v-if="inventory" :title="DEVICES_LIST_TITLE" :summary="deviceSummary(devices)" :open="open ?? false" data-test="devices-list" @update:open="open = $event">
          <SegmentedControl v-model="family" :options="tabs" :label="FAMILY_TABS_LABEL" touch />
          <p v-if="!shown.length" class="m-0 text-secondary" :data-test="`devices-empty-${family}`">{{ emptyText }}</p>

          <ul v-else class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Devices">
            <li v-for="device in shown" :key="device.mac" class="flex flex-col gap-1 border-b border-hairline pb-2 last:border-b-0" :data-test="`device-${device.mac}`">
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
              <p class="m-0 break-all text-[length:var(--text-small)] text-muted">
                {{ detail(device) }}
                <span v-if="advanced" class="font-mono"> · {{ device.mac }}</span>
                <span v-if="advanced && family === 'ipv4' && device.ipv6?.length" class="font-mono"> · {{ device.ipv6.join(", ") }}</span>
              </p>
              <p v-if="portsLine(device)" class="m-0 text-[length:var(--text-small)] text-muted" data-test="device-ports">{{ portsLine(device) }}</p>
              <p v-if="device.suggested_name && !device.name" class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary" data-test="suggestion">
                <span class="min-w-0 break-words">{{ device.approved ? SUGGESTION_APPROVED(device.suggested_name) : SUGGESTION_NEW_DEVICE(device.suggested_name) }}</span>
                <BaseButton v-if="device.approved" variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="name(device.mac, device.suggested_name)">{{ SUGGESTION_ACCEPT }}</BaseButton>
                <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="dismissSuggestion(device.mac)">{{ SUGGESTION_DISMISS }}</BaseButton>
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
        </Disclosure>

        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
        <p v-else-if="loadError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ loadError }}</p>
      </template>
    </div>
  </Panel>
</template>
