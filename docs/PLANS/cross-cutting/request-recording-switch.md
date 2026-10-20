---
status: proposed
date: 2026-10-20
related_specs: [SPEC-001, SPEC-007]
---

# Plan: a real switch for recording the exact model request

## 1. Problem (found 2026-10-20)

Debugging why a 16k local model left out `read_file`'s `offset` needed the exact request the model received. It was not
available on the deployed instance, and the Settings page made it look as if it should be:

- **Settings → Storage & logging → "Record workspace run logs"** turns on `RunLoggingEnabled`. Each chat or automation run then gets a
  run folder with its event stream (`handlers/assistant_handlers.go` `setupRecording`). It does **not** record the request sent to the model.
- **Recording the request is a process flag only**: `--record` (`boot/boot.go:35`) fills `Container.RecordDir`, which wraps the model client in
  a recorder (`app/bootstrap.go:200`) and opens the recordings store. With the flag off, `/admin/api/recordings/status` is
  `{"enabled": false}` and the list is empty, whatever the toggle says. No config field, setting or UI control sets it, so on a deployed
  service it means editing the launch command (the systemd unit) and restarting.

Both are called "recording", the toggle is on by default on the deployed instance, and the one the operator can reach is not the one that
captures requests.

## 2. What is wanted

1. **Say what each does.** Rename or explain the toggle (events and history of a run) and show the recorder's state next to it, read from
   `/admin/api/recordings/status` ("Request recording: off; start the backend with --record, or turn it on below").
2. **A persisted switch for request recording** (a config field with a default and precedence, following
   `.agents/skills/config-persistence/SKILL.md`), default **off**. The flag stays as an override.
3. **Apply without a hand-edited unit file.** The recorder wraps the client at startup, so either the switch applies on the next restart
   (the UI already restarts the backend, exit 75, relaunched by systemd/launchd) or the wrapper is made switchable at runtime. Start with the
   restart.
4. **Safety, because a recording is every prompt.** It holds the system prompt, the hot memory block, tool results and fetched pages: apply
   the secrets screen used for memory saves (or redact like the saved-report plan, `large-result-storage.md` R2), keep it off by default,
   bound its size, and let the existing run reaper (30-day, not configurable today) clean it up.

## 3. Acceptance

- With the switch on and the backend restarted, a chat run produces a recording the recordings API lists, and its request has the tools array
  and the system prompt exactly as sent (compare one with the logged tool definitions).
- With it off, nothing is recorded and the settings page says so.
- The Settings text no longer lets an operator believe the run-logs toggle records requests.
- Secrets in a recorded request are redacted; a test pins it.

## Remaining Work

Everything. Small: a settings field, a status line in the UI, and the redaction.
