<script setup lang="ts">
import { ref, onMounted, provide } from 'vue'
import AdminHeader from './components/layout/AdminHeader.vue'
import Dashboard from './components/dashboard/Dashboard.vue'
import Settings from './components/settings/Settings.vue'
import Logs from './components/logs/Logs.vue'
import AgentIde from './components/AgentIde/AgentIde.vue'
import { useModels } from './composables/models/useModels'
import { useMcpServers } from './composables/system/useMcpServers'
import Toast from './components/ui/Toast.vue'
import ConfirmDialog from './components/ui/ConfirmDialog.vue'
import { useConfirm } from './composables/ui/useConfirm'
import type { AppTab, SettingsTab } from './types'

const { isOpen, options, handleConfirm, handleCancel } = useConfirm()

const activeTab = ref<AppTab>('dashboard')
const activeSettingsTab = ref<SettingsTab>('local')

provide('setActiveTab', (tab: AppTab) => {
  activeTab.value = tab
})

provide('activeSettingsTab', activeSettingsTab)
provide('setActiveSettingsTab', (tab: SettingsTab) => {
  activeSettingsTab.value = tab
  activeTab.value = 'settings'
})

const { state, refresh: refreshModels } = useModels()
const { refresh: refreshMcp } = useMcpServers()

onMounted(() => {
  Promise.all([refreshModels(), refreshMcp()])
})
</script>

<template>
  <div class="min-h-screen bg-gray-900 text-gray-300 font-sans">
    <AdminHeader v-model:activeTab="activeTab" />

    <main :class="[activeTab === 'agent-ide' ? 'w-full px-4 md:px-6 py-4 md:py-6' : 'max-w-7xl mx-auto p-4 md:p-6', 'transition-[padding,max-width] duration-300']">
      <div v-if="!state" class="flex justify-center items-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>

      <div v-else class="h-full">
        <Dashboard v-if="activeTab === 'dashboard'" />
        <Settings  v-else-if="activeTab === 'settings'" />
        <Logs      v-else-if="activeTab === 'logs'" />
        <AgentIde v-else-if="activeTab === 'agent-ide'" />
      </div>
    </main>
    <Toast />
    <ConfirmDialog 
      v-model="isOpen"
      v-bind="options"
      @confirm="handleConfirm"
      @cancel="handleCancel"
    />
  </div>
</template>
