<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { SIMPLE_USE_NETWORK } from "../../constants/netaudit"
import type { ScanRange } from "../../types/netaudit"

// Network · Settings · What may be scanned: which networks scans may look at. Every change saves at once, so there is
// no Save button. Simple is one decision ("Use my network"); Advanced lists the ranges and the networks this machine is on.
// The scan limits (time per device, confinement) are ScanLimitsSettings, which reads the same shared scope.
const scope = useScanScope()
const { advanced } = useNetauditViewMode()
const typed = ref("")

const suggestions = scope.unusedSuggestions
const canUseNetwork = computed(() => suggestions.value.length > 0)
const devicesLabel = (r: ScanRange) => `${r.devices} ${r.devices === 1 ? "device" : "devices"}`

async function addTyped() {
  if (!typed.value.trim()) return
  if (await scope.addRange(typed.value)) typed.value = ""
}

onMounted(() => void scope.load())
</script>

<template>
  <div class="flex flex-col gap-4">
    <ErrorState v-if="scope.loadError.value" title="The allowed networks could not be read" :cause="scope.loadError.value" next="Check that the server is running, then retry.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="scope.load">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="scope.loading.value && !scope.scope.value" label="Reading the allowed networks" />

    <template v-else-if="scope.scope.value">
      <Panel title="What may be scanned">
        <div class="flex flex-col gap-3">
          <p v-if="!scope.ranges.value.length" class="m-0 text-secondary">Scanning is off. Choose your network to turn it on; only devices inside it are ever looked at.</p>
          <ul v-else class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Allowed networks">
            <li v-for="r in scope.ranges.value" :key="r.cidr" class="flex flex-wrap items-center justify-between gap-2">
              <span class="min-w-0 break-words text-primary">
                <span :class="advanced ? 'font-mono' : ''">{{ advanced ? r.cidr : "Your network" }}</span>
                <span class="text-muted"> · {{ devicesLabel(r) }}</span>
              </span>
              <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="scope.removeRange(r.cidr)">{{ advanced ? "Remove" : "Stop allowing" }}</BaseButton>
            </li>
          </ul>

          <BaseButton v-if="!advanced && canUseNetwork" class-name="self-start max-sm:!h-11" :loading="scope.saving.value" @click="scope.useMyNetwork">{{ SIMPLE_USE_NETWORK }}</BaseButton>
          <p v-else-if="!advanced && !scope.ranges.value.length" class="m-0 text-[length:var(--text-small)] text-muted">
            No private network was found on this computer. Switch to Advanced to type one.
          </p>

          <template v-if="advanced">
            <section v-if="suggestions.length" aria-label="Networks this computer is on" class="flex flex-col gap-1">
              <p class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">This computer is on</p>
              <div v-for="s in suggestions" :key="s.cidr" class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-mono text-[length:var(--text-small)] text-secondary">{{ s.cidr }} · {{ s.interface }} · {{ s.devices }} devices</span>
                <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="scope.saving.value" @click="scope.addRange(s.cidr)">Allow</BaseButton>
              </div>
            </section>
            <p v-if="scope.scope.value.suggestions_error" class="m-0 text-[length:var(--text-small)] text-muted">The networks this computer is on could not be listed: {{ scope.scope.value.suggestions_error }}</p>
            <form class="flex flex-wrap items-end gap-2" @submit.prevent="addTyped">
              <FormField label="Add an address or range" hint="For example 192.168.1.0/24, 192.168.1.10-192.168.1.40 or a device name on your network.">
                <template #default="{ id, describedBy }">
                  <input :id="id" v-model="typed" type="text" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" class="h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" />
                </template>
              </FormField>
              <BaseButton type="submit" variant="secondary" class-name="max-sm:!h-11" :loading="scope.saving.value" :disabled="!typed.trim()">Add</BaseButton>
            </form>
          </template>

          <p v-if="scope.saveError.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ scope.saveError.value }}</p>
        </div>
      </Panel>
    </template>
  </div>
</template>
