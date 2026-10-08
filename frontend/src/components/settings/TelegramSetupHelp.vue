<script setup lang="ts">
import ContextDrawer from "../layout/ContextDrawer.vue"
import Panel from "../common/layout/Panel.vue"
import Icon from "../icons/Icon.vue"

defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: "update:open", open: boolean): void }>()

const FOCUSABLE = 'button:not([disabled]), a[href]'
const MESSAGE_FLOWS = [
  { title: "Outgoing reports · bot token + Chat ID", steps: ["App report", "Bot sends", "Your chat"] },
  { title: "Incoming messages · optional webhook + workspace", steps: ["You message", "Webhook", "Workspace"] },
]

function keepFocusInside(event: KeyboardEvent) {
  const container = event.currentTarget as HTMLElement
  const controls = container.querySelectorAll<HTMLElement>(FOCUSABLE)
  const first = controls[0]
  const last = controls[controls.length - 1]
  if (!first || !last) return
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
</script>

<template>
  <div @keydown.tab="keepFocusInside">
    <ContextDrawer :open="open" title="Telegram setup help" wide @update:open="emit('update:open', $event)">
      <div class="flex min-w-0 flex-col gap-4 text-[length:var(--text-small)] text-muted [counter-reset:section]">
        <p class="m-0">Connect your bot to send reports to Telegram. Add an inbound webhook if you also want to message the agent from Telegram.</p>

        <figure class="m-0 rounded-[var(--radius-sm)] border border-hairline bg-surface p-4">
          <figcaption class="mb-3 font-mono text-secondary">Two values, two different jobs</figcaption>
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="min-w-0 rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3">
              <div class="mb-2 flex items-center gap-2 text-secondary"><Icon name="send" size="sm" /> Bot token</div>
              <code class="break-all font-mono text-primary"><span class="text-accent-brand">123456789</span>:<span class="text-secondary">example-token</span></code>
              <p class="m-0 mt-2">Bot ID + colon + secret. Copy all three parts into Bot token.</p>
            </div>
            <div class="min-w-0 rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3">
              <div class="mb-2 flex items-center gap-2 text-secondary"><Icon name="message" size="sm" /> Chat ID</div>
              <code class="font-mono text-primary">987654321</code>
              <p class="m-0 mt-2">Identifies the destination chat. Goes in Chat ID.</p>
            </div>
          </div>
          <p class="m-0 mt-3 text-faint">Illustrative values only. The number before the token's colon identifies the bot; obtain your destination Chat ID separately.</p>
        </figure>

        <Panel title="Get your bot token" preserve-case>
          <ol class="m-0 list-decimal space-y-2 pl-5">
            <li>Open the verified @BotFather account in Telegram and send <code class="font-mono text-secondary">/newbot</code>. Follow its prompts to create a bot.</li>
            <li>For an existing bot, use <code class="font-mono text-secondary">/mybots</code>, select the bot, then API Token.</li>
            <li>Copy the complete token, including the colon. Paste it into <strong class="text-secondary">Bot token</strong>. Treat it like a password.</li>
          </ol>
          <p class="m-0 mt-3"><a href="https://core.telegram.org/bots/features#botfather" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Source: Telegram's BotFather guide</a></p>
        </Panel>

        <Panel title="Find the destination Chat ID" preserve-case>
          <ol class="m-0 list-decimal space-y-2 pl-5">
            <li>For a private chat, open your bot, press <strong class="text-secondary">Start</strong> and send it a message.</li>
            <li>For a group, add your bot and send a command addressed to it, such as <code class="font-mono text-secondary">/start@your_bot_username</code>.</li>
            <li>Before registering a webhook, open the address below in your browser. Replace <code class="font-mono text-secondary">YOUR_BOT_TOKEN</code> with the full token.</li>
          </ol>
          <code class="mt-3 block break-all rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3 font-mono text-secondary">https://api.telegram.org/botYOUR_BOT_TOKEN/getUpdates</code>
          <p class="m-0 mt-3">Find the message you just sent in the response. Copy its <code class="font-mono text-secondary">message.chat.id</code>, including any minus sign, into <strong class="text-secondary">Chat ID</strong>.</p>
          <pre class="m-0 mt-3 overflow-x-auto rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3 font-mono text-primary" aria-label="Example update: copy chat id 987654321">{
  "message": {
    "chat": { <span class="text-accent-brand">"id": 987654321</span> },
    "text": "Hello"
  }
}</pre>
          <p class="m-0 mt-3">If the result is empty, send another message and refresh. getUpdates cannot be used while a webhook is registered. For an existing setup, reuse the saved Chat ID or read it from the incoming webhook payload.</p>
          <p class="m-0 mt-3"><a href="https://core.telegram.org/bots/api#getupdates" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Source: Telegram getUpdates</a> · <a href="https://core.telegram.org/bots/api#message" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Message fields</a></p>
        </Panel>

        <Panel title="Fill in the app settings and save" preserve-case>
          <dl class="m-0 space-y-3">
            <div><dt class="font-medium text-secondary">Connector name</dt><dd class="m-0 mt-1">Choose a label such as <code class="font-mono">my-telegram</code>. The app uses it to identify the connector and build the webhook URL. It does not need to match your bot's Telegram name.</dd></div>
            <div><dt class="font-medium text-secondary">Type, Chat ID and Bot token</dt><dd class="m-0 mt-1">Select Telegram and enter the two values above. Click Add connector (or Apply changes), then Save communication settings.</dd></div>
            <div><dt class="font-medium text-secondary">Allow the agent to send</dt><dd class="m-0 mt-1">In Settings · Agent Guardrails, enable Communication and save. To grant it just to one workspace, enable Communication in that workspace's Settings instead. Host network access must be on. Approve each notification controls whether sends pause for your approval.</dd></div>
            <div><dt class="font-medium text-secondary">Outgoing notifications only</dt><dd class="m-0 mt-1">Leave Workspace for inbound messages and Webhook secret token blank. A webhook is optional for sending reports.</dd></div>
          </dl>
          <p class="m-0 mt-3 text-faint">These steps describe this app's Communication settings.</p>
        </Panel>

        <figure class="m-0 rounded-[var(--radius-sm)] border border-hairline bg-surface p-4">
          <figcaption class="mb-3 font-mono text-secondary">How messages travel</figcaption>
          <div class="space-y-4">
            <div v-for="flow in MESSAGE_FLOWS" :key="flow.title">
              <p class="m-0 mb-2 text-secondary">{{ flow.title }}</p>
              <div class="grid grid-cols-3 gap-2 text-center font-mono text-secondary">
                <div v-for="step in flow.steps" :key="step" class="rounded-[var(--radius-sm)] border border-hairline bg-canvas p-2">{{ step }}</div>
              </div>
              <svg viewBox="0 0 300 16" class="mt-1 h-4 w-full text-accent-brand" fill="none" stroke="currentColor" aria-hidden="true"><path d="M50 8h95m-6-5 6 5-6 5m16-5h95m-6-5 6 5-6 5" /></svg>
            </div>
          </div>
        </figure>

        <Panel title="Optional: receive messages through a webhook" preserve-case>
          <ol class="m-0 list-decimal space-y-2 pl-5">
            <li>Enter an existing <strong class="text-secondary">Workspace for inbound messages</strong>. Its ID is the part of that workspace's app URL immediately after <code class="font-mono">/workspaces/</code>.</li>
            <li>Optionally choose a separate <strong class="text-secondary">Webhook secret token</strong>: 1–256 letters, digits, underscores or hyphens. Telegram includes this secret in its requests so the app can check them.</li>
            <li>Apply your changes and save. In the <strong class="text-secondary">Inbound webhook</strong> panel, enter a Public host: a public HTTPS host or base URL that reaches this server. The app adds <code class="break-all font-mono">/api/v1/webhooks/your-connector-name</code>.</li>
            <li>Click <strong class="text-secondary">Register</strong>, then <strong class="text-secondary">Verify</strong>. Register replaces any webhook already registered for this bot. Send your bot a message to check that it reaches the selected workspace.</li>
          </ol>
          <p class="m-0 mt-3">Telegram cannot reach localhost or a private LAN address. A public reverse proxy or tunnel must route the webhook to this server.</p>
          <p class="m-0 mt-3"><a href="https://core.telegram.org/bots/api#setwebhook" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Source: Telegram webhook requirements and secret tokens</a></p>
        </Panel>

        <Panel title="If it does not work" preserve-case>
          <ul class="m-0 list-disc space-y-2 pl-5">
            <li><strong class="text-secondary">Unauthorized:</strong> check that Bot token contains the entire token from BotFather.</li>
            <li><strong class="text-secondary">Chat not found:</strong> check the Chat ID, press Start in your private bot chat, or confirm the bot belongs to the destination group.</li>
            <li><strong class="text-secondary">Webhook delivery error:</strong> use Verify to read Telegram's error. Check the public HTTPS host and that it routes to this server.</li>
          </ul>
          <p class="m-0 mt-3"><a href="https://core.telegram.org/bots/tutorial#sending-messages" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Source: Telegram's sending messages guide</a></p>
        </Panel>
        <p class="m-0 text-faint">Telegram guidance checked against its official documentation on 3 October 2026. Examples are illustrative and never use your saved credentials.</p>
      </div>
    </ContextDrawer>
  </div>
</template>
