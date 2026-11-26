<script setup lang="ts">
import { computed, ref } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import DataTable from "../common/display/DataTable.vue";
import StatusTag from "../common/display/StatusTag.vue";
import SearchInput from "../common/forms/SearchInput.vue";
import SegmentedControl from "../common/forms/SegmentedControl.vue";
import EmptyState from "../common/feedback/EmptyState.vue";
import ModelCard from "./ModelCard.vue";
import { usePersistedState } from "../../composables/ui/usePersistedState";
import { filterModels, isLocalModel, modelStatus, orderModels, routingRole } from "../../domain/models";
import { formatTokenCount } from "../../utils/format/units";
import { MODEL_SCOPES, MODEL_VIEWS } from "../../types/model";
import type { GlobalConfig, ProviderType } from "../../types/admin";
import type { ActiveModel, Model, ModelScope, ModelView } from "../../types/model";
import type { ChoiceOption, DataTableColumn } from "../../types/ui";

// Models › Models: every model, local and cloud, as cards or a list (the choice is remembered on this device; a phone always shows
// cards). The running model comes first. A filter and All / Local / Cloud narrow it; Edit opens the side panel.

const props = defineProps<{
  models: Model[];
  config?: Pick<GlobalConfig, "primary_model" | "fallback_model">;
  active?: ActiveModel;
  providerLabel: (provider: ProviderType) => string;
}>();
const emit = defineEmits<{ (e: "edit", model: Model): void; (e: "start", name: string): void; (e: "stop"): void }>();

const VIEW_KEY = "models-view";
const VIEW_VERSION = 1;
const VIEW_LABELS: Record<ModelView, string> = { cards: "Cards", list: "List" };
const SCOPE_LABELS: Record<ModelScope, string> = { all: "All", local: "Local", cloud: "Cloud" };
const NO_VALUE = "—";

const view = usePersistedState<ModelView>(VIEW_KEY, {
  version: VIEW_VERSION,
  fallback: "cards",
  parse: (data) => (MODEL_VIEWS.includes(data as ModelView) ? (data as ModelView) : null),
});
const scope = ref<ModelScope>("all");
const filter = ref("");

const counts = computed<Record<ModelScope, number>>(() => {
  const local = props.models.filter(isLocalModel).length;
  return { all: props.models.length, local, cloud: props.models.length - local };
});
const scopeOptions = computed<ChoiceOption[]>(() => MODEL_SCOPES.map((s) => ({ value: s, label: `${SCOPE_LABELS[s]} ${counts.value[s]}` })));
const viewOptions: ChoiceOption[] = MODEL_VIEWS.map((v) => ({ value: v, label: VIEW_LABELS[v] }));
const shown = computed(() => orderModels(filterModels(props.models, scope.value, filter.value)));
// One local model runs at a time: Start waits until the running one is stopped.
const localActive = computed(() => props.models.some((m) => isLocalModel(m) && m.active));
const role = (m: Model) => routingRole(m, props.config);
const activeOf = (m: Model) => (props.active?.name === m.name ? props.active : undefined);

const modelKey = (m: Model) => `${m.provider}/${m.name}`;
const COLUMNS: DataTableColumn<Model>[] = [
  { key: "name", label: "Model" },
  { key: "where", label: "Where", value: (m) => props.providerLabel(m.provider) },
  { key: "context", label: "Context", numeric: true, value: (m) => (m.metadata?.context_length ? formatTokenCount(m.metadata.context_length) : NO_VALUE) },
  { key: "port", label: "Port", numeric: true, value: (m) => (m.port ? `:${m.port}` : NO_VALUE) },
  { key: "status", label: "State" },
  { key: "actions", label: "Actions" },
];
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex flex-wrap items-center gap-3">
      <SearchInput v-model="filter" label="Filter models" class="w-full sm:w-64" />
      <SegmentedControl v-model="scope" :options="scopeOptions" label="Which models" variant="compact" />
      <span class="flex-1"></span>
      <SegmentedControl v-model="view" :options="viewOptions" label="Show as" variant="compact" class="max-sm:hidden" />
    </div>

    <EmptyState v-if="!shown.length" title="No model matches" body="Clear the filter or choose All." />

    <template v-else>
      <div :class="['grid grid-cols-[repeat(auto-fill,minmax(min(100%,280px),1fr))] gap-3', view === 'list' ? 'sm:hidden' : '']" data-test="model-cards">
        <ModelCard
          v-for="m in shown"
          :key="modelKey(m)"
          :model="m"
          :provider-label="providerLabel(m.provider)"
          :role="role(m)"
          :active="activeOf(m)"
          :can-start="!localActive"
          @start="emit('start', m.name)"
          @stop="emit('stop')"
          @edit="emit('edit', m)"
        />
      </div>
      <div v-if="view === 'list'" class="max-sm:hidden" data-test="model-list">
        <DataTable :columns="COLUMNS" :rows="shown" :row-key="modelKey" caption="Models">
          <template #cell-name="{ row }">
            <span class="font-medium text-primary">{{ row.name }}</span>
            <span v-if="row.metadata?.quantization" class="ml-2 font-mono text-[length:var(--text-small)] text-faint">{{ row.metadata.quantization }}</span>
            <StatusTag v-if="role(row)" class="ml-2" state="neutral" :label="role(row)!" />
          </template>
          <template #cell-status="{ row }">
            <StatusTag v-bind="modelStatus(row)" />
          </template>
          <template #cell-actions="{ row }">
            <span class="inline-flex items-center gap-1">
              <template v-if="isLocalModel(row)">
                <BaseButton v-if="row.active" variant="ghost" size="sm" icon="stop" icon-only :label="`Stop ${row.name}`" @click="emit('stop')" />
                <BaseButton v-else variant="ghost" size="sm" icon="play" icon-only :label="`Start ${row.name}`" :disabled="localActive" @click="emit('start', row.name)" />
              </template>
              <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${row.name}`" @click="emit('edit', row)" />
            </span>
          </template>
        </DataTable>
      </div>
    </template>
  </div>
</template>
