<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import { useSharedRouterConnections } from "../../composables/netaudit/useRouterConnections"
import { formatRelativeTime } from "../../utils/format/time"
import { toNetwork } from "../../router/routes"
import {
  ROUTER_CONN_ADDRESS, ROUTER_CONN_ADDRESS_HINT, ROUTER_CONN_CHANGE, ROUTER_CONN_CHANGE_ADDRESS, ROUTER_CONN_CHECK, ROUTER_CONN_CHECK_HINT, ROUTER_CONN_FINGERPRINT, ROUTER_CONN_INTRO, ROUTER_CONN_KEY, ROUTER_CONN_KEY_HINT, ROUTER_CONN_KEY_REMOVE, ROUTER_CONN_KEY_SAVE, ROUTER_CONN_NEVER,
  ROUTER_CONN_PASSPHRASE, ROUTER_CONN_READ, ROUTER_CONN_READING, ROUTER_CONN_SAVE, ROUTER_CONN_STEPS, ROUTER_CONN_TRUNCATED, ROUTER_CONN_TRUST, ROUTER_CONN_TRUSTED, ROUTER_CONN_USER, ROUTER_CONN_USER_HINT,
  ROUTER_CONN_WATCH_LINK, ROUTER_CONN_WATCH_OFF, ROUTER_CONN_WATCH_ON, ROUTER_CONN_WHEN,
} from "../../constants/netaudit"

// Where each device on this network connected, from the router's own connection table (Constitution I.15). Set up once in three steps (where the
// router is, the key to sign in with, the router's own key to trust), then read when the operator presses; every step asks for the passphrase. It
// shows no byte counts, because the router keeps none. A place can be sent to the route trace. Names and addresses come from the router and are
// shown as text only.
// The reading itself (the devices and their places) is drawn by the device list of the Traffic page; this part only sets up and reads.
const conn = useSharedRouterConnections()

const address = ref("")
const user = ref("")
const key = ref("")
const changing = ref(false)
const showSetup = computed(() => conn.step.value !== "ready" || changing.value)
// What the form starts with: what is already saved.
onMounted(async () => {
  await conn.load()
  address.value = conn.state.value?.address ?? ""
  user.value = conn.state.value?.user ?? ""
})

async function saveTarget() {
  if (await conn.saveTarget(address.value, user.value)) changing.value = false
}
async function saveKey() {
  if (await conn.saveKey(key.value)) key.value = ""
}

const last = computed(() => conn.state.value?.last ?? null)
</script>

