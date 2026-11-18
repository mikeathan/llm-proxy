<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import SelectInput from "../components/common/forms/SelectInput.vue"
import Callout from "../components/common/feedback/Callout.vue"
import Icon from "../components/icons/Icon.vue"
import RadioRadarCanvas from "../components/radio/RadioRadarCanvas.vue"
import RadioTrack from "../components/radio/RadioTrack.vue"
import RadioMemberRow from "../components/radio/RadioMemberRow.vue"
import { useRadioDevice } from "../composables/radio/useRadioDevice"
import { usePolling } from "../composables/ui/usePolling"
import { toRadio } from "../router/routes"
import {
  RADIO_CLASS_HELP, RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_MARK_BUTTON, RADIO_MARK_LABEL, RADIO_MARK_PLACEHOLDER, RADIO_MARK_SAVE, RADIO_REASONS,
  RADIO_RING_WORDS, RADIO_TREND_WORDS, radioChannelBand, radioDbmLine, radioRangeLine, radioUsualLine, radioWifiWord,
} from "../constants/radio"
import {
  RADIO_DEVICE_ADDRESS, RADIO_DEVICE_ADDRESS_FIXED, RADIO_DEVICE_ADDRESS_PRIVATE, RADIO_DEVICE_ANSWER, RADIO_DEVICE_BACK, RADIO_DEVICE_CALIBRATE,
  RADIO_DEVICE_CALIBRATE_AT, RADIO_DEVICE_CALIBRATE_HELP, RADIO_DEVICE_CHANCES, RADIO_DEVICE_CLIENTS, RADIO_DEVICE_FACTORS, RADIO_DEVICE_FAMILY,
  RADIO_DEVICE_HAND_BACK, RADIO_DEVICE_HISTORY, RADIO_DEVICE_KIND_LABEL, RADIO_DEVICE_LOADING, RADIO_DEVICE_MAKER, RADIO_DEVICE_MODEL, RADIO_DEVICE_NETWORK,
  RADIO_DEVICE_NOT_MINE, RADIO_DEVICE_RADIOS, RADIO_DEVICE_SAVED, RADIO_DEVICE_SHORT_ID, RADIO_DEVICE_USES, RADIO_DEVICE_WHAT, RADIO_DEVICE_WHERE,
  RADIO_DEVICE_WHOSE, RADIO_DEVICE_WIFI, RADIO_EVIDENCE_BOX_YOURS, RADIO_EVIDENCE_LAN, RADIO_EVIDENCE_NET_OTHER, RADIO_EVIDENCE_NET_YOURS,
  RADIO_EVIDENCE_NONE, RADIO_EVIDENCE_ROUTER, RADIO_EVIDENCE_YOUR_AP, RADIO_FACTOR_WORDS, RADIO_HINT_SUFFIX, RADIO_KIND_ICON, RADIO_KIND_ORDER,
  RADIO_KIND_SOURCES, RADIO_KIND_WORDS, RADIO_MOTION_WORDS, RADIO_OWNERSHIP_WORDS, RADIO_RADAR_SOLO_HELP, radioOnWiFiCycles, radioSureWord,
  RADIO_DEVICE_GONE_TITLE, RADIO_DEVICE_FAILED_TITLE, radioDeviceChange, RADIO_DEVICE_EVIDENCE, radioEvidenceLine,
} from "../constants/radioIdentity"
import { conclusionOf, defaultLabel, evidenceOf, nameOf, ownershipOf } from "../utils/radio/identity"
import { edgeToFit } from "../utils/radio/radar"
import type { RadioDeviceKind } from "../types/radio"

