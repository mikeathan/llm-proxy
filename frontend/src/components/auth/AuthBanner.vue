<script setup lang="ts">
import { computed } from "vue"
import Icon from "../icons/Icon.vue"
import { useAuth } from "../../composables/auth/useAuth"
import { toSettings } from "../../router/routes"

// Shown while sign-in is not set up. It stays until it is: the interface is open to anyone who can reach the server.
// Beyond this machine the wording is stronger; the server's own address is not known here, so it says what to check.
const auth = useAuth()
const visible = computed(() => auth.status.value !== null && !auth.status.value.required)
const encrypted = computed(() => auth.status.value?.encrypted === true)
</script>

<template>
  <div v-if="visible" role="status" data-test="auth-banner" class="mb-4 border border-state-running bg-state-running/[0.08] px-3 py-2.5 text-state-running">
    <div class="flex flex-wrap items-center gap-2">
      <Icon name="warning" size="sm" class-name="shrink-0" />
      <span class="text-[length:var(--text-small)] font-medium leading-snug">
        Sign-in is not set up. Anyone on your network can change settings and run the assistant.<template v-if="!encrypted"> The connection is not encrypted either.</template>
      </span>
      <RouterLink :to="toSettings('sign-in')" class="text-[length:var(--text-small)] font-semibold underline underline-offset-2">Set up sign-in</RouterLink>
    </div>
  </div>
</template>