<template>
  <div class="flex flex-col gap-3" data-test="router-conn">
    <p v-if="conn.step.value !== 'ready'" class="m-0 text-secondary" data-test="router-intro">{{ ROUTER_CONN_INTRO }}</p>
    <p v-if="conn.error.value" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="router-error">{{ conn.error.value }}</p>

    <section v-if="conn.loaded.value && showSetup" class="flex flex-col gap-3 border-t border-hairline pt-3" data-test="router-setup">
      <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted" data-test="router-step">{{ ROUTER_CONN_STEPS[conn.step.value === "ready" ? "target" : conn.step.value] }}</h3>

      <form v-if="conn.step.value === 'target' || changing" class="flex flex-col gap-2" @submit.prevent="saveTarget">
        <FormField :label="ROUTER_CONN_ADDRESS" :hint="ROUTER_CONN_ADDRESS_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="address" type="text" inputmode="decimal" autocomplete="off" spellcheck="false" placeholder="192.168.50.1" :aria-describedby="describedBy" class="h-11 w-full max-w-sm min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" data-test="router-address" />
          </template>
        </FormField>
        <FormField :label="ROUTER_CONN_USER" :hint="ROUTER_CONN_USER_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="user" type="text" autocomplete="off" autocapitalize="none" spellcheck="false" placeholder="admin" :aria-describedby="describedBy" class="h-11 w-full max-w-sm min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" data-test="router-user" />
          </template>
        </FormField>
        <div class="flex flex-wrap items-center gap-3">
          <BaseButton type="submit" variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="conn.busy.value === 'target'" :disabled="!!conn.busy.value || !address.trim() || !user.trim()" data-test="router-save-target">{{ ROUTER_CONN_SAVE }}</BaseButton>
          <span class="text-[length:var(--text-small)] text-muted">{{ ROUTER_CONN_PASSPHRASE }}</span>
        </div>
      </form>

      <form v-else-if="conn.step.value === 'key'" class="flex flex-col gap-2" @submit.prevent="saveKey">
        <FormField :label="ROUTER_CONN_KEY" :hint="ROUTER_CONN_KEY_HINT">
          <template #default="{ id, describedBy }">
            <textarea :id="id" v-model="key" rows="5" autocomplete="off" autocapitalize="none" spellcheck="false" :aria-describedby="describedBy" class="min-h-24 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-1.5 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" data-test="router-key" />
          </template>
        </FormField>
        <div class="flex flex-wrap items-center gap-3">
          <BaseButton type="submit" variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="conn.busy.value === 'key'" :disabled="!!conn.busy.value || !key.trim()" data-test="router-save-key">{{ ROUTER_CONN_KEY_SAVE }}</BaseButton>
          <span class="text-[length:var(--text-small)] text-muted">{{ ROUTER_CONN_PASSPHRASE }}</span>
        </div>
      </form>

      <div v-else-if="conn.step.value === 'trust'" class="flex flex-col gap-2">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ ROUTER_CONN_CHECK_HINT }}</p>
        <p v-if="conn.candidate.value" class="m-0 break-all font-mono text-primary" data-test="router-fingerprint">{{ ROUTER_CONN_FINGERPRINT(conn.candidate.value) }}</p>
        <div class="flex flex-wrap items-center gap-3">
          <BaseButton v-if="!conn.candidate.value" variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="conn.busy.value === 'fingerprint'" :disabled="!!conn.busy.value" data-test="router-check" @click="conn.check()">{{ ROUTER_CONN_CHECK }}</BaseButton>
          <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="conn.busy.value === 'pin'" :disabled="!!conn.busy.value" data-test="router-trust" @click="conn.trust()">{{ ROUTER_CONN_TRUST }}</BaseButton>
          <span class="text-[length:var(--text-small)] text-muted">{{ ROUTER_CONN_PASSPHRASE }}</span>
        </div>
      </div>

      <div v-if="conn.step.value !== 'target' || conn.state.value?.key_saved" class="flex flex-wrap items-center gap-3">
        <BaseButton v-if="conn.step.value !== 'ready' && conn.step.value !== 'target'" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="!!conn.busy.value" data-test="router-change-address" @click="changing = !changing">{{ ROUTER_CONN_CHANGE_ADDRESS }}</BaseButton>
        <BaseButton v-if="conn.state.value?.key_saved" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="!!conn.busy.value" data-test="router-remove-key" @click="conn.removeKey()">{{ ROUTER_CONN_KEY_REMOVE }}</BaseButton>
      </div>
    </section>

    <template v-if="conn.loaded.value && conn.step.value === 'ready'">
      <div class="flex flex-wrap items-center gap-3 border-t border-hairline pt-3">
        <BaseButton :loading="conn.busy.value === 'read'" :disabled="!!conn.busy.value" class-name="max-sm:!h-11" data-test="router-read" @click="conn.read()">{{ conn.busy.value === "read" ? ROUTER_CONN_READING : ROUTER_CONN_READ }}</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="router-change" @click="changing = !changing">{{ ROUTER_CONN_CHANGE }}</BaseButton>
        <span class="text-[length:var(--text-small)] text-muted">{{ ROUTER_CONN_PASSPHRASE }}</span>
      </div>
      <p class="m-0 break-all text-[length:var(--text-small)] text-muted" data-test="router-trusted">{{ ROUTER_CONN_TRUSTED(conn.state.value!.fingerprint!) }}</p>

      <p v-if="conn.state.value?.watching" class="m-0 text-[length:var(--text-small)] text-muted" data-test="router-watch-on">{{ ROUTER_CONN_WATCH_ON }}</p>
      <p v-else class="m-0 flex flex-wrap items-center gap-x-3 text-[length:var(--text-small)] text-muted" data-test="router-watch-off">
        <span>{{ ROUTER_CONN_WATCH_OFF }}</span>
        <RouterLink :to="toNetwork('modules')" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8">{{ ROUTER_CONN_WATCH_LINK }}</RouterLink>
      </p>

      <p v-if="!last" class="m-0 text-muted" data-test="router-never">{{ ROUTER_CONN_NEVER }}</p>
      <template v-else>
        <p class="m-0 text-secondary" data-test="router-when">{{ ROUTER_CONN_WHEN(formatRelativeTime(last.at), last.lines, last.kept) }}</p>
        <p v-if="last.truncated" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="router-truncated">{{ ROUTER_CONN_TRUNCATED }}</p>
      </template>
    </template>
  </div>
</template>
