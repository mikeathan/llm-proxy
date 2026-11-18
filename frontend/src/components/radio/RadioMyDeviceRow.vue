<script setup lang="ts">
import { computed } from "vue"
import Icon from "../icons/Icon.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import RadioPresenceStrip from "./RadioPresenceStrip.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { hoursLine } from "../../utils/radio/myDevices"
import type { MyDeviceRow } from "../../types/radio"
import { toRadioDevice } from "../../router/routes"
import { RADIO_KIND_ICON } from "../../constants/radioIdentity"
import {
  RADIO_CLASS_BY_OWNER, RADIO_CLASS_BY_RULES, RADIO_CLASS_WORDS, RADIO_DEVICE_REMOVE, RADIO_MOVING_NOW, RADIO_NEVER_HEARD, RADIO_NOW_WORDS, RADIO_SIGNAL_WORDS,
  radioLastHeardLine,
} from "../../constants/radio"

// Radio · My devices: one of your devices as a row that opens its page. Simple says what you need at a glance: the name, home or not, when it
// was last heard, the router box it is on and whether it is moving, and the last day as one sentence. Advanced adds the signal, the class and
// the hour squares, and the Remove button (beside the link, never inside it).
const props = defineProps<{ row: MyDeviceRow; advanced: boolean; busy: boolean }>()
const emit = defineEmits<{ (e: "remove", row: MyDeviceRow): void }>()
const NOW_STATE = { here: "running", away: "neutral", not_listening: "queued" } as const
const facts = computed(() => {
  const r = props.row
  const out = [r.lastHeard ? radioLastHeardLine(formatRelativeTime(r.lastHeard)) : RADIO_NEVER_HEARD]
  if (r.where) out.unshift(r.where)
  if (r.moving) out.push(RADIO_MOVING_NOW)
  return out
})
const details = computed(() => {
  const r = props.row
  const out = [`${RADIO_CLASS_WORDS[r.cls]} (${r.classBy === "owner" ? RADIO_CLASS_BY_OWNER : RADIO_CLASS_BY_RULES})`]
  if (r.signal && r.signal !== "unknown") out.push(`${RADIO_SIGNAL_WORDS[r.signal]} signal`)
  return out
})
</script>

<template>
  <li class="flex flex-col gap-2 py-3" data-test="radio-my-device" :data-id="row.short" :data-state="row.state">
    <div class="flex items-start gap-3">
      <RouterLink :to="toRadioDevice(row.short)" class="flex min-h-11 min-w-0 flex-1 items-start gap-3 rounded-[var(--radius-md)] hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2" data-test="radio-my-device-open">
        <span class="mt-0.5 flex h-8 w-8 flex-none items-center justify-center rounded-[var(--radius-md)] border" :class="row.state === 'here' ? 'border-accent-brand text-accent-brand' : 'border-hairline text-muted'" aria-hidden="true">
          <Icon :name="RADIO_KIND_ICON[row.kind]" size="sm" />
        </span>
        <span class="flex min-w-0 flex-1 flex-col gap-1">
          <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span class="min-w-0 break-words font-medium text-primary" data-test="radio-my-device-name">{{ row.label }}</span>
            <StatusTag :state="NOW_STATE[row.state]" :label="RADIO_NOW_WORDS[row.state]" />
          </span>
          <span class="text-[length:var(--text-small)] text-secondary" data-test="radio-my-device-facts">{{ facts.join(" · ") }}</span>
          <span class="text-[length:var(--text-small)] text-muted" data-test="radio-my-device-hours">{{ hoursLine(row) }}</span>
          <span v-if="advanced" class="text-[length:var(--text-small)] text-muted" data-test="radio-my-device-details">{{ details.join(" · ") }}</span>
        </span>
        <Icon name="chevron-right" size="sm" class-name="mt-2 flex-none text-faint" />
      </RouterLink>
      <BaseButton v-if="advanced" variant="secondary" class-name="flex-none max-sm:!h-11" :disabled="busy" data-test="radio-my-device-remove" @click="emit('remove', row)">{{ RADIO_DEVICE_REMOVE }}</BaseButton>
    </div>
    <RadioPresenceStrip v-if="advanced && row.presence" :device="row.presence" class="pl-11" />
  </li>
</template>
