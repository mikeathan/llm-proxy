<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ModuleSwitchNote from "../stacks/ModuleSwitchNote.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useDnsCheck } from "../../composables/netaudit/useDnsCheck"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  DNS_CHECK_COPIED, DNS_CHECK_COPY, DNS_CHECK_COPY_FAILED, DNS_CHECK_COPY_HINT, DNS_CHECK_DETAILS, DNS_CHECK_HINT, DNS_CHECK_INTERNET_OFF, DNS_CHECK_LAN_OFF, DNS_CHECK_SERVER_HINT, DNS_CHECK_SERVER_LABEL,
  DNS_CHECK_INTERVAL_LABEL, DNS_CHECK_NAMES_HINT, DNS_CHECK_NAMES_LABEL, DNS_CHECK_OPEN_SETTINGS, DNS_CHECK_READ_FAILED_NEXT,
  DNS_CHECK_DIFFERENCES_HINT, DNS_CHECK_FINDING_HINT, DNS_CHECK_FINDING_LABEL, DNS_CHECK_DIFFERENCES_TITLE, DNS_CHECK_MORE_DIFFERENCES, DNS_CHECK_OUTSIDE_SAID, DNS_CHECK_PARTLY, DNS_CHECK_ROUTER_SAID,
  DNS_CHECK_READ_FAILED_TITLE, DNS_CHECK_READING, DNS_CHECK_RETRY, DNS_CHECK_SAVE, DNS_CHECK_TITLE, DNS_CHECK_URL_HINT, DNS_CHECK_URL_LABEL,
} from "../../constants/netaudit"
import { dnsCheckFreshness, dnsCheckLine, dnsCheckReport, dnsDifferenceRows } from "../../utils/netaudit/dnsCheck"
import { formatRelativeTime } from "../../utils/format/time"
import { copyText } from "../../utils/clipboard"
import { toSettings } from "../../router/routes"

// Network → Settings → Router DNS check: comparing the router's DNS with an outside resolver, and what has been
// recorded so far. It only records. Simple is the switch and one status line; Advanced adds the names, how often, which resolver,
// and a button that copies the results to bring back after a week.
const { state, error, busy, load, save } = useDnsCheck()
const { advanced } = useNetauditViewMode()
const names = ref("")
const url = ref("")
const server = ref("")
const interval = ref("")
const copied = ref("")

const NEVER_YEAR = 1
const since = computed(() => {
  const first = state.value?.summary.first
  return first && new Date(first).getUTCFullYear() > NEVER_YEAR ? formatRelativeTime(first) : ""
})
const line = computed(() => (state.value ? dnsCheckLine(state.value, since.value) : ""))
const fresh = computed(() => (state.value?.enabled ? dnsCheckFreshness(state.value, Date.now()) : { text: "", late: false }))
const differences = computed(() => (state.value ? dnsDifferenceRows(state.value.summary, Date.now()) : { rows: [], hidden: 0 }))

watch(state, (s) => {
  if (!s) return
  names.value = s.names.join("\n")
  url.value = s.resolver_url === s.default_resolver_url ? "" : s.resolver_url
  server.value = s.server
  interval.value = String(s.interval_minutes)
}, { immediate: true })

const submit = () => save({
  names: names.value.split("\n").map((n) => n.trim()).filter(Boolean),
  server: server.value,
  resolver_url: url.value,
  interval_minutes: Number(interval.value),
})
const toggleFinding = (e: Event) => save({ finding_enabled: (e.target as HTMLInputElement).checked })
const copy = async () => {
  copied.value = (await copyText(JSON.stringify(state.value ? dnsCheckReport(state.value) : {}, null, 2))) ? DNS_CHECK_COPIED : DNS_CHECK_COPY_FAILED
}
onMounted(load)
</script>

