<script setup lang="ts">
import ProcessLogs from '../components/ProcessLogs.vue'
import { useMetrics } from '../composables/useMetrics'

const { logs } = useMetrics()
</script>

<template>
  <ProcessLogs :logs="logs" />
</template>
