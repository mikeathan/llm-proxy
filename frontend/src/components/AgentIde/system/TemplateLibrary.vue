<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { TemplateService } from '../../../services/template/templateService';
import type { Template, TemplateMetadata } from '../../../types/templates';
import type { ChoiceOption } from '../../../types/ui';
import BaseButton from '../../common/buttons/BaseButton.vue';
import MicroLabel from '../../common/display/MicroLabel.vue';
import SearchInput from '../../common/forms/SearchInput.vue';
import SelectInput from '../../common/forms/SelectInput.vue';
import LoadingState from '../../common/feedback/LoadingState.vue';
import EmptyState from '../../common/feedback/EmptyState.vue';
import ErrorState from '../../common/feedback/ErrorState.vue';

// The workspace Playbooks section (/workspaces/:ws/playbooks): expert task
// templates to start a new file from, or to append to the file that is open.

const props = defineProps<{
  /** The open file a playbook can be appended to, if any. */
  appendTarget: string | null;
}>();

const emit = defineEmits<{
  (e: 'inject', template: Template, mode: 'append' | 'create'): void;
}>();

const ALL_CATEGORIES = '';

const templates = ref<TemplateMetadata[]>([]);
const loading = ref(true);
const loadError = ref<string | null>(null);
const searchQuery = ref('');
const selectedCategory = ref(ALL_CATEGORIES);

const categoryOptions = computed<ChoiceOption[]>(() => [
  { value: ALL_CATEGORIES, label: 'All categories' },
  ...[...new Set(templates.value.map((t) => t.category))].sort().map((c) => ({ value: c, label: c })),
]);

const filteredTemplates = computed(() => {
  const query = searchQuery.value.trim().toLowerCase();
  return templates.value.filter((t) => {
    const matchesSearch = !query || t.name.toLowerCase().includes(query) || t.id.toLowerCase().includes(query);
    const matchesCategory = selectedCategory.value === ALL_CATEGORIES || t.category === selectedCategory.value;
    return matchesSearch && matchesCategory;
  });
});

async function fetchTemplates() {
  loading.value = true;
  loadError.value = null;
  try {
    templates.value = await TemplateService.listTemplates();
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : 'The playbook library could not be read.';
  } finally {
    loading.value = false;
  }
}

async function handleAction(id: string, mode: 'append' | 'create') {
  try {
    emit('inject', await TemplateService.getTemplate(id), mode);
  } catch (err) {
    console.error('Failed to fetch template detail', err);
  }
}

onMounted(fetchTemplates);
</script>

<template>
  <div class="flex min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2">
      <SearchInput v-model="searchQuery" label="Search playbooks" class="min-w-[12rem] flex-1" />
      <SelectInput v-model="selectedCategory" :options="categoryOptions" label="Category" />
    </div>
    <p v-if="!props.appendTarget" class="m-0 text-[length:var(--text-small)] text-muted">
      Open a file to append a playbook to it; "New file" works any time.
    </p>

    <LoadingState v-if="loading" label="Loading playbooks" :rows="3" />
    <ErrorState v-else-if="loadError" title="Could not load playbooks" :cause="loadError" next="Check the templates directory in Settings, then retry.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="fetchTemplates">Retry</BaseButton></template>
    </ErrorState>
    <EmptyState
      v-else-if="filteredTemplates.length === 0"
      :title="templates.length ? 'No playbooks match' : 'No playbooks yet'"
      :body="templates.length ? 'Try another search or category.' : 'Playbook templates appear here when the templates directory has any.'"
    />
    <ul v-else class="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(100%,280px),1fr))] gap-2 p-0">
      <li
        v-for="t in filteredTemplates"
        :key="t.id"
        data-test="playbook"
        class="flex flex-col gap-2 rounded-[var(--radius-sm)] border border-hairline bg-surface p-3"
      >
        <span class="flex flex-wrap items-center gap-2">
          <MicroLabel>{{ t.category }}</MicroLabel>
          <span class="font-mono text-[length:var(--text-micro)] text-faint">{{ t.id }}</span>
        </span>
        <h3 class="m-0 text-[length:var(--text-body)] font-semibold text-primary">{{ t.name }}</h3>
        <span class="mt-auto flex flex-wrap gap-2">
          <BaseButton variant="secondary" size="sm" icon="plus" @click="handleAction(t.id, 'create')">New file</BaseButton>
          <BaseButton
            variant="ghost"
            size="sm"
            :disabled="!props.appendTarget"
            @click="handleAction(t.id, 'append')"
          >{{ props.appendTarget ? `Append to ${props.appendTarget}` : 'Append' }}</BaseButton>
        </span>
      </li>
    </ul>
  </div>
</template>