<template>
  <Panel :title="DNS_CHECK_TITLE">
    <div class="flex flex-col gap-3" data-test="dns-check">
      <ErrorState v-if="error && !state" :title="DNS_CHECK_READ_FAILED_TITLE" :cause="error" :next="DNS_CHECK_READ_FAILED_NEXT">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">{{ DNS_CHECK_RETRY }}</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" :label="DNS_CHECK_READING" />

      <template v-else>
        <ModuleSwitchNote :enabled="state.enabled" :hint="DNS_CHECK_HINT" :details="DNS_CHECK_DETAILS" />

        <p v-if="state.enabled" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="dns-check-status">{{ line }}</p>

        <p v-if="fresh.text" :role="fresh.late ? 'alert' : undefined" class="m-0 text-[length:var(--text-small)]" :class="fresh.late ? 'text-state-error' : 'text-secondary'" data-test="dns-check-fresh">{{ fresh.text }}</p>

        <section v-if="differences.rows.length" class="flex flex-col gap-2" data-test="dns-check-differences">
          <h3 class="m-0 text-[length:var(--text-small)] font-semibold text-primary">{{ DNS_CHECK_DIFFERENCES_TITLE }}</h3>
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ DNS_CHECK_DIFFERENCES_HINT }}</p>
          <ul class="m-0 flex list-none flex-col gap-2 p-0">
            <li v-for="row in differences.rows" :key="row.key" class="flex min-w-0 flex-col gap-0.5 rounded-[var(--radius-md)] border border-control px-3 py-2 text-[length:var(--text-small)]" data-test="dns-check-difference">
              <span class="break-words font-medium text-primary">{{ row.name }} <span class="font-normal text-muted">· {{ row.when }}<template v-if="row.partly"> · {{ DNS_CHECK_PARTLY }}</template></span></span>
              <span class="break-words font-mono text-secondary">{{ DNS_CHECK_ROUTER_SAID }} {{ row.router }}</span>
              <span class="break-words font-mono text-secondary">{{ DNS_CHECK_OUTSIDE_SAID }} {{ row.outside }}</span>
            </li>
          </ul>
          <p v-if="differences.hidden > 0" class="m-0 text-[length:var(--text-small)] text-muted">{{ differences.hidden }} {{ DNS_CHECK_MORE_DIFFERENCES }}</p>
        </section>

        <RouterLink v-if="state.enabled && (state.status.problem === DNS_CHECK_INTERNET_OFF || state.status.problem === DNS_CHECK_LAN_OFF)" :to="toSettings('guardrails')" data-test="dns-check-settings" class="inline-flex min-h-11 items-center self-start text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring sm:min-h-8">{{ DNS_CHECK_OPEN_SETTINGS }}</RouterLink>

        <form v-if="advanced" class="flex flex-col gap-3" @submit.prevent="submit">
          <FormField :label="DNS_CHECK_NAMES_LABEL" :hint="DNS_CHECK_NAMES_HINT">
            <template #default="{ id, describedBy }">
              <textarea :id="id" v-model="names" rows="3" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" data-test="dns-check-names" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" />
            </template>
          </FormField>
          <FormField :label="DNS_CHECK_INTERVAL_LABEL">
            <template #default="{ id }">
              <select :id="id" v-model="interval" data-test="dns-check-interval" class="h-11 min-w-0 self-start rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8">
                <option v-for="minutes in state.allowed_intervals" :key="minutes" :value="String(minutes)">Every {{ minutes }} minutes</option>
              </select>
            </template>
          </FormField>
          <FormField :label="DNS_CHECK_SERVER_LABEL" :hint="DNS_CHECK_SERVER_HINT">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="server" type="text" inputmode="decimal" autocomplete="off" spellcheck="false" placeholder="192.168.1.1" :aria-describedby="describedBy" data-test="dns-check-server" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" />
            </template>
          </FormField>
          <FormField :label="DNS_CHECK_URL_LABEL" :hint="DNS_CHECK_URL_HINT">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="url" type="text" inputmode="url" autocomplete="off" spellcheck="false" :placeholder="state.default_resolver_url" :aria-describedby="describedBy" data-test="dns-check-url" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" />
            </template>
          </FormField>
          <BaseButton variant="secondary" size="sm" type="submit" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="dns-check-save">{{ DNS_CHECK_SAVE }}</BaseButton>
        </form>

        <label v-if="advanced" class="flex min-h-11 items-start gap-2 text-[length:var(--text-small)] text-primary sm:min-h-8">
          <input type="checkbox" :checked="state.finding_enabled === true" :disabled="busy" data-test="dns-check-finding" class="mt-1" @change="toggleFinding">
          <span class="flex flex-col"><span class="font-medium">{{ DNS_CHECK_FINDING_LABEL }}</span><span class="text-muted">{{ DNS_CHECK_FINDING_HINT }}</span></span>
        </label>

        <div v-if="advanced" class="flex flex-wrap items-center gap-2">
          <BaseButton variant="ghost" size="sm" icon="copy" class-name="max-sm:!h-11" data-test="dns-check-copy" @click="copy">{{ DNS_CHECK_COPY }}</BaseButton>
          <span role="status" class="text-[length:var(--text-small)] text-muted" data-test="dns-check-copy-note">{{ copied || DNS_CHECK_COPY_HINT }}</span>
        </div>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
