<script setup lang="ts">
import { computed, onMounted } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useDeviceProbe } from "../../composables/netaudit/useDeviceProbe"
import { HELPER_ACTION_LABELS, PROBE_NO_OS, PROBE_OS_UNRELIABLE, PROBE_CLEAR, PROBE_CLEAR_HINT, PROBE_NO_UDP, PROBE_PASSPHRASE_NOTE, PROBE_RUNNING, PROBE_UDP_FILTERED } from "../../constants/netaudit"
import { formatRelativeTime } from "../../utils/format/time"
import type { OsGuess, UdpPort } from "../../types/netaudit"

// Scan one device with the privileged helper (UDP services, OS guess), from the device's own row, and show the last result kept on the
// device. The buttons appear only when the helper is installed, ready and has the action switched on, so a computer without it sees no
// new control; a kept result is shown whenever there is one. A UDP port with no reply is "open or blocked", never "open". Nothing here
// makes a finding: it is what the last scan saw, with its time. OS guesses from a device with no open port are demoted, not hidden.
const props = defineProps<{ ip?: string; mac?: string; udpPorts?: UdpPort[]; udpAt?: string; osGuesses?: OsGuess[]; osAt?: string; noOpenPorts?: boolean }>()
const emit = defineEmits<{ (e: "probed"): void }>()
const { actions, running, freshUdp, freshOs, error, load, run, clear } = useDeviceProbe(() => props.ip, () => props.mac)
onMounted(load)

// The newest result held on this page wins until the list is read again; after that the kept one is the same.
const udp = computed(() => freshUdp.value ?? (props.udpAt ? (props.udpPorts ?? []) : null))
const os = computed(() => freshOs.value ?? (props.osAt ? (props.osGuesses ?? []) : null))

async function start(id: "scan_udp" | "os_detect") {
  if (await run(id)) emit("probed")
}

async function drop(kind: "udp" | "os") {
  if (await clear(kind)) emit("probed")
}
</script>

<template>
  <div v-if="actions.length || udp || os" class="flex flex-col gap-2" data-test="probe">
    <div v-if="actions.length" class="flex flex-wrap items-center gap-2">
      <BaseButton
        v-for="id in actions"
        :key="id"
        variant="secondary"
        size="sm"
        class-name="max-sm:!h-11"
        :disabled="running !== null"
        :loading="running === id"
        :title="HELPER_ACTION_LABELS[id]?.hint"
        :data-test="`probe-${id}`"
        @click="start(id)"
      >{{ HELPER_ACTION_LABELS[id]?.title ?? id }}</BaseButton>
      <span class="text-[length:var(--text-small)] text-muted">{{ PROBE_PASSPHRASE_NOTE }}</span>
    </div>
    <p v-if="running" class="m-0 text-[length:var(--text-small)] text-muted" role="status">{{ PROBE_RUNNING }}</p>
    <p v-if="error" class="m-0 break-words text-[length:var(--text-small)] text-state-error" role="alert" data-test="probe-error">{{ error }}</p>
    <div v-if="udp" class="flex flex-col gap-0.5 text-[length:var(--text-small)] text-secondary" data-test="probe-udp">
      <span class="flex items-center gap-2 text-muted">UDP scan{{ udpAt ? `, ${formatRelativeTime(udpAt)}` : "" }}
      <BaseButton v-if="mac" variant="ghost" size="sm" class-name="max-sm:!h-11" :title="PROBE_CLEAR_HINT" data-test="probe-clear-udp" @click="drop('udp')">{{ PROBE_CLEAR }}</BaseButton>
      </span>
      <span v-if="!udp.length">{{ PROBE_NO_UDP }}</span>
      <span v-for="p in udp" :key="p.port" class="break-words">
        <span class="font-mono">{{ p.port }}/udp</span><span v-if="p.service"> {{ p.service }}</span>
        <span v-if="p.state !== 'open'" class="text-muted"> · {{ PROBE_UDP_FILTERED }}</span>
      </span>
    </div>
    <div v-if="os" class="flex flex-col gap-0.5 text-[length:var(--text-small)] text-secondary" data-test="probe-os">
      <span class="flex items-center gap-2 text-muted">OS detection{{ osAt ? `, ${formatRelativeTime(osAt)}` : "" }}
      <BaseButton v-if="mac" variant="ghost" size="sm" class-name="max-sm:!h-11" :title="PROBE_CLEAR_HINT" data-test="probe-clear-os" @click="drop('os')">{{ PROBE_CLEAR }}</BaseButton>
      </span>
      <span v-if="!os.length">{{ PROBE_NO_OS }}</span>
      <!-- With no open port a percentage is not evidence: the warning leads, the guesses are muted and carry no percentage. -->
      <span v-if="noOpenPorts && os.length" class="text-muted" data-test="probe-os-unreliable">{{ PROBE_OS_UNRELIABLE }}</span>
      <span v-for="(g, i) in os" :key="i" class="break-words" :class="{ 'text-muted': noOpenPorts }" data-test="probe-os-guess">{{ g.name }}<template v-if="!noOpenPorts"> ({{ g.accuracy }}%)</template></span>
    </div>
  </div>
</template>
