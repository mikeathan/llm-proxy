<script setup lang="ts">
import { computed, ref } from "vue"
import { useRouter } from "vue-router"
import AuthCard from "../components/auth/AuthCard.vue"
import PassphraseInput from "../components/auth/PassphraseInput.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import FormField from "../components/common/forms/FormField.vue"
import { useAuth } from "../composables/auth/useAuth"
import { errorMessage } from "../utils/errors"
import { PRODUCT_NAME } from "../config/brand"

// First run: the person at the server makes a one-time code with `auth setup-code`, and enters it here with the
// passphrase they choose. The same length rule as the server is shown before it is asked, so the usual mistake costs no trip.
const MIN_LENGTH = 12
const SETUP_FAILED = "Setup failed. Check the connection and try again."
const router = useRouter()
const auth = useAuth()

const code = ref("")
const passphrase = ref("")
const repeat = ref("")
const error = ref("")
const busy = ref(false)

const mismatch = computed(() => repeat.value !== "" && repeat.value !== passphrase.value)
const tooShort = computed(() => passphrase.value !== "" && [...passphrase.value].length < MIN_LENGTH)
const ready = computed(() => code.value.trim() !== "" && [...passphrase.value].length >= MIN_LENGTH && repeat.value === passphrase.value)

async function submit() {
  if (!ready.value || busy.value) return
  busy.value = true
  error.value = ""
  try {
    await auth.setup(code.value.trim(), passphrase.value)
    await router.replace("/")
  } catch (err) {
    error.value = errorMessage(err, SETUP_FAILED)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthCard title="Set up sign-in" lead="Until you do, anyone on your network can change settings and run the assistant.">
    <p class="m-0 text-[length:var(--text-small)] text-secondary">On the server, run <code class="font-mono text-primary">{{ PRODUCT_NAME }} auth setup-code</code> and enter the code it prints.</p>
    <form class="flex flex-col gap-3" novalidate @submit.prevent="submit">
      <FormField label="Setup code">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="code" type="text" autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" :aria-describedby="describedBy" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 font-mono uppercase text-primary focus-visible:outline-none focus-visible:ring-2" />
        </template>
      </FormField>
      <PassphraseInput v-model="passphrase" label="New passphrase" autocomplete="new-password" :hint="`${MIN_LENGTH} characters or more. A few words is fine.`" :error="tooShort ? `Use at least ${MIN_LENGTH} characters.` : ''" />
      <PassphraseInput v-model="repeat" label="Repeat it" autocomplete="new-password" :error="mismatch ? 'The two do not match.' : error" />
      <BaseButton type="submit" :loading="busy" :disabled="!ready" class-name="max-sm:!h-11">Set up sign-in</BaseButton>
    </form>
  </AuthCard>
</template>