// Radio · one device's own page: what it is (kind, maker, model, how sure and from what), whose it probably is and why (the evidence, the
// sorting's chances and the numbers that decided them), where it is (alone on the radar: a shaded ring from its nearest to its farthest distance,
// because one sensor cannot say which way), the network and router it belongs to (each a link), how its sorting changed, and the owner's answers.
const route = useRoute()
const id = computed(() => String(route.params.id ?? ""))
const dev = useRadioDevice(id)
const REFRESH_MS = 30000
const v = computed(() => dev.view.value)
const d = computed(() => v.value?.device ?? null)
const ident = computed(() => d.value?.ident)
const own = computed(() => (d.value ? ownershipOf(d.value) : "unknown"))
const edge = computed(() => (d.value ? edgeToFit([d.value.far_m * 1.2]) : 80))
const evidence = computed(() => (d.value ? evidenceOf(d.value, {
  yourAP: RADIO_EVIDENCE_YOUR_AP, boxYours: RADIO_EVIDENCE_BOX_YOURS, router: RADIO_EVIDENCE_ROUTER, lan: RADIO_EVIDENCE_LAN, netYours: RADIO_EVIDENCE_NET_YOURS,
  netOther: RADIO_EVIDENCE_NET_OTHER, none: RADIO_EVIDENCE_NONE, onWiFi: radioOnWiFiCycles,
}) : []))
const chances = computed(() => RADIO_CLASS_ORDER.filter((c) => c !== "unknown").map((c) => ({ c, pct: v.value?.chances[c] ?? 0 })))
const canCalibrate = computed(() => !!d.value && !d.value.usual && (d.value.mine || !!ident.value?.your_ap))
const kindOptions = RADIO_KIND_ORDER.map((k) => ({ value: k, label: RADIO_KIND_WORDS[k] }))
const naming = ref(false)
const label = ref("")
const metres = ref(1)

async function saveMine(): Promise<void> {
  const name = label.value.trim() || (d.value ? defaultLabel(d.value) : "")
  if (name && (await dev.markMine(name))) naming.value = false
}

