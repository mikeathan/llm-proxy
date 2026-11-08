<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useHelper } from "../../composables/netaudit/useHelper"
import { copyText } from "../../utils/clipboard"
import {
  HELPER_ACTIONS_TITLE, HELPER_ACTION_LABELS, HELPER_ACTION_NOT_INSTALLED, HELPER_COPIED, HELPER_COPY, HELPER_COPY_FAILED, HELPER_HELP,
  HELPER_INSTALL_STEP, HELPER_LOAD_FAILED, HELPER_NOT_INSTALLED, HELPER_PASSPHRASE_NOTE, HELPER_RANGES_NONE_DECLARED, HELPER_RANGES_ONLY, HELPER_RANGES_TITLE, HELPER_TITLE, HELPER_UNSUPPORTED, HELPER_WHAT,
} from "../../constants/netaudit"

// Network · Settings · Privileged helper: where the helper stands and what to do about it. The service can never install the helper (a
// root command does, on purpose), so this card says so plainly and gives the command; what it does switch is each action, with the
// passphrase. The ranges are the Network settings' own. It reads the state and never starts a scan.
const { status, error, busy, load, setAction } = useHelper()
const copied = ref("")

const label = (id: string) => HELPER_ACTION_LABELS[id] ?? { title: id, hint: "" }
const rangesText = (r: string[]) => r.join(", ")
// The helper keeps no range of its own: every call is held to the ranges declared in the Network settings, so there is nothing to compare.
const rangesNote = computed(() => (status.value && status.value.declared_ranges.length === 0 ? HELPER_RANGES_NONE_DECLARED : HELPER_RANGES_ONLY))
const needsInstallCommand = computed(() => Boolean(status.value && status.value.supported && !status.value.installed))
const copy = async () => {
  copied.value = (await copyText(status.value?.install_command ?? "")) ? HELPER_COPIED : HELPER_COPY_FAILED
}
onMounted(load)
</script>

<template>
  <Panel :title="HELPER_TITLE">
    <div class="flex flex-col gap-3" data-test="helper">
      <ErrorState v-if="error && !status" :title="HELPER_LOAD_FAILED" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!status" label="Reading the privileged helper" />

      <template v-else>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ HELPER_WHAT }}</p>

        <p v-if="!status.supported" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="helper-unsupported">{{ HELPER_UNSUPPORTED }}</p>
        <p v-else-if="!status.installed" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="helper-not-installed">{{ HELPER_NOT_INSTALLED }}</p>
        <p v-else-if="!status.ready" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="helper-not-ready">{{ status.not_ready }}</p>

        <div v-if="needsInstallCommand" class="flex flex-col gap-1.5" data-test="helper-command">
          <span v-if="!status.installed" class="text-[length:var(--text-small)] text-secondary">{{ HELPER_INSTALL_STEP }}</span>
          <code class="block overflow-x-auto rounded-[var(--radius-sm)] bg-surface-raised px-2.5 py-2 font-mono text-[length:var(--text-small)] text-primary">{{ status.install_command }}</code>
          <div class="flex flex-wrap items-center gap-2">
            <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="helper-copy" @click="copy">{{ HELPER_COPY }}</BaseButton>
            <span v-if="copied" role="status" class="text-[length:var(--text-small)] text-muted">{{ copied }}</span>
          </div>
          <span class="text-[length:var(--text-small)] text-muted" data-test="helper-help">{{ HELPER_HELP }}</span>
        </div>

        <section v-if="status.installed" class="flex flex-col gap-2" :aria-label="HELPER_ACTIONS_TITLE" data-test="helper-actions">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-primary">{{ HELPER_ACTIONS_TITLE }}</h3>
          <ul class="m-0 flex list-none flex-col gap-2 p-0">
            <li v-for="a in status.actions" :key="a.id" class="flex items-start justify-between gap-3" :data-test="`helper-action-${a.id}`">
              <div class="flex min-w-0 flex-col gap-0.5">
                <span class="font-medium text-primary">{{ label(a.id).title }}</span>
                <span class="text-[length:var(--text-small)] text-muted">{{ label(a.id).hint }}</span>
                <span v-if="a.installed === false" class="text-[length:var(--text-small)] text-state-running" data-test="helper-action-missing">{{ HELPER_ACTION_NOT_INSTALLED }}</span>
              </div>
              <BaseToggle :model-value="a.on" :label="label(a.id).title" hide-label :disabled="busy || !status.ready || a.installed === false" @update:model-value="setAction(a.id, $event)" />
            </li>
          </ul>
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ HELPER_PASSPHRASE_NOTE }}</p>
        </section>

        <section v-if="status.installed" class="flex flex-col gap-1" :aria-label="HELPER_RANGES_TITLE" data-test="helper-ranges">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-primary">{{ HELPER_RANGES_TITLE }}</h3>
          <span class="font-mono text-[length:var(--text-small)] text-secondary" data-test="helper-held">{{ status.declared_ranges.length ? rangesText(status.declared_ranges) : "—" }}</span>
          <span :class="['text-[length:var(--text-small)]', status.declared_ranges.length ? 'text-muted' : 'text-state-running']" data-test="helper-ranges-note">{{ rangesNote }}</span>
        </section>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="helper-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
