# Assistant UI Chat Architecture — Webhook Sessions

Reference for the [`assistant-ui-chat`](../SKILL.md) skill, loaded on demand. You are changing how connector/webhook sessions show in the chat UI.

## Webhook Sessions

Webhook-triggered agent runs (Telegram, etc.) bypass the frontend's `sendMessage()` flow. Events arrive via SSE as normal, but there's a critical difference:

- **`sendMessage()`** pushes `{ role: 'user', content: text }` to `messages.value` at line 168. The builder finds this user message when processing subsequent events.
- **Webhook sessions** have NO user message in `messages.value` because `sendMessage()` was never called.

Without a user-role message, `groupTurns()` at line 28 (`m.role !== 'user'`) skips all messages, producing zero turns. The assistant bubble never renders.

**Fix:** In `applySessionUpdate()`, when receiving a `session_started` lifecycle event:
1. Check `!loading.value` — avoid pushing during active manual `sendMessage()`
2. Check `p.snippet` is present (contains the user message text)
3. Check for duplicates — don't push if `messages.value` already has the same text
4. Push `{ role: 'user', content: p.snippet }` to `messages.value`

This makes the user message available for `groupTurns()` to anchor the turn, and subsequent SSE events flow through the normal builder pipeline.
