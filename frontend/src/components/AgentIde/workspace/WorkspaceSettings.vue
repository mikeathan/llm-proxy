<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { DispatcherService } from "../../../services/automation/dispatcherService";
import { useToast } from "../../../composables/useToast";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { useUnsavedChangesGuard } from "../../../composables/ui/useUnsavedChangesGuard";
import { GUARDRAIL_RULES, fieldSource, normalizeLayer, seedLayer } from "../../../domain/guardrailLayers";
import { errorMessage } from "../../../utils/errors";
import type { AgentGuardrailsConfig, GuardrailSection, GuardrailSource } from "../../../types/admin";
import GuardrailForm from "../system/GuardrailForm.vue";
import Panel from "../../common/layout/Panel.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import LoadingState from "../../common/feedback/LoadingState.vue";
import ErrorState from "../../common/feedback/ErrorState.vue";
import SettingsActions from "../../settings/SettingsActions.vue";

// Workspace · Settings: the workspace's own guardrail layer over the global
// policy. Without a layer the workspace simply uses the global policy; with one
// the form shows each effective value and whether it is inherited, overridden
// or an exception (the backend's merge, domain/guardrailLayers). Save writes
// the layer; "Reset to global policy" removes it — a separate, confirmed step.
// The draft survives visits to other destinations (the page is kept alive), so
// only leaving this section or workspace asks about unsaved edits.
const props = defineProps<{
  workspaceId: string;
  globalGuardrails: AgentGuardrailsConfig;
}>();
const emit = defineEmits<{ (e: "dirty-change", dirty: boolean): void }>();

const SETTINGS_SECTION = "settings";
const WORKSPACES_DESTINATION = "workspaces";

const toast = useToast();
const { confirm } = useConfirm();

// The whole workspace config (saved back whole: the endpoint replaces it).
const stored = ref<Record<string, unknown> | null>(null);
const savedLayer = ref<AgentGuardrailsConfig | null>(null);
const draft = ref<AgentGuardrailsConfig | null>(null);
const loading = ref(true);
const loadError = ref("");
const saving = ref(false);
const saveError = ref("");
const resetting = ref(false);

const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(savedLayer.value));
watch(dirty, (value) => emit("dirty-change", value), { immediate: true });

useUnsavedChangesGuard(dirty, {
  discards: (to) =>
    to.meta.destination === WORKSPACES_DESTINATION && !(to.params.ws === props.workspaceId && to.params.section === SETTINGS_SECTION),
});

async function load() {
  loading.value = true;
  loadError.value = "";
  saveError.value = "";
  try {
    const config = (await DispatcherService.getWorkspaceConfig(props.workspaceId)) as Record<string, unknown>;
    const layer = config.guardrails as AgentGuardrailsConfig | undefined;
    stored.value = config;
    savedLayer.value = layer ? normalizeLayer(layer, props.globalGuardrails) : null;
    draft.value = copy(savedLayer.value);
  } catch (e) {
    loadError.value = errorMessage(e);
  } finally {
    loading.value = false;
  }
}

const customise = () => {
  draft.value = seedLayer(props.globalGuardrails);
};
const discard = () => {
  saveError.value = "";
  draft.value = copy(savedLayer.value);
};

async function save() {
  if (!stored.value || !draft.value) return;
  saving.value = true;
  saveError.value = "";
  const next = { ...stored.value, guardrails: copy(draft.value) };
  try {
    await DispatcherService.updateWorkspaceConfig(props.workspaceId, next);
    stored.value = next;
    savedLayer.value = copy(draft.value);
    toast.success(`Saved the policy of ${props.workspaceId}`);
  } catch (e) {
    saveError.value = `Could not save the workspace policy: ${errorMessage(e)}. Your changes are still here — try again.`;
  } finally {
    saving.value = false;
  }
}

async function resetToGlobal() {
  if (!stored.value) return;
  const ok = await confirm({
    title: `Reset ${props.workspaceId} to the global policy?`,
    message: "Its own guardrail settings are removed now, including any unsaved edits. The global policy applies from the next tool call.",
    type: "warning",
    confirmText: "Reset",
  });
  if (!ok) return;
  resetting.value = true;
  saveError.value = "";
  const { guardrails: _removed, ...rest } = stored.value;
  try {
    await DispatcherService.updateWorkspaceConfig(props.workspaceId, rest);
    stored.value = rest;
    savedLayer.value = null;
    draft.value = null;
    toast.success(`${props.workspaceId} now uses the global policy`);
  } catch (e) {
    saveError.value = `Could not reset the workspace policy: ${errorMessage(e)}`;
  } finally {
    resetting.value = false;
  }
}

// ── Readouts ────────────────────────────────────────────────────────────────
const SECTIONS = Object.keys(GUARDRAIL_RULES) as GuardrailSection[];
const counts = computed(() => {
  const tally: Record<GuardrailSource, number> = { inherited: 0, overridden: 0, exception: 0 };
  for (const section of SECTIONS) {
    for (const field of Object.keys(GUARDRAIL_RULES[section])) {
      tally[fieldSource(section, field, props.globalGuardrails, draft.value)]++;
    }
  }
  return tally;
});
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

onMounted(load);
watch(() => props.workspaceId, load);
</script>

<template>
  <div class="flex flex-col gap-4">
    <LoadingState v-if="loading" label="Loading the workspace policy" />
    <ErrorState
      v-else-if="loadError"
      title="Could not load the workspace policy"
      :cause="loadError"
      next="Check that the workspace still exists and its config file is readable, then retry."
    >
      <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
    </ErrorState>

    <template v-else>
      <Panel title="Guardrail policy">
        <template v-if="savedLayer" #actions>
          <BaseButton variant="ghost" size="sm" icon="refresh" :loading="resetting" @click="resetToGlobal">Reset to global policy</BaseButton>
        </template>

        <div v-if="!draft" class="flex flex-col items-start gap-3">
          <p class="m-0 max-w-[72ch] text-secondary">
            <span class="font-mono text-primary">{{ workspaceId }}</span> uses the global policy (Settings · Agent Guardrails) as it is.
          </p>
          <p class="m-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
            A workspace policy adds to the global one: extra list entries, switches the global policy leaves off, and its own
            limits and network access. It cannot remove global entries or switch off what the global policy turns on.
          </p>
          <BaseButton variant="secondary" icon="plus" @click="customise">Customise for this workspace</BaseButton>
        </div>

        <div v-else class="flex flex-col gap-4">
          <p data-test="policy-summary" class="m-0 text-[length:var(--text-small)] text-muted">
            Against the global policy: {{ plural(counts.overridden, "override") }} · {{ plural(counts.exception, "exception") }} ·
            everything else inherited. Network access is always set by a workspace policy.
          </p>
          <GuardrailForm v-model="draft" :inherited="globalGuardrails" />
        </div>
      </Panel>

      <SettingsActions
        v-if="draft || dirty"
        :dirty="dirty"
        :saving="saving"
        :error="saveError"
        save-label="Save workspace policy"
        @save="save"
        @discard="discard"
      />
      <p v-else-if="saveError" role="alert" class="m-0 text-right text-[length:var(--text-small)] text-state-error">{{ saveError }}</p>
    </template>
  </div>
</template>
