<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import Callout from "../common/feedback/Callout.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import DeviceRow from "./DeviceRow.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useDevices } from "../../composables/netaudit/useDevices"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  DEVICE_FILTER_LABEL,
  DEVICE_FILTER_LABELS,
  DEVICE_FILTERS,
  DEVICE_GROUP_TITLES,
  DEVICE_SEARCH_LABEL,
  DEVICE_SEARCH_PLACEHOLDER,
  DEVICES_BASELINE_NOTE,
  DEVICES_CHECK_OFF,
  DEVICES_EMPTY,
  DEVICES_NO_MATCH,
  DEVICES_TITLE,
  FORGET_CONFIRM,
  GATEWAY_CHANGED_BODY,
  GATEWAY_CHANGED_TITLE,
  UNNAMED_HINT,
  deviceCount,
} from "../../constants/netaudit"
import { deviceSummary, filterCounts, groupDevices, nameableDevices, presenceLine, udpCoverageLine } from "../../utils/netaudit/devices"
import type { DeviceFilter, DeviceStatus, NetworkDevice } from "../../types/netaudit"

// Network · Devices: who has been seen on the network. A search box and filter chips narrow the list; the devices fall into
// Needs a look, Online and Offline, each folded to one line. A device is one row that opens in place. Naming a device
// approves it. A new device is never hidden: it is in the first group, open, and All shows it. The switches that control the
// check are on the Settings tab. Simple shows names and one decision per device; Advanced adds hardware and IPv6 addresses
// and the open-port list.
const { inventory, presence, loadError, actionError, busy, load, name, approve, markImportant, forget, dismissSuggestion, confirmGateway } = useDevices()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

const filter = ref<DeviceFilter>("all")
const query = ref("")
const devices = computed(() => inventory.value?.devices ?? [])
const udpCoverage = computed(() => udpCoverageLine(devices.value))
const counts = computed(() => filterCounts(devices.value))
const filterOptions = computed(() => DEVICE_FILTERS.map((value) => ({ value, label: `${DEVICE_FILTER_LABELS[value]} (${counts.value[value]})` })))
const groups = computed(() => groupDevices(devices.value, filter.value, query.value))
const narrowed = computed(() => filter.value !== "all" || query.value.trim() !== "")
const showUnnamedHint = computed(() => filter.value === "unnamed" && nameableDevices(devices.value).length > 0)

// A group is open unless it is Offline, which is folded to a line; searching or filtering opens every group, because the person
// asked to see those devices. A fold the person made themselves wins.
const folded = ref<Partial<Record<DeviceStatus, boolean>>>({})
const isOpen = (status: DeviceStatus) => folded.value[status] ?? (narrowed.value || status !== "offline")

// Rows open themselves for a device that needs a decision, once, when the list first arrives, so claiming it is one tap; a row
// the person opens or closes is theirs, and approving the last new device does not fold anything under them.
const expanded = ref<string[]>([])
let seeded = false
watch(inventory, (inv) => {
  if (seeded || !inv) return
  seeded = true
  expanded.value = inv.devices.filter((d) => d.new).map((d) => d.mac)
}, { immediate: true })
const toggleRow = (mac: string) => {
  expanded.value = expanded.value.includes(mac) ? expanded.value.filter((m) => m !== mac) : [...expanded.value, mac]
}

const editing = ref<string | null>(null)
const draft = ref("")

function startNaming(device: NetworkDevice) {
  editing.value = device.mac
  draft.value = device.name ?? device.suggested_name ?? ""
  if (!expanded.value.includes(device.mac)) toggleRow(device.mac)
}

async function saveName(device: NetworkDevice) {
  if (await name(device.mac, draft.value)) editing.value = null
}

async function remove(device: NetworkDevice) {
  const ok = await confirm({ ...FORGET_CONFIRM, type: "warning" })
  if (ok) await forget(device.mac)
}

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
        <div v-if="presence" class="flex flex-col gap-0.5">
          <p :class="['m-0 text-[length:var(--text-small)]', presence.problem && presence.enabled ? 'text-state-error' : 'text-muted']" data-test="presence-line">{{ presenceLine(presence) }}</p>
          <p v-if="!presence.enabled && devices.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="check-off">{{ DEVICES_CHECK_OFF }}</p>
        </div>

        <Callout v-if="inventory?.gateway.changed" tone="error" :title="GATEWAY_CHANGED_TITLE" data-test="gateway-changed">
          {{ GATEWAY_CHANGED_BODY }}
          <template #actions>
            <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="busy" @click="confirmGateway">I know why: confirm</BaseButton>
          </template>
        </Callout>

        <p v-if="inventory && !inventory.baselined" class="m-0 text-[length:var(--text-small)] text-muted">{{ DEVICES_BASELINE_NOTE }}</p>

        <template v-if="inventory && devices.length">
          <p class="m-0 text-secondary" data-test="devices-summary">{{ deviceSummary(devices) }}</p>
          <p v-if="udpCoverage" class="m-0 text-[length:var(--text-small)] text-muted" data-test="udp-coverage">{{ udpCoverage }}</p>
          <div class="flex flex-col gap-2">
            <input
              v-model="query"
              type="search"
              autocomplete="off"
              spellcheck="false"
              :aria-label="DEVICE_SEARCH_LABEL"
              :placeholder="DEVICE_SEARCH_PLACEHOLDER"
              data-test="devices-search"
              class="h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"
            />
            <SegmentedControl v-model="filter" :options="filterOptions" :label="DEVICE_FILTER_LABEL" touch />
          </div>
          <p v-if="showUnnamedHint" class="m-0 text-[length:var(--text-small)] text-muted" data-test="unnamed-hint">{{ UNNAMED_HINT }}</p>

          <p v-if="!groups.length" class="m-0 text-secondary" data-test="devices-no-match">{{ DEVICES_NO_MATCH }}</p>
          <div v-else class="flex flex-col gap-2" data-test="devices-list">
            <Disclosure
              v-for="group in groups"
              :key="group.status"
              :title="DEVICE_GROUP_TITLES[group.status]"
              :summary="deviceCount(group.devices.length)"
              :open="isOpen(group.status)"
              :data-test="`group-${group.status}`"
              @update:open="folded[group.status] = $event"
            >
              <ul class="m-0 flex list-none flex-col p-0" :aria-label="DEVICE_GROUP_TITLES[group.status]">
                <DeviceRow
                  v-for="device in group.devices"
                  :key="device.mac"
                  :device="device"
                  :expanded="expanded.includes(device.mac)"
                  :editing="editing === device.mac"
                  :draft="draft"
                  :busy="busy"
                  :advanced="advanced"
                  @toggle="toggleRow(device.mac)"
                  @approve="approve(device.mac)"
                  @start-naming="startNaming(device)"
                  @save-name="saveName(device)"
                  @cancel-naming="editing = null"
                  @remove="remove(device)"
                  @toggle-important="markImportant(device.mac, !device.important)"
                  @accept-suggestion="name(device.mac, device.suggested_name ?? '')"
                  @dismiss-suggestion="dismissSuggestion(device.mac)"
                  @probed="load"
                  @update:draft="draft = $event"
                />
              </ul>
            </Disclosure>
          </div>
        </template>
        <p v-else-if="inventory" class="m-0 text-secondary" data-test="devices-empty">{{ DEVICES_EMPTY }}</p>

        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
        <p v-else-if="loadError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ loadError }}</p>
      </template>
    </div>
  </Panel>
</template>
