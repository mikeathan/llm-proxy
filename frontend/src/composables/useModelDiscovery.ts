import { ref, computed, type Ref } from 'vue';
import type { AvailableModel } from '../types';

export function useModelDiscovery(
  availableModels: Ref<AvailableModel[]>,
  searchQuery: Ref<string>
) {
  const expandedGroups = ref<Set<string>>(new Set());

  const groupedModels = computed(() => {
    const groups: Record<string, { name: string; items: AvailableModel[] }> = {};
    
    const q = searchQuery.value.toLowerCase();
    const filtered = availableModels.value.filter(m => 
      m.name.toLowerCase().includes(q) || m.filename.toLowerCase().includes(q)
    );

    for (const m of filtered) {
      const base = m.metadata?.name || m.name || 'Unknown';
      const key = base.toLowerCase();
      
      if (!groups[key]) {
        groups[key] = { 
          name: base, 
          items: []
        };
      }
      groups[key].items.push(m);
    }
    
    return Object.values(groups).sort((a, b) => a.name.localeCompare(b.name));
  });

  function toggleGroup(name: string) {
    if (expandedGroups.value.has(name)) {
      expandedGroups.value.delete(name);
    } else {
      expandedGroups.value.add(name);
    }
  }

  return {
    groupedModels,
    expandedGroups,
    toggleGroup
  };
}