watch(id, () => { dev.view.value = null; void dev.load() })
onMounted(() => { void dev.load() })
usePolling(() => { void dev.load() }, REFRESH_MS)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="radio-device-page">
    <RouterLink :to="toRadio('around')" class="inline-flex min-h-11 items-center gap-1 self-start text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-device-back"><Icon name="chevron-left" size="sm" />{{ RADIO_DEVICE_BACK }}</RouterLink>
    <Callout v-if="dev.error.value && !d" :tone="dev.gone.value ? 'info' : 'error'" :title="dev.gone.value ? RADIO_DEVICE_GONE_TITLE : RADIO_DEVICE_FAILED_TITLE"><span data-test="radio-device-error">{{ dev.error.value }}</span></Callout>
    <p v-else-if="!d" class="m-0 text-muted">{{ RADIO_DEVICE_LOADING }}</p>
    <template v-else>
      <header class="flex items-start gap-3" data-test="radio-device-header">
        <span class="flex h-12 w-12 flex-none items-center justify-center rounded-[var(--radius-md)] border" :class="own === 'mine' || own === 'likely' ? 'border-accent-brand text-accent-brand' : 'border-hairline text-muted'" aria-hidden="true"><Icon :name="RADIO_KIND_ICON[ident?.kind ?? 'unknown']" size="lg" /></span>
        <div class="flex min-w-0 flex-col gap-0.5">
          <h1 class="m-0 break-words text-[length:var(--text-heading)] text-primary" data-test="radio-device-title">{{ nameOf(d) }}</h1>
          <p class="m-0 text-secondary" data-test="radio-device-conclusion">{{ conclusionOf(d) }}</p>
          <p class="m-0 text-[length:var(--text-small)] text-muted"><span :class="own === 'mine' || own === 'likely' ? 'text-accent-brand' : ''">{{ RADIO_OWNERSHIP_WORDS[own] }}</span> · {{ RADIO_DEVICE_SHORT_ID }} <span class="font-mono">{{ d.id }}</span></p>
        </div>
      </header>

      <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]">
        <div class="flex flex-col gap-4">
          <Panel :title="RADIO_DEVICE_WHERE">
            <div class="flex flex-col gap-2" data-test="radio-device-where">
              <RadioRadarCanvas :devices="[d]" :selected="d.id" :edge="edge" solo />
              <p class="m-0 text-primary" data-test="radio-device-range">{{ radioRangeLine(d.near_m, d.far_m) }} · {{ RADIO_RING_WORDS[d.ring] }}</p>
              <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ radioDbmLine(d.dbm) }}<template v-if="d.channel"> · {{ radioChannelBand(d.channel) }}</template><template v-if="!d.usual"> · {{ RADIO_TREND_WORDS[d.trend] }}</template><template v-if="ident?.motion"> · {{ RADIO_MOTION_WORDS[ident.motion] }}</template></p>
              <p v-if="d.usual_dbm" class="m-0 text-[length:var(--text-small)] text-muted">{{ radioUsualLine(d.usual_dbm, d.dbm) }}</p>
              <p class="m-0 text-[length:var(--text-small)] text-faint">{{ RADIO_RADAR_SOLO_HELP }}</p>
              <RadioTrack v-if="!d.usual" :id="d.id" />
            </div>
          </Panel>
        </div>

        <div class="flex min-w-0 flex-col gap-4">
          <Panel :title="RADIO_DEVICE_WHAT">
            <dl class="m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-[length:var(--text-small)]" data-test="radio-device-what">
              <dt class="text-muted">{{ RADIO_DEVICE_KIND_LABEL }}</dt>
              <dd class="m-0 text-primary" data-test="radio-device-kind">{{ RADIO_KIND_WORDS[ident?.kind ?? 'unknown'] }}<span v-if="ident && ident.kind_source !== 'owner_said'" class="text-muted"> — {{ radioSureWord(ident.kind_confidence).toLowerCase() }}, {{ RADIO_KIND_SOURCES[ident.kind_source] }}</span></dd>
              <template v-if="ident?.brand"><dt class="text-muted">{{ RADIO_DEVICE_MAKER }}</dt><dd class="m-0 text-primary" data-test="radio-device-maker">{{ ident.brand }} <span v-if="ident.brand_hint" class="text-muted">{{ RADIO_HINT_SUFFIX }}</span></dd></template>
              <template v-if="ident?.model"><dt class="text-muted">{{ RADIO_DEVICE_MODEL }}</dt><dd class="m-0 text-primary" data-test="radio-device-model">{{ ident.model }}</dd></template>
              <template v-if="d.detail?.wifi_generation"><dt class="text-muted">{{ RADIO_DEVICE_WIFI }}</dt><dd class="m-0 text-primary">{{ radioWifiWord(d.detail.wifi_generation) }}</dd></template>
              <template v-if="d.network"><dt class="text-muted">{{ RADIO_DEVICE_NETWORK }}</dt><dd class="m-0 text-primary">{{ d.network }}</dd></template>
              <template v-if="d.detail"><dt class="text-muted">{{ RADIO_DEVICE_ADDRESS }}</dt><dd class="m-0 text-primary">{{ d.detail.random_address ? RADIO_DEVICE_ADDRESS_PRIVATE : RADIO_DEVICE_ADDRESS_FIXED }}</dd></template>
            </dl>
            <p v-if="v?.evidence" class="m-0 mt-2 text-[length:var(--text-small)] text-secondary" data-test="radio-device-evidence-line"><span class="text-muted">{{ RADIO_DEVICE_EVIDENCE }}:</span> {{ radioEvidenceLine(v.evidence) }}</p>
            <p v-if="ident?.family" class="m-0 mt-2 text-[length:var(--text-small)] text-muted" data-test="radio-device-family">{{ RADIO_DEVICE_FAMILY(ident.family) }}</p>
          </Panel>

          <Panel :title="RADIO_DEVICE_WHOSE">
            <div class="flex flex-col gap-3" data-test="radio-device-whose">
              <ul class="m-0 flex list-disc flex-col gap-0.5 pl-5 text-[length:var(--text-small)] text-secondary" data-test="radio-device-evidence"><li v-for="e in evidence" :key="e">{{ e }}</li></ul>
              <div v-if="!d.mine" class="flex flex-col gap-1">
                <p class="m-0 text-[length:var(--text-small)] text-secondary"><span class="text-primary">{{ RADIO_CLASS_WORDS[d.class] }}</span> — {{ RADIO_CLASS_HELP[d.class] }}<template v-if="d.detail"> ({{ RADIO_REASONS[d.detail.why] ?? d.detail.why }})</template></p>
                <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICE_CHANCES }}</p>
                <ul class="m-0 flex list-none flex-col gap-1 p-0" data-test="radio-device-chances">
                  <li v-for="x in chances" :key="x.c" class="grid grid-cols-[8rem_1fr_3rem] items-center gap-2 text-[length:var(--text-small)]">
                    <span class="text-secondary">{{ RADIO_CLASS_WORDS[x.c] }}</span>
                    <span class="h-1.5 overflow-hidden rounded-full bg-hairline" aria-hidden="true"><span class="block h-full bg-accent-brand" :style="{ width: x.pct + '%' }" /></span>
                    <span class="text-right font-mono text-muted">{{ x.pct }}%</span>
                  </li>
                </ul>
                <p v-if="v?.factors.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-device-factors">{{ RADIO_DEVICE_FACTORS }}: {{ v.factors.map((f) => RADIO_FACTOR_WORDS[f.feature] ?? f.feature).join(", ") }}.</p>
              </div>
            </div>
          </Panel>

          <Panel v-if="v && (v.uses || v.radios.length || v.clients.length)" :title="RADIO_DEVICE_NETWORK">
            <div class="flex flex-col gap-2" data-test="radio-device-network">
              <template v-if="v.uses"><p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICE_USES }}</p><RadioMemberRow :member="v.uses" /></template>
              <template v-if="v.radios.length"><p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICE_RADIOS }}</p><ul class="m-0 list-none divide-y divide-hairline p-0"><li v-for="m in v.radios" :key="m.id"><RadioMemberRow :member="m" /></li></ul></template>
              <template v-if="v.clients.length"><p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICE_CLIENTS }} ({{ v.clients.length }})</p><ul class="m-0 list-none divide-y divide-hairline p-0"><li v-for="m in v.clients" :key="m.id"><RadioMemberRow :member="m" /></li></ul></template>
            </div>
          </Panel>

          <Panel :title="RADIO_DEVICE_ANSWER">
            <div class="flex flex-col gap-3" data-test="radio-device-answer">
              <div v-if="!d.mine" class="flex flex-wrap gap-2">
                <BaseButton v-if="!naming" variant="primary" class-name="max-sm:!h-11" :disabled="dev.busy.value" data-test="radio-device-mine" @click="naming = true; label = defaultLabel(d)">{{ RADIO_MARK_BUTTON }}</BaseButton>
                <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="dev.busy.value" data-test="radio-device-not-mine" @click="dev.setClass('not_mine')">{{ RADIO_DEVICE_NOT_MINE }}</BaseButton>
                <BaseButton variant="ghost" class-name="max-sm:!h-11" :disabled="dev.busy.value" data-test="radio-device-hand-back" @click="dev.setClass('')">{{ RADIO_DEVICE_HAND_BACK }}</BaseButton>
              </div>
              <form v-if="naming" class="flex flex-wrap items-end gap-2" @submit.prevent="saveMine">
                <label class="flex flex-col gap-1 text-[length:var(--text-small)] text-secondary">{{ RADIO_MARK_LABEL }}
                  <input v-model="label" type="text" maxlength="60" :placeholder="RADIO_MARK_PLACEHOLDER" class="h-8 w-64 max-w-full rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary focus-visible:outline-none focus-visible:ring-2 max-sm:h-11" data-test="radio-device-mine-input" />
                </label>
                <BaseButton type="submit" variant="primary" class-name="max-sm:!h-11" :disabled="dev.busy.value" data-test="radio-device-mine-save">{{ RADIO_MARK_SAVE }}</BaseButton>
              </form>
              <label class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary">{{ RADIO_DEVICE_KIND_LABEL }}
                <SelectInput :model-value="ident?.kind ?? 'unknown'" :options="kindOptions" :label="RADIO_DEVICE_KIND_LABEL" class="max-sm:!h-11" data-test="radio-device-kind-select" @update:model-value="dev.setKind($event as RadioDeviceKind)" />
              </label>
              <form v-if="canCalibrate" class="flex flex-wrap items-end gap-2" data-test="radio-device-calibrate" @submit.prevent="dev.calibrate(metres)">
                <label class="flex flex-col gap-1 text-[length:var(--text-small)] text-secondary">{{ RADIO_DEVICE_CALIBRATE_AT }}
                  <input v-model.number="metres" type="number" min="0.5" max="30" step="0.5" class="h-8 w-24 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary focus-visible:outline-none focus-visible:ring-2 max-sm:h-11" />
                </label>
                <BaseButton type="submit" variant="secondary" class-name="max-sm:!h-11" :disabled="dev.busy.value">{{ RADIO_DEVICE_CALIBRATE }}</BaseButton>
                <p class="m-0 w-full text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICE_CALIBRATE_HELP }}</p>
              </form>
              <p v-if="dev.saved.value" class="m-0 text-[length:var(--text-small)] text-state-success" data-test="radio-device-saved">{{ RADIO_DEVICE_SAVED }}</p>
              <p v-if="dev.error.value && d" class="m-0 text-[length:var(--text-small)] text-state-error">{{ dev.error.value }}</p>
            </div>
          </Panel>

          <Panel v-if="v?.history.length" :title="RADIO_DEVICE_HISTORY">
            <ul class="m-0 flex list-none flex-col gap-1 p-0 text-[length:var(--text-small)] text-secondary" data-test="radio-device-history">
              <li v-for="(h, i) in v.history" :key="i"><span class="font-mono text-muted">{{ h.day }}</span> {{ radioDeviceChange(h.from, h.to, h.reason, RADIO_CLASS_WORDS, RADIO_REASONS) }}</li>
            </ul>
          </Panel>
        </div>
      </div>
    </template>
  </div>
</template>
