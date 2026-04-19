<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { TemplateService } from '../../../services/templateService';
import type { Template, TemplateMetadata } from '../../../types/templates';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'inject', template: Template, mode: 'append' | 'create'): void;
}>();

const templates = ref<TemplateMetadata[]>([]);
const loading = ref(false);
const searchQuery = ref('');
const selectedCategory = ref<string>('All');

const categories = computed(() => {
  const cats = new Set(templates.value.map(t => t.category));
  return ['All', ...Array.from(cats)].sort();
});

const filteredTemplates = computed(() => {
  return templates.value.filter(t => {
    const matchesSearch = t.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
                         t.id.toLowerCase().includes(searchQuery.value.toLowerCase());
    const matchesCategory = selectedCategory.value === 'All' || t.category === selectedCategory.value;
    return matchesSearch && matchesCategory;
  });
});

const fetchTemplates = async () => {
  loading.value = true;
  try {
    templates.value = await TemplateService.listTemplates();
  } catch (err) {
    console.error('Failed to load templates', err);
  } finally {
    loading.value = false;
  }
};

const handleAction = async (id: string, mode: 'append' | 'create') => {
  try {
    const full = await TemplateService.getTemplate(id);
    emit('inject', full, mode);
    emit('close');
  } catch (err) {
    console.error('Failed to fetch template detail', err);
  }
};

onMounted(fetchTemplates);
</script>

<template>
  <div v-if="show" class="drawer-overlay" @click.self="emit('close')">
    <div class="drawer-panel shadow-2xl">
      <div class="drawer-header">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-xl font-bold text-white">Task Playbooks</h2>
            <p class="text-xs text-gray-400 mt-1 italic">Inject expert-crafted automation steps</p>
          </div>
          <button @click="emit('close')" class="close-btn group">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <!-- Search & Filter -->
        <div class="mt-6 flex flex-col gap-4">
          <div class="relative">
            <input 
              v-model="searchQuery"
              type="text" 
              placeholder="Search playbooks..." 
              class="search-input"
            />
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 absolute left-3 top-2.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </div>

          <div class="flex flex-wrap gap-2">
            <button 
              v-for="cat in categories" 
              :key="cat"
              @click="selectedCategory = cat"
              :class="['filter-pill', selectedCategory === cat ? 'filter-pill--active' : '']"
            >
              {{ cat }}
            </button>
          </div>
        </div>
      </div>

      <div class="drawer-content">
        <div v-if="loading" class="flex flex-col items-center justify-center py-20 gap-4">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
          <span class="text-sm text-gray-500">Indexing playbooks...</span>
        </div>

        <div v-else-if="filteredTemplates.length === 0" class="flex flex-col items-center justify-center py-20 text-gray-500 italic">
          <p>No playbooks found matching your criteria.</p>
        </div>

        <div v-else class="grid grid-cols-1 gap-3 p-1">
            <div 
              v-for="t in filteredTemplates" 
              :key="t.id"
              class="template-card group"
            >
              <div class="card-content">
                <div class="flex flex-col gap-1 flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="category-tag">{{ t.category }}</span>
                    <span class="text-[9px] font-mono text-gray-600">{{ t.id }}</span>
                  </div>
                  <h3 class="template-name truncate">{{ t.name }}</h3>
                </div>
                
                <div class="template-actions">
                  <button 
                    @click="handleAction(t.id, 'append')"
                    class="mini-action-btn append-btn"
                    title="Append to selection"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M12 4v16m8-8H4" />
                    </svg>
                    <span>Append</span>
                  </button>
                  <button 
                    @click="handleAction(t.id, 'create')"
                    class="mini-action-btn create-btn"
                    title="Create new file"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                    </svg>
                    <span>New</span>
                  </button>
                </div>
              </div>
            </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.drawer-overlay {
  @apply fixed inset-0 z-[100] bg-black/60 backdrop-blur-sm flex justify-end animate-in fade-in duration-300;
}

.drawer-panel {
  @apply w-full max-w-md h-full bg-gray-900 border-l border-white/10 flex flex-col animate-in slide-in-from-right duration-500;
}

.drawer-header {
  @apply p-8 border-b border-gray-800/50 bg-gray-900/50;
}

.close-btn {
  @apply p-2 bg-gray-800 hover:bg-gray-700 text-gray-400 hover:text-white rounded-full transition-all;
}

.search-input {
  @apply w-full bg-black/40 border border-gray-800 rounded-lg py-2 pl-10 pr-4 text-sm text-white focus:border-blue-500 focus:outline-none transition-all;
}

.filter-pill {
  @apply px-3 py-1 rounded-full text-[10px] font-bold uppercase tracking-wider bg-gray-800 text-gray-500 hover:text-gray-300 transition-all border border-transparent;
}

.filter-pill--active {
  @apply bg-blue-600/20 text-blue-400 border-blue-600/30;
}

.drawer-content {
  @apply flex-1 overflow-y-auto p-8;
}

.template-card {
  @apply relative p-4 bg-gray-900/40 border border-white/5 rounded-xl transition-all hover:bg-gray-800/60 overflow-hidden;
}

.card-content {
  @apply flex items-center justify-between gap-4;
}

.category-tag {
  @apply px-1.5 py-0.5 rounded text-[8px] font-black uppercase bg-gray-800 text-gray-500 border border-white/5;
}

.template-name {
  @apply text-sm font-bold text-gray-400 group-hover:text-gray-100 transition-colors;
}

.template-actions {
  @apply flex gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity duration-200;
}

.mini-action-btn {
  @apply flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[9px] font-black uppercase tracking-tighter transition-all hover:scale-105 active:scale-95;
}

.append-btn {
  @apply bg-blue-500/10 hover:bg-blue-500/20 text-blue-400 border border-blue-500/20;
}

.create-btn {
  @apply bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 border border-emerald-500/20;
}
</style>
