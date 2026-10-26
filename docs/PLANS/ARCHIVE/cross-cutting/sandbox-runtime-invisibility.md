---
status: complete
date: 2026-08-25
related_specs: [SPEC-006]
constitution_references: [II.3]
---

# Sandbox Runtime Invisibility (`.sandbox` must never reach the agent)

## Problem

The sandbox runtime directory (`.sandbox`, the shell provider's HOME layout under the
workspace root) is an internal invariant agents must not browse. It was enforced only by a
terminal input-side gate (`checkBlockedPaths` on a hardcoded `sandboxRuntimeDirs` list). Two
leaks bypassed that invariant:

1. **Filesystem listing** — `FileSystemTools.ListDirectory` filtered configured
   `BlockedFilenames` but not the internal `.sandbox` path, so `list_directory` on the
   workspace root returned `.sandbox/` as a visible entry.
2. **Terminal output** — the input gate only rejects *explicit* `.sandbox` operands
   (`find .sandbox`, `du -sh .sandbox`). Recursive traversal (`find .`, `du -sh .`,
   `ls -la`, `tree`) emits `.sandbox` paths in its **output** with no output-side filter.

Observed with a cloud model (OpenRouter `ox-alpha`): the assistant run's `find . -path
./node_modules -prune -o -type f -print` returned `./.sandbox/.npm/_cacache/...` paths, and
`list_directory` surfaced `.sandbox/` in the workspace listing. Any model running a recursive
command would hit the same gap — it is not model-specific.

## Fix (single source of truth, no per-tool code)

- `tools/security.go` — new `internalBlockedPaths` list (the home of the shared
  `blockedFilename` / `blockedPathEntry` matchers) and `effectiveBlockedFilenames(user)`
  which merges user-configured blocked filenames with the internal invariant paths.
- `filesystem.go` — `validateFilePath` and `ListDirectory` now enforce
  `effectiveBlockedFilenames(cfg.BlockedFilenames)` through the existing `blockedFilename`
  helper: `.sandbox` reads are explicitly blocked and listings hide it.
- `terminal.go` — `checkBlockedPaths` uses the merged list (replacing the bespoke
  `sandboxRuntimeDirs` branch); new generic `redactBlockedPaths(output, blocked)` scrubs
  output lines referencing any blocked path segment, applied in both `executeShell`
  (shell-pool path) and `executeLocal`. Output redaction passes the internal invariant
  list (the terminal tool has no user `BlockedFilenames` at execution time — those are
  enforced input-side per call); the function is generic, so threading the merged list
  into output later is a one-line change.

Adding a future internal invariant path (e.g. another runtime dir) is one entry in
`internalBlockedPaths` — filesystem validation, listings, terminal input, and terminal
output all pick it up through the shared helpers.

`read_file` / `list_directory` *into* `.sandbox` were already blocked by the filesystem
tool's hidden-path rule; the search tool is internet-only. No frontend change needed.

## Tests

- `TestFileSystemTools_ListDirectory_HidesSandboxRuntime` — listing must omit `.sandbox`
  and keep ordinary entries.
- `TestRedactBlockedPaths` — find/du/ls/deep-path output scrubbed; ordinary and empty
  output unchanged; user blocked filenames and internal paths both redact via the merged
  list, and the internal path is not caught by the user list alone.
