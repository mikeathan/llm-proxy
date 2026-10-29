<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import PassphraseInput from "../auth/PassphraseInput.vue"
import ExposurePanel from "./ExposurePanel.vue"
import { sessionLengthProblem } from "../../utils/auth/sessionLength"
import { copyText } from "../../utils/clipboard"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useSignInSettings } from "../../composables/auth/useSignInSettings"
import { formatAbsoluteTime } from "../../utils/format/time"
import { ROUTE_NAMES } from "../../types/routes"
import { MIN_PASSPHRASE_LENGTH, TOKEN_NAME_MAX, TOKEN_SCOPE_OPTIONS, EVENT_LABELS } from "../../constants/auth"
import type { TokenScope } from "../../types/auth"

// Settings · Sign-in: change the passphrase, see where you are signed in, make and revoke script tokens, and read the recent
// sign-in log. Nothing here shows a secret except a new token, once. Making a token asks for the passphrase again.
const router = useRouter()
const { confirm } = useConfirm()
const s = useSignInSettings()

const oldPass = ref("")
const newPass = ref("")
const passDone = ref(false)
const tooShort = computed(() => newPass.value !== "" && [...newPass.value].length < MIN_PASSPHRASE_LENGTH)

async function changePass() {
  passDone.value = false
  if (await s.changePassphrase(oldPass.value, newPass.value)) {
    oldPass.value = ""
    newPass.value = ""
    passDone.value = true
  }
}

const idleHours = ref(0)
const absoluteDays = ref(0)
const lengthSaved = ref(false)
watch(s.sessionLength, (len) => {
  if (!len) return
  idleHours.value = len.idle_hours
  absoluteDays.value = len.absolute_days
}, { immediate: true })
const lengthProblem = computed(() => (s.sessionLength.value ? sessionLengthProblem(idleHours.value, absoluteDays.value, s.sessionLength.value) : ""))
async function saveLength() {
  lengthSaved.value = false
  if (lengthProblem.value) return
  lengthSaved.value = await s.saveSessionLength(idleHours.value, absoluteDays.value)
}

const tokenName = ref("")
const tokenScope = ref<TokenScope>("admin")
async function makeToken() {
  if (await s.createToken(tokenName.value.trim(), tokenScope.value)) tokenName.value = ""
}
const copied = ref(false)
const copyFailed = ref(false)
const tokenEl = ref<HTMLElement | null>(null)
async function copyToken() {
  if (!s.created.value) return
  copied.value = await copyText(s.created.value.token)
  copyFailed.value = !copied.value
  if (copyFailed.value) selectToken()
}
// When no copy works the token is selected, so the person only has to press the copy shortcut.
function selectToken() {
  const el = tokenEl.value
  const selection = window.getSelection()
  if (!el || !selection) return
  const range = document.createRange()
  range.selectNodeContents(el)
  selection.removeAllRanges()
  selection.addRange(range)
}

async function endEverywhere() {
  const ok = await confirm({ title: "Sign out everywhere?", message: "Every browser, including this one, will have to sign in again. Script tokens keep working.", confirmText: "Sign out everywhere", type: "warning" })
  if (ok && (await s.signOutEverywhere())) await router.replace({ name: ROUTE_NAMES.login })
}

async function revokeToken(id: string, name: string) {
  const ok = await confirm({ title: "Revoke this token?", message: `Anything using “${name}” stops working at once.`, confirmText: "Revoke", type: "warning" })
  if (ok) await s.revokeToken(id)
}

onMounted(() => { void s.load() })
</script>

