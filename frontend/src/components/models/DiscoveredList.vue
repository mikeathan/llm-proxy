<script setup lang="ts">
import { ref, toRefs } from 'vue'
import type { AvailableModel } from '../../types'
import { extractDynamicTags, formatFileSize } from '../../utils/model-discovery'
import { useModelDiscovery } from '../../composables/useModelDiscovery'
import ModelTags from './ModelTags.vue'
import Icon from '../icons/Icon.vue'

const props = defineProps<{
  availableModels: AvailableModel[]
  alreadyConfigured: string[]
}>()

const emit = defineEmits<{
  (e: 'select', model: AvailableModel): void
}>()

const { availableModels } = toRefs(props)
const searchQuery = ref('')
const { groupedModels, expandedGroups, toggleGroup } = useModelDiscovery(availableModels, searchQuery)

function isConfigured(filename: string) {
  return props.alreadyConfigured.includes(filename)
}
</script>

<template>
  <div class="discovered-section">
    <div class="header-row">
      <h3 class="discovered-title">
        Local Model Storage
        <span class="count-badge">{{ availableModels.length }} Files</span>
      </h3>
      <div class="search-container">
        <input v-model="searchQuery" placeholder="Filter models..." class="search-input" />
      </div>
    </div>

    <div class="discovered-wrapper custom-scrollbar">
      <div v-if="!availableModels.length" class="discovered-empty">
        No models detected in scanning path.
      </div>

      <div class="groups-container">
        <div v-for="group in groupedModels" :key="group.name" class="model-group">
          <!-- Group Header -->
          <div 
            @click="toggleGroup(group.name)"
            class="group-header"
            :class="{ 'expanded': expandedGroups.has(group.name) }"
          >
            <div class="group-info">
              <div class="chevron" :class="{ 'rotate-90': expandedGroups.has(group.name) }">
                <Icon name="chevron-right" size="sm" />
              </div>
              <div class="group-details">
                <div class="group-name">{{ group.name }}</div>
                <div class="group-meta">
                   {{ group.items.length }} variant{{ group.items.length > 1 ? 's' : '' }} • {{ group.items[0]?.metadata?.architecture || 'Unknown Arch' }}
                </div>
              </div>
            </div>
            
            <div class="group-tags">
              <span 
                v-for="tag in extractDynamicTags(group.name)" 
                :key="tag.label" 
                class="model-tag" 
                :class="tag.color"
              >
                {{ tag.label }}
              </span>
            </div>
          </div>

          <!-- Group Content (Versions) -->
          <div v-if="expandedGroups.has(group.name)" class="group-content">
            <div 
              v-for="model in group.items" 
              :key="model.filename"
              class="variant-item"
              :class="{ 'configured': isConfigured(model.filename) }"
            >
              <div class="variant-info">
                <div class="flex flex-col gap-0.5">
                  <span class="variant-filename" :title="model.filename">{{ model.filename }}</span>
                  <div class="flex items-center gap-2">
                    <span class="text-[9px] text-gray-500" v-if="model.metadata.context_length">CTX: {{ Math.round(model.metadata.context_length / 1024) }}K</span>
                    <span class="text-[9px] text-gray-500" v-if="model.metadata.author">By {{ model.metadata.author }}</span>
                  </div>
                </div>
                <div class="variant-badges">
                  <ModelTags :metadata="model.metadata" />
                  <span class="size-badge">{{ formatFileSize(model.size_bytes) }}</span>
                </div>
              </div>

              <div class="variant-actions">
                <button 
                  v-if="!isConfigured(model.filename)"
                  @click.stop="emit('select', model)"
                  class="btn-select-mini"
                >
                  Select
                </button>
                <div v-else class="check-icon">
                  <Icon name="check" size="sm" />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.size-badge {
  @apply text-[10px] bg-white/5 border border-white/10 rounded px-1.5 py-0.5 text-gray-400 font-mono tracking-tighter;
}

.header-row {
  @apply mb-4 flex items-center justify-between;
}
.discovered-title {
  @apply text-xs font-bold text-gray-500 uppercase tracking-widest flex items-center gap-2;
}
.count-badge {
  @apply bg-gray-800 text-gray-500 px-1.5 py-0.5 rounded text-[10px];
}
.search-container {
  @apply flex-1 max-w-[180px] ml-4;
}
.search-input {
  @apply w-full bg-gray-900 border border-gray-700/50 rounded px-2 py-1 text-xs text-white 
         focus:border-blue-500 outline-none transition-all;
}
.discovered-wrapper {
  @apply max-h-[350px] overflow-y-auto pr-1;
}
.discovered-empty {
  @apply text-center text-gray-600 py-10 text-xs italic;
}

.model-group {
  @apply mb-2 overflow-hidden border border-gray-700/30 rounded-lg bg-gray-900/20 transition-all;
}
.group-header {
  @apply flex items-center justify-between p-3 cursor-pointer transition-all border-b border-transparent
         hover:bg-gray-800/50;
}
.group-header.expanded {
  @apply bg-gray-800/40 border-gray-700/50;
}

.group-info {
  @apply flex items-center gap-3 flex-1 min-w-0;
}
.chevron {
  @apply text-gray-600 transition-transform duration-200;
}
.group-details {
  @apply truncate;
}
.group-name {
  @apply text-xs font-bold text-gray-200 truncate;
}
.group-meta {
  @apply text-[10px] text-gray-600 font-medium;
}
.group-tags {
  @apply flex gap-1 ml-4;
}

.group-content {
  @apply bg-black/20 p-1.5 space-y-1;
}
.variant-item {
  @apply flex items-center justify-between p-2 pl-8 rounded bg-gray-800/30 border border-transparent 
         transition-all hover:border-gray-700 hover:bg-gray-800/50;
}
.variant-item.configured {
  @apply opacity-60 grayscale-[0.5];
}

.variant-info {
  @apply min-w-0 flex-1 flex items-center justify-between pr-4 gap-4;
}
.variant-badges {
  @apply flex items-center gap-2 shrink-0;
}
.variant-filename {
  @apply text-[10px] text-gray-400 truncate font-mono;
}

.btn-select-mini {
  @apply px-3 py-1 bg-gray-700 hover:bg-blue-600 text-white text-[9px] font-black uppercase tracking-widest rounded transition-colors;
}
.check-icon {
  @apply text-green-500 mr-2;
}

.model-tag {
  @apply text-[9px] font-bold px-1.5 py-0.5 rounded uppercase tracking-tighter;
}

.custom-scrollbar::-webkit-scrollbar { width: 4px; }
.custom-scrollbar::-webkit-scrollbar-thumb { background: #374151; border-radius: 10px; }
</style>
