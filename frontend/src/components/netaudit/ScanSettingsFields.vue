<script setup lang="ts">
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import FormField from "../common/forms/FormField.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import Callout from "../common/feedback/Callout.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useTargetCheck } from "../../composables/netaudit/useTargetCheck"
import { SCAN_DEPTH_OPTIONS, VIEW_MODE_OPTIONS } from "../../constants/netaudit"
import ScanSummary from "./ScanSummary.vue"
import { scanFromForm } from "../../utils/netaudit/scan"
import { toSettings } from "../../router/routes"
import type { ScanDepth } from "../../types/netaudit"
import { computed } from "vue"

// What a network audit scans and how. Simple is two choices (how hard, and whether to run the safe checks); Advanced
// adds the networks and the ports. These are the audit's settings: the server applies them when it runs, so the
// agent can neither widen nor narrow them.
const targets = defineModel<string>("targets", { required: true })
const depth = defineModel<ScanDepth>("depth", { required: true })
const ports = defineModel<string>("ports", { required: true })
const checks = defineModel<boolean>("checks", { required: true })
const knownVulns = defineModel<boolean>("knownVulns", { required: true })

const { mode, advanced, set } = useNetauditViewMode()
// Say now, not at 2 a.m., which typed networks the audit would refuse.
const { problems } = useTargetCheck(() => targets.value)
const summary = computed(() => scanFromForm({ scanTargets: targets.value, scanDepth: depth.value, scanPorts: ports.value, scanChecks: checks.value, scanKnownVulns: knownVulns.value }))
const INPUT_CLASS = "h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2"
</script>

<template>
  <div class="flex flex-col gap-4" data-test="scan-settings">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="min-w-0" data-test="scan-summary"><ScanSummary :scan="summary" /></div>
      <SegmentedControl :model-value="mode" :options="VIEW_MODE_OPTIONS" label="How much detail" touch @update:model-value="set($event as 'simple' | 'advanced')" />
    </div>
    <FormField label="How thoroughly" hint="Thorough looks harder and takes a few minutes; quick checks the common ports in seconds.">
      <template #default="{ id, describedBy }">
        <select :id="id" v-model="depth" :aria-describedby="describedBy" class="form-control">
          <option v-for="o in SCAN_DEPTH_OPTIONS" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </template>
    </FormField>
    <ToggleField v-model="checks" label="Check for common weak settings" hint="SMBv1, anonymous FTP, a VNC or Redis with no password, exposed NFS shares, expired certificates and a few more. They only read; nothing tries a password and nothing changes on a device." />
    <ToggleField v-model="knownVulns" label="Look for known vulnerabilities" hint="Compares the software the scan recognises with a list of publicly known flaws kept on this computer. A match means it may be affected, not that it is. The list is downloaded in Settings → Network scan; without it the audit still runs and says this check was skipped." />
    <template v-if="advanced">
      <FormField label="Networks to scan" hint="Leave empty to scan every network you allowed in Settings → Network scan. Or list addresses, ranges or device names, separated by commas; anything outside the allowed networks is refused when the audit runs.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="targets" type="text" autocomplete="off" spellcheck="false" placeholder="every allowed network" :aria-describedby="describedBy" :class="INPUT_CLASS" />
        </template>
      </FormField>
      <Callout v-if="problems.length" tone="warning" title="The audit would not scan some of these" data-test="target-problems">
        <ul class="m-0 flex list-none flex-col gap-1 p-0">
          <li v-for="p in problems" :key="p.entry ?? p.reason" class="break-words"><span v-if="p.entry" class="font-mono">{{ p.entry }}</span><template v-if="p.entry">: </template>{{ p.reason }}</li>
        </ul>
        <template #actions>
          <RouterLink :to="toSettings('network-scan')" class="text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline">Choose the allowed networks</RouterLink>
        </template>
      </Callout>
      <RouterLink :to="toSettings('network-scan')" class="-mt-2 self-start text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline">Choose the allowed networks in Settings</RouterLink>
      <FormField label="Ports" hint="Leave empty for the common ports, or list them, for example 22,80,8000-8100.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="ports" type="text" autocomplete="off" spellcheck="false" placeholder="common ports" :aria-describedby="describedBy" :class="INPUT_CLASS" />
        </template>
      </FormField>
    </template>
  </div>
</template>
