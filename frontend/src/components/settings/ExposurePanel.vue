<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { AuthService } from "../../services/auth/authService"
import { errorMessage } from "../../utils/errors"
import type { AuthExposure } from "../../types/auth"

// Settings · Sign-in · Exposure: where the server listens and what protects it, with each item that needs attention saying
// what to do. It holds no secret, and it reads only what the server reports.
const exposure = ref<AuthExposure | null>(null)
const error = ref("")

onMounted(async () => {
  try {
    exposure.value = await AuthService.exposure()
  } catch (err) {
    error.value = errorMessage(err, "The exposure summary could not be read.")
  }
})

const yesNo = (on: boolean, yes: string, no: string) => (on ? yes : no)
</script>

<template>
  <Panel title="Exposure" preserve-case data-test="exposure">
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="exposure-error">{{ error }}</p>
    <div v-else-if="exposure" class="flex flex-col gap-4">
      <div v-if="exposure.attention.length" class="flex flex-col gap-2" data-test="exposure-attention">
        <Callout v-for="a in exposure.attention" :key="a.id" tone="warning" :title="a.text">{{ a.fix }}</Callout>
      </div>
      <p v-else class="m-0 text-[length:var(--text-small)] text-secondary" data-test="exposure-ok">Nothing needs attention.</p>

      <dl class="m-0 grid grid-cols-[max-content_minmax(0,1fr)] items-baseline gap-x-4 gap-y-2" data-test="exposure-facts">
        <dt><MicroLabel>Listening on</MicroLabel></dt>
        <dd class="m-0 break-words"><span class="font-mono">{{ exposure.listening.address }}</span> · {{ yesNo(exposure.listening.network, "other machines can connect", "this machine only") }}</dd>
        <dt><MicroLabel>Sign-in</MicroLabel></dt>
        <dd class="m-0">{{ yesNo(exposure.sign_in_required, "Required", "Not set up") }}</dd>
        <dt><MicroLabel>Connection</MicroLabel></dt>
        <dd class="m-0">{{ yesNo(exposure.encrypted, "Encrypted (HTTPS)", "Not encrypted") }}</dd>
        <dt><MicroLabel>Model proxy key</MicroLabel></dt>
        <dd class="m-0">{{ yesNo(exposure.inference_key_required, "Required for /v1", "Not required: /v1 is open") }}</dd>
        <dt><MicroLabel>Webhook secrets</MicroLabel></dt>
        <dd class="m-0">{{ exposure.webhooks.inbound ? `${exposure.webhooks.with_secret} of ${exposure.webhooks.inbound} connectors that receive messages` : "No connector receives messages" }}</dd>
        <dt><MicroLabel>Agent sandbox</MicroLabel></dt>
        <dd class="m-0">{{ exposure.sandbox.enabled ? `On · files: ${exposure.sandbox.filesystem}` : "Off" }}</dd>
      </dl>
    </div>
  </Panel>
</template>
