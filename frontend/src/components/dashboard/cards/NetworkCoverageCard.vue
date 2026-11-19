<script setup lang="ts">
import { useRouter } from "vue-router"
import CoverageStrip from "../../netaudit/CoverageStrip.vue"
import { toNetwork } from "../../../router/routes"

// Dashboard card · Coverage: what is watching and what is not. Opening a tab goes to the Network page, where the modules are switched.
const router = useRouter()
</script>

<template>
  <CoverageStrip data-test="network-coverage-card" @open-tab="router.push(toNetwork($event))" />
</template>
