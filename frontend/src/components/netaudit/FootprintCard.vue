<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useFootprint } from "../../composables/netaudit/useFootprint"
import { footprintResultText } from "../../utils/netaudit/footprint"
import { formatRelativeTime } from "../../utils/format/time"
import {
  FOOTPRINT_ADD_DOMAIN, FOOTPRINT_ADD_EMAIL, FOOTPRINT_CHECKING, FOOTPRINT_CHECK_NOW, FOOTPRINT_CHECK_TITLES, FOOTPRINT_DOMAIN_LABEL, FOOTPRINT_DOMAIN_PLACEHOLDER, FOOTPRINT_EMAIL_LABEL,
  FOOTPRINT_EMAIL_PLACEHOLDER, FOOTPRINT_FREQUENCIES, FOOTPRINT_FREQUENCY_LABEL, FOOTPRINT_INTRO, FOOTPRINT_KEY_HINT, FOOTPRINT_KEY_LABEL, FOOTPRINT_KEY_MISSING, FOOTPRINT_KEY_REMOVE, FOOTPRINT_KEY_SAVE,
  FOOTPRINT_KEY_SAVED, FOOTPRINT_LOAD_FAILED, FOOTPRINT_NONE, FOOTPRINT_REMOVE, FOOTPRINT_SERVICES_HINT, FOOTPRINT_SERVICES_TITLE, FOOTPRINT_SERVICE_TITLES, FOOTPRINT_TITLE,
} from "../../constants/netaudit"
import type { FootprintFrequency } from "../../types/netaudit"

// Network → Settings → Your domains and email addresses: what is watched outside the network, how often, and which outside services
// may be asked (each says what it learns). Looking at a watch by hand is "Check now"; the nightly report reads the stored answers.
const { state, error, busy, checking, load, check, apply, saveKey, removeKey } = useFootprint()
const domain = ref("")
const email = ref("")
const key = ref("")
const INPUT = "h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8"

async function addDomain() {
  if (await apply({ action: "add_domain", name: domain.value })) domain.value = ""
}
async function addEmail() {
  if (await apply({ action: "add_email", address: email.value })) email.value = ""
}
async function storeKey() {
  if (await saveKey(key.value)) key.value = ""
}
onMounted(load)
</script>

<template>
  <Panel :title="FOOTPRINT_TITLE">
    <div class="flex flex-col gap-3" data-test="footprint">
      <ErrorState v-if="error && !state" :title="FOOTPRINT_LOAD_FAILED" :cause="error" next="">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Try again</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading…" />

      <template v-else>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ FOOTPRINT_INTRO }}</p>

        <section class="flex flex-col gap-2" :aria-label="FOOTPRINT_SERVICES_TITLE" data-test="footprint-services">
          <h3 class="m-0 text-[length:var(--text-small)] font-semibold text-primary">{{ FOOTPRINT_SERVICES_TITLE }}</h3>
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ FOOTPRINT_SERVICES_HINT }}</p>
          <label v-for="s in state.services" :key="s.id" class="flex min-h-11 items-start gap-2 text-[length:var(--text-small)] text-secondary sm:min-h-8">
            <input type="checkbox" class="mt-1" :checked="s.on" :disabled="busy" :data-test="`service-${s.id}`" @change="apply({ action: 'service', service: s.id, on: ($event.target as HTMLInputElement).checked })" />
            <span><span class="font-semibold text-primary">{{ FOOTPRINT_SERVICE_TITLES[s.id] }}</span><br />{{ s.learns }}</span>
          </label>
          <form class="flex flex-col gap-2" @submit.prevent="storeKey">
            <FormField :label="FOOTPRINT_KEY_LABEL" :hint="state.key_saved ? FOOTPRINT_KEY_SAVED : FOOTPRINT_KEY_HINT">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="key" type="password" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" data-test="footprint-key" :class="INPUT" />
              </template>
            </FormField>
            <div class="flex flex-wrap gap-2">
              <BaseButton variant="secondary" size="sm" type="submit" class-name="max-sm:!h-11" :disabled="busy || !key" data-test="footprint-key-save">{{ FOOTPRINT_KEY_SAVE }}</BaseButton>
              <BaseButton v-if="state.key_saved" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="footprint-key-remove" @click="removeKey">{{ FOOTPRINT_KEY_REMOVE }}</BaseButton>
            </div>
          </form>
        </section>

        <p v-if="!state.watches.length" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="footprint-none">{{ FOOTPRINT_NONE }}</p>
        <ul v-else class="m-0 flex list-none flex-col gap-3 p-0" data-test="footprint-watches">
          <li v-for="w in state.watches" :key="w.label" class="flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline p-3" :data-test="`watch-${w.label}`">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="font-mono text-[length:var(--text-small)] text-primary">{{ w.name }}</span>
              <div class="flex flex-wrap items-center gap-2">
                <select :value="w.frequency" :aria-label="FOOTPRINT_FREQUENCY_LABEL" :disabled="busy" :class="INPUT" data-test="watch-frequency" @change="apply({ action: 'frequency', label: w.label, frequency: ($event.target as HTMLSelectElement).value as FootprintFrequency })">
                  <option v-for="f in FOOTPRINT_FREQUENCIES" :key="f.value" :value="f.value">{{ f.label }}</option>
                </select>
                <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="!!checking" data-test="watch-check" @click="check(w.label)">{{ checking === w.label ? FOOTPRINT_CHECKING : FOOTPRINT_CHECK_NOW }}</BaseButton>
                <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="watch-remove" @click="apply({ action: 'remove', label: w.label })">{{ FOOTPRINT_REMOVE }}</BaseButton>
              </div>
            </div>
            <ul class="m-0 flex list-none flex-col gap-1 p-0 text-[length:var(--text-small)] text-secondary">
              <li v-for="r in w.results" :key="r.check" :data-test="`result-${r.check}`">
                <span class="font-semibold">{{ FOOTPRINT_CHECK_TITLES[r.check] }}:</span> {{ footprintResultText(r) }} <span class="text-muted">({{ formatRelativeTime(r.at) }})</span>
              </li>
              <li v-if="w.kind === 'email' && !state.key_saved" class="text-state-running" data-test="needs-key">{{ FOOTPRINT_KEY_MISSING }}</li>
            </ul>
          </li>
        </ul>

        <form class="flex flex-col gap-2" @submit.prevent="addDomain">
          <FormField :label="FOOTPRINT_DOMAIN_LABEL">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="domain" type="text" inputmode="url" autocomplete="off" spellcheck="false" :placeholder="FOOTPRINT_DOMAIN_PLACEHOLDER" :aria-describedby="describedBy" data-test="footprint-domain" :class="INPUT" />
            </template>
          </FormField>
          <BaseButton variant="secondary" size="sm" type="submit" class-name="self-start max-sm:!h-11" :disabled="busy || !domain" data-test="footprint-domain-add">{{ FOOTPRINT_ADD_DOMAIN }}</BaseButton>
        </form>
        <form class="flex flex-col gap-2" @submit.prevent="addEmail">
          <FormField :label="FOOTPRINT_EMAIL_LABEL">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="email" type="email" autocomplete="off" spellcheck="false" :placeholder="FOOTPRINT_EMAIL_PLACEHOLDER" :aria-describedby="describedBy" data-test="footprint-email" :class="INPUT" />
            </template>
          </FormField>
          <BaseButton variant="secondary" size="sm" type="submit" class-name="self-start max-sm:!h-11" :disabled="busy || !email" data-test="footprint-email-add">{{ FOOTPRINT_ADD_EMAIL }}</BaseButton>
        </form>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="footprint-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
