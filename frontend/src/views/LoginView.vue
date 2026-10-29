<script setup lang="ts">
import { onMounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import AuthCard from "../components/auth/AuthCard.vue"
import PassphraseInput from "../components/auth/PassphraseInput.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import { useAuth } from "../composables/auth/useAuth"
import { safeNext } from "../utils/auth/guard"
import { errorMessage } from "../utils/errors"
import { PRODUCT_NAME } from "../config/brand"

// Sign in: one field. A username would add nothing (there is one operator). The hidden username field is for password
// managers, which look for one to file the passphrase under.
const SIGN_IN_FAILED = "Sign-in failed. Check the connection and try again."
const route = useRoute()
const router = useRouter()
const auth = useAuth()

const passphrase = ref("")
const error = ref("")
const busy = ref(false)

async function submit() {
  if (!passphrase.value || busy.value) return
  busy.value = true
  error.value = ""
  try {
    await auth.login(passphrase.value)
    await router.replace(safeNext(route.query.next))
  } catch (err) {
    error.value = errorMessage(err, SIGN_IN_FAILED)
    passphrase.value = ""
  } finally {
    busy.value = false
  }
}

onMounted(() => { void auth.refresh() })
</script>

<template>
  <AuthCard title="Sign in" lead="Enter your passphrase to continue.">
    <form class="flex flex-col gap-3" novalidate @submit.prevent="submit">
      <input type="text" name="username" value="operator" autocomplete="username" class="sr-only" tabindex="-1" aria-hidden="true" readonly />
      <PassphraseInput v-model="passphrase" label="Passphrase" autocomplete="current-password" :error="error" />
      <BaseButton type="submit" :loading="busy" :disabled="!passphrase" class-name="max-sm:!h-11">Sign in</BaseButton>
    </form>
    <p class="m-0 text-[length:var(--text-small)] text-faint">Forgotten it? On the server, run <code class="font-mono text-secondary">{{ PRODUCT_NAME }} auth reset</code>, then <code class="font-mono text-secondary">{{ PRODUCT_NAME }} auth setup-code</code>.</p>
  </AuthCard>
</template>
