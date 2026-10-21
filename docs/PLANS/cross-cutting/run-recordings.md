---
status: complete
date: 2026-10-20
related_specs: [SPEC-001, SPEC-007]
---

# Plan: make run recordings readable and safe

Supersedes the premise of the proposed "request recording switch" (PR #103, closed): a new switch was not needed.

## 1. What was found (2026-10-20)

Debugging a live failure needed the exact request a 16k model received. The Settings toggle "Record workspace run logs" was
on, `/admin/api/recordings/status` said `enabled: false` and listed nothing, and `--record` is a process flag with no setting
or UI control. Reading the code showed the picture was different from what the page implied:

- The model client is **always** wrapped in a recorder (`app/bootstrap.go` `newClientFactory`), and `setupRecording` gives each
  run's folder to it, so with run logging on every run already wrote `recording.jsonl` with every request and reply, with
  `--record` off. (`docs/data-layout.md` said it needed `--record`; it did not.)
- Nothing could read them: the recordings store existed only with `--record`, and even then the API returned a recording's
  metadata (path, size), never its contents.
- They were written **unmasked**: the system prompt, hot memory, tool results, fetched pages and anything pasted, for 30 days.

## 2. What was built

1. **Masking on write.** `tools.RedactForRecording` (token shapes, headers, JWTs, private-key blocks, URL passwords, credential
   assignments; a cheap hint check first, after Hermes' `agent/redact.py`) injected into the recorder (`recorder.WithRedactor`,
   so `recorder` does not import `tools`). The caller's messages are never modified. Chunk lines are masked per chunk.
2. **The store covers `runs/`** whenever run recordings exist; `AppServices.RecordDir()` and the status `enabled` keep meaning the
   `--record` flag, so the Recordings page and replay references do not change. Status adds `run_recordings`.
3. **`GET /admin/api/recordings/content/{id}`** with `type`, `last` and `max_bytes`: `?type=request&last=1` is the last request,
   bounded and flagged when cut. Every string is masked as it is served (`recordings.RedactJSONLines`), so recordings written before
   masking existed are not readable raw. Masking walks the JSON, so key order (the order a tool lists its arguments) and number text are kept and the line is otherwise as it was written. The store lists `recording.jsonl` files and not `events.jsonl`.
4. **The Settings text** says what the toggle records, that secrets are masked and the 30-day retention.

## 3. Acceptance

- A run with run logging on and `--record` off appears in `/admin/api/recordings`, and its last request has the tools array and
  system prompt as sent (compare with the logged tool definitions).
- A chat message containing a key-shaped string shows `[REDACTED]` in the recording and the model still receives it.
- With run logging off nothing new is recorded.

## Remaining Work

- Existing recordings on disk were written unmasked and age out with the 30-day run reaper (not configurable today). The content endpoint masks every string it serves, so they are not readable raw through the API, but the files themselves stay raw until the reaper removes them or they are deleted.
- A secret split across two streamed chunks can survive the per-chunk masking.
- The content endpoint is as open as the rest of the admin API (see the admin-authentication plan).
