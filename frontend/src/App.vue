<script setup lang="ts">
import { ref, onMounted } from 'vue'
import AdminHeader from './components/AdminHeader.vue'
import DashboardView from './views/DashboardView.vue'
import SettingsView from './views/SettingsView.vue'
import LogsView from './views/LogsView.vue'
import WorkspacesView from './views/WorkspacesView.vue'
import { useModels } from './composables/useModels'
import { useMcpServers } from './composables/useMcpServers'

type Tab = 'dashboard' | 'settings' | 'logs' | 'workspaces'

const activeTab = ref<Tab>('dashboard')

const { state, refresh: refreshModels } = useModels()
const { refresh: refreshMcp } = useMcpServers()

onMounted(() => {
  Promise.all([refreshModels(), refreshMcp()])
})
</script>

<template>
  <div class="min-h-screen bg-gray-900 text-gray-300 font-sans">
    <AdminHeader v-model:activeTab="activeTab" />

    <main class="max-w-7xl mx-auto p-4 md:p-6 space-y-6">
      <div v-if="!state" class="flex justify-center items-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>

      <template v-else>
        <DashboardView v-if="activeTab === 'dashboard'" />
        <SettingsView  v-else-if="activeTab === 'settings'" />
        <LogsView      v-else-if="activeTab === 'logs'" />
        <WorkspacesView v-else-if="activeTab === 'workspaces'" />
      </template>
    </main>
  </div>
</template>
