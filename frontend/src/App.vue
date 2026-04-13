<script setup lang="ts">
import { ref, onMounted } from 'vue'
import AdminHeader from './components/layout/AdminHeader.vue'
import Dashboard from './components/dashboard/Dashboard.vue'
import Settings from './components/settings/Settings.vue'
import Logs from './components/logs/Logs.vue'
import AgentIde from './components/AgentIde/AgentIde.vue'
import { useModels } from './composables/useModels'
import { useMcpServers } from './composables/useMcpServers'

type Tab = 'dashboard' | 'settings' | 'logs' | 'agent-ide'

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

    <main :class="[activeTab === 'agent-ide' ? 'w-full px-4 md:px-6 py-4 md:py-6' : 'max-w-7xl mx-auto p-4 md:p-6', 'transition-all duration-300']">
      <div v-if="!state" class="flex justify-center items-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>

      <template v-else>
        <Dashboard v-if="activeTab === 'dashboard'" />
        <Settings  v-else-if="activeTab === 'settings'" />
        <Logs      v-else-if="activeTab === 'logs'" />
        <AgentIde v-else-if="activeTab === 'agent-ide'" />
      </template>
    </main>
  </div>
</template>
