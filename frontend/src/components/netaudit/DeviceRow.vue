<script setup lang="ts">
import { computed, useId } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Icon from "../icons/Icon.vue"
import { DEVICE_IMPORTANT_NOTE, DEVICE_MARK_IMPORTANT, DEVICE_NAME_MAX, DEVICE_UNMARK_IMPORTANT, SUGGESTION_ACCEPT, SUGGESTION_APPROVED, SUGGESTION_DISMISS, SUGGESTION_NEW_DEVICE } from "../../constants/netaudit"
import { deviceAddress, deviceLabel, deviceSeen, deviceStatus, portsLine, portsSummary } from "../../utils/netaudit/devices"
import { formatRelativeTime } from "../../utils/format/time"
import type { NetworkDevice } from "../../types/netaudit"

// One device in the list: its name, address and status on one line, and everything else folded under it. Opening a row shows
// what can be done about the device (claim it, name it, remove it) in place, so there is no second screen. Advanced adds the
// hardware address, every IPv6 address and the list of open ports. Naming state is held by the list, so a saved name closes
// the form only once the server has accepted it.
const props = defineProps<{
  device: NetworkDevice
  expanded: boolean
  editing: boolean
  draft: string
  busy: boolean
  advanced: boolean
}>()
const emit = defineEmits<{
  (e: "toggle"): void
  (e: "approve"): void
  (e: "start-naming"): void
  (e: "save-name"): void
  (e: "cancel-naming"): void
  (e: "remove"): void
  (e: "toggle-important"): void
  (e: "accept-suggestion"): void
  (e: "dismiss-suggestion"): void
  (e: "update:draft", value: string): void
}>()

const bodyId = useId()
const status = computed(() => deviceStatus(props.device))
const ports = computed(() => (props.advanced ? portsLine(props.device) : portsSummary(props.device)))
const extraIPv6 = computed(() => (props.advanced && props.device.ip && props.device.ipv6?.length ? props.device.ipv6.join(", ") : ""))
</script>

<template>
  <li class="border-b border-hairline last:border-b-0" :data-test="`device-${device.mac}`">
    <button
      type="button"
      :aria-expanded="expanded"
      :aria-controls="expanded ? bodyId : undefined"
      class="flex min-h-11 w-full items-center gap-3 rounded-[var(--radius-md)] px-2 py-1.5 text-left hover:bg-surface-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-focus-ring"
      data-test="device-toggle"
      @click="emit('toggle')"
    >
      <Icon name="chevron-right" size="xs" :class-name="['flex-none text-muted transition-transform duration-[var(--motion-fast)]', expanded ? 'rotate-90' : ''].join(' ')" />
      <span class="flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:items-center sm:gap-3">
        <span class="flex min-w-0 flex-wrap items-center gap-2">
          <span class="min-w-0 break-words text-primary">{{ deviceLabel(device) }}</span>
          <StatusTag v-if="device.new" state="running" label="New" />
        </span>
        <span class="min-w-0 break-all text-[length:var(--text-small)] text-muted sm:ml-auto" data-test="device-line">{{ deviceAddress(device, false) }}</span>
      </span>
      <span class="flex flex-none items-center gap-1.5 text-[length:var(--text-small)] text-muted" data-test="device-status">
        <span aria-hidden="true" :class="['h-1.5 w-1.5 flex-none', status === 'online' ? 'bg-state-success' : 'bg-transparent shadow-[inset_0_0_0_1px_currentColor]']"></span>
        <span class="max-sm:sr-only">{{ status === "online" ? "Online" : deviceSeen(device) }}</span>
      </span>
    </button>

    <div v-if="expanded" :id="bodyId" class="flex flex-col gap-2 pb-3 pl-7 pr-2">
      <p class="m-0 text-[length:var(--text-small)] text-muted">
        First seen {{ formatRelativeTime(device.first_seen) }}
        <span v-if="advanced" class="font-mono"> · {{ device.mac }}</span>
      </p>
      <p v-if="device.vendor && !device.randomized" class="m-0 text-[length:var(--text-small)] text-muted" data-test="device-maker">Made by {{ device.vendor }}</p>
      <p v-if="advanced && !device.ip" class="m-0 break-all font-mono text-[length:var(--text-small)] text-muted" data-test="device-ipv6">{{ deviceAddress(device, true) }}</p>
      <p v-if="extraIPv6" class="m-0 break-all font-mono text-[length:var(--text-small)] text-muted" data-test="device-ipv6">{{ extraIPv6 }}</p>
      <p v-if="ports" class="m-0 text-[length:var(--text-small)] text-muted" data-test="device-ports">{{ ports }}</p>
      <p v-if="device.important" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="device-important">{{ DEVICE_IMPORTANT_NOTE }}</p>

      <p v-if="device.suggested_name && !device.name" class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary" data-test="suggestion">
        <span class="min-w-0 break-words">{{ device.approved ? SUGGESTION_APPROVED(device.suggested_name) : SUGGESTION_NEW_DEVICE(device.suggested_name) }}</span>
        <BaseButton v-if="device.approved" variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="emit('accept-suggestion')">{{ SUGGESTION_ACCEPT }}</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="emit('dismiss-suggestion')">{{ SUGGESTION_DISMISS }}</BaseButton>
      </p>

      <form v-if="editing" class="flex flex-wrap items-center gap-2" @submit.prevent="emit('save-name')">
        <input
          :value="draft"
          type="text"
          :maxlength="DEVICE_NAME_MAX"
          autocomplete="off"
          aria-label="Device name"
          class="h-8 min-w-0 flex-1 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"
          @input="emit('update:draft', ($event.target as HTMLInputElement).value)"
        />
        <BaseButton type="submit" size="sm" class-name="max-sm:!h-11" :loading="busy">Save</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('cancel-naming')">Cancel</BaseButton>
      </form>
      <div v-else class="flex flex-wrap gap-2">
        <BaseButton v-if="!device.approved" variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="emit('approve')">It's mine</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="emit('start-naming')">{{ device.name ? "Rename" : "Name it" }}</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="device-important-toggle" @click="emit('toggle-important')">{{ device.important ? DEVICE_UNMARK_IMPORTANT : DEVICE_MARK_IMPORTANT }}</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" @click="emit('remove')">Remove</BaseButton>
      </div>
    </div>
  </li>
</template>
