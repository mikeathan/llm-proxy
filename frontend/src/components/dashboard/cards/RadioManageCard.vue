<script setup lang="ts">
import { provide } from "vue"
import RadioClaim from "../../radio/RadioClaim.vue"
import RadioManageDevices from "../../radio/RadioManageDevices.vue"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"

// Dashboard card · Claim and manage your devices: the decisions about which devices are yours (Find my device, a few questions, add one by its
// address, remove one). The radar and My devices show what was decided.
// Several panels in one card: each draws its own frame and title (pitfall 74).
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="radio-manage-card">
    <RadioClaim />
    <RadioManageDevices />
  </div>
</template>
