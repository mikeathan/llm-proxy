<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRouter } from "vue-router"
import Callout from "../common/feedback/Callout.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import CopyButton from "../common/display/CopyButton.vue"
import { useNetauditReadiness } from "../../composables/netaudit/useNetauditReadiness"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { REQUIREMENT_COPY } from "../../constants/netaudit"
import { toSettings } from "../../router/routes"
import type { SettingsTab } from "../../types/admin"
import type { Requirement } from "../../types/netaudit"

// What is missing before a scan can run, each with the one step that meets it. Blocking ones are warnings; the ones
// that only limit scans are notes. `here` is the settings section this sits on, so it does not offer a link to itself.
const props = defineProps<{ here?: SettingsTab }>()

const router = useRouter()
const { readiness, error, refresh } = useNetauditReadiness()
const { advanced } = useNetauditViewMode()
const help = useNetauditHelp()
const scope = useScanScope()

const requirements = computed(() => readiness.value?.requirements ?? [])

const copyOf = (req: Requirement) => REQUIREMENT_COPY[req.code]
const body = (req: Requirement) => (advanced.value ? req.message : copyOf(req).simple)
const settingsFix = (req: Requirement) => (req.fix?.kind === 'open_settings' && req.fix.section !== props.here ? req.fix.section : null)

async function confirmFix(action: string) {
  if (await scope.confirmFix(action)) await refresh()
}

onMounted(refresh)
</script>

<template>
  <div v-if="requirements.length || error" class="flex flex-col gap-2" data-test="readiness">
    <Callout v-if="error" tone="error" title="Could not check whether scans can run">{{ error }}</Callout>
    <Callout v-for="req in requirements" :key="req.code" :tone="req.blocking ? 'warning' : 'info'" :title="copyOf(req).title">
      <p class="m-0">{{ body(req) }}</p>
      <div v-if="req.fix?.kind === 'copy'" class="mt-2 flex items-center gap-2">
        <code class="min-w-0 break-all font-mono text-[length:var(--text-small)] text-primary">{{ req.fix.text }}</code>
        <CopyButton :text="req.fix.text" title="Copy the install command" />
      </div>
      <template #actions>
        <BaseButton v-if="settingsFix(req)" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="router.push(toSettings(settingsFix(req)!))">Open settings</BaseButton>
        <BaseButton v-if="req.fix?.kind === 'confirm'" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="confirmFix(req.fix.action)">Allow anyway</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="help.show(copyOf(req).help)">Learn more</BaseButton>
      </template>
    </Callout>
  </div>
</template>