<template>
  <div class="flex min-w-0 flex-col gap-4" data-test="sign-in-settings">
    <ExposurePanel />
    <LoadingState v-if="s.loading.value && !s.sessions.value.length" label="Reading the sign-in details" />
    <ErrorState v-else-if="s.error.value" title="The sign-in details could not be read" :cause="s.error.value" next="Check that the server is running, then try again.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="s.load">Retry</BaseButton></template>
    </ErrorState>
    <template v-else>
      <p v-if="s.actionError.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="action-error">{{ s.actionError.value }}</p>

      <Panel title="Passphrase" preserve-case>
        <form class="flex max-w-md flex-col gap-3" novalidate @submit.prevent="changePass">
          <PassphraseInput v-model="oldPass" label="Current passphrase" autocomplete="current-password" />
          <PassphraseInput v-model="newPass" label="New passphrase" autocomplete="new-password" :hint="`${MIN_PASSPHRASE_LENGTH} characters or more.`" :error="tooShort ? `Use at least ${MIN_PASSPHRASE_LENGTH} characters.` : ''" />
          <div class="flex flex-wrap items-center gap-3">
            <BaseButton type="submit" :disabled="!oldPass || !newPass || tooShort" class-name="max-sm:!h-11">Change passphrase</BaseButton>
            <span v-if="passDone" role="status" class="text-[length:var(--text-small)] text-state-success" data-test="pass-done">Changed. Your other sessions were signed out.</span>
          </div>
        </form>
      </Panel>

      <Panel v-if="s.sessionLength.value" title="How long you stay signed in" preserve-case>
        <form class="flex max-w-xl flex-col gap-3" novalidate data-test="session-length-form" @submit.prevent="saveLength">
          <p class="m-0 text-[length:var(--text-small)] text-secondary">A browser is signed out after this long without use, and never stays signed in longer than the absolute limit however busy it is. A change applies to the next request, with no restart, and asks for your passphrase.</p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormField label="Signed out after idle (hours)" :hint="`Default ${s.sessionLength.value.default_idle_hours}. At most ${s.sessionLength.value.max_idle_hours}.`">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model.number="idleHours" type="number" inputmode="numeric" min="1" :max="s.sessionLength.value.max_idle_hours" :aria-describedby="describedBy" data-test="idle-hours" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 text-primary focus-visible:outline-none focus-visible:ring-2" />
              </template>
            </FormField>
            <FormField label="Always signed out after (days)" :hint="`Default ${s.sessionLength.value.default_absolute_days}. At most ${s.sessionLength.value.max_absolute_days}.`">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model.number="absoluteDays" type="number" inputmode="numeric" min="1" :max="s.sessionLength.value.max_absolute_days" :aria-describedby="describedBy" data-test="absolute-days" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 text-primary focus-visible:outline-none focus-visible:ring-2" />
              </template>
            </FormField>
          </div>
          <p v-if="lengthProblem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="session-length-error">{{ lengthProblem }}</p>
          <div class="flex flex-wrap items-center gap-3">
            <BaseButton type="submit" :disabled="!!lengthProblem" class-name="max-sm:!h-11">Save</BaseButton>
            <span v-if="lengthSaved" role="status" class="text-[length:var(--text-small)] text-state-success" data-test="session-length-done">Saved.</span>
          </div>
        </form>
      </Panel>

      <Panel title="Where you are signed in" preserve-case>
        <ul class="m-0 flex list-none flex-col gap-2 p-0" data-test="sessions">
          <li v-for="x in s.sessions.value" :key="x.id" :data-current="x.current" class="flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-2 first:border-t-0 first:pt-0">
            <span class="flex min-w-0 flex-col">
              <span class="break-words font-semibold text-primary">{{ x.browser || "Unknown browser" }}<template v-if="x.current"> · this one</template></span>
              <span class="break-words text-[length:var(--text-small)] text-muted">{{ x.address || "unknown address" }} · last used {{ formatAbsoluteTime(x.last_seen) }}</span>
            </span>
            <BaseButton v-if="!x.current" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="s.revokeSession(x.id)">Sign out</BaseButton>
          </li>
        </ul>
        <div class="mt-3"><BaseButton variant="danger" class-name="max-sm:!h-11" data-test="sign-out-everywhere" @click="endEverywhere">Sign out everywhere</BaseButton></div>
      </Panel>

      <Panel title="Script access" preserve-case>
        <div class="flex flex-col gap-3">
          <p class="m-0 text-[length:var(--text-small)] text-secondary">A script uses a token instead of your passphrase, so you can revoke one script without signing yourself out. “Admin” reaches the admin API; “Model proxy” reaches only <code class="font-mono">/v1</code>. A token cannot do the actions that need your passphrase.</p>

          <div v-if="s.created.value" role="status" class="flex flex-col gap-2 border border-accent-info p-3" data-test="new-token">
            <p class="m-0 font-semibold text-primary">Copy “{{ s.created.value.name }}” now. It is shown only once.</p>
            <code ref="tokenEl" class="select-all break-all font-mono text-[length:var(--text-small)] text-primary" data-test="token-value">{{ s.created.value.token }}</code>
            <div class="flex flex-wrap items-center gap-2">
              <BaseButton variant="secondary" class-name="max-sm:!h-11" @click="copyToken">{{ copied ? "Copied" : "Copy" }}</BaseButton>
              <span v-if="copyFailed" role="alert" class="text-[length:var(--text-small)] text-state-error" data-test="copy-failed">This page cannot copy for you. The token is selected: press Ctrl+C (Cmd+C on a Mac).</span>
              <BaseButton variant="ghost" class-name="max-sm:!h-11" @click="s.dismissCreated">I have copied it</BaseButton>
            </div>
          </div>

          <form class="grid max-w-xl grid-cols-1 items-end gap-3 sm:grid-cols-[1fr_auto_auto]" novalidate @submit.prevent="makeToken">
            <FormField label="Name">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="tokenName" type="text" :maxlength="TOKEN_NAME_MAX" :aria-describedby="describedBy" placeholder="eval scripts" class="min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 text-primary focus-visible:outline-none focus-visible:ring-2" />
              </template>
            </FormField>
            <FormField label="Can reach">
              <template #default="{ id }"><SelectInput :id="id" v-model="tokenScope" :options="TOKEN_SCOPE_OPTIONS" label="Can reach" /></template>
            </FormField>
            <BaseButton type="submit" :disabled="!tokenName.trim()" class-name="max-sm:!h-11">Make token</BaseButton>
          </form>

          <ul v-if="s.tokens.value.length" class="m-0 flex list-none flex-col gap-2 p-0" data-test="tokens">
            <li v-for="t in s.tokens.value" :key="t.id" class="flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-2">
              <span class="flex min-w-0 flex-col">
                <span class="break-words font-semibold text-primary">{{ t.name }} <span class="font-normal text-muted">· {{ t.scope === "admin" ? "Admin" : "Model proxy" }}</span></span>
                <span class="text-[length:var(--text-small)] text-muted">Made {{ formatAbsoluteTime(t.created_at) }} · {{ t.last_used ? `last used ${formatAbsoluteTime(t.last_used)}` : "never used" }}</span>
              </span>
              <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="revokeToken(t.id, t.name)">Revoke</BaseButton>
            </li>
          </ul>
          <p v-else class="m-0 text-[length:var(--text-small)] text-muted">No tokens yet.</p>
        </div>
      </Panel>

      <Panel title="Recent sign-ins" preserve-case>
        <ul v-if="s.events.value.length" class="m-0 flex list-none flex-col gap-1 p-0 font-mono text-[length:var(--text-small)]" data-test="events">
          <li v-for="(e, i) in [...s.events.value].reverse()" :key="i" class="break-words text-secondary">{{ formatAbsoluteTime(e.at) }} · {{ EVENT_LABELS[e.kind] ?? e.kind }}<template v-if="e.address"> · {{ e.address }}</template><template v-if="e.detail"> · {{ e.detail }}</template></li>
        </ul>
        <p v-else class="m-0 text-[length:var(--text-small)] text-muted">Nothing yet.</p>
      </Panel>
    </template>
  </div>
</template>
