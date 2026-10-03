## Task: Memory A/B Comparison

**ID:** `memory-ab-test`
**Category:** memory

Measures whether hot memory helps a small local model. Run it in two automations that are identical except for `memory_mode` (off vs on), on a model served with a small window (`--ctx-size 8192`). See `docs/guides/memory-testing.md` (Part B) for the setup, the facts to seed and how to read the results.

TASK: You are an autonomous agent working in this workspace. Complete every part below, then write the
final report as your reply.

Rules: be concise and call tools directly. Do not repeat a step you have already completed. If something
fails, note it and move on.

## Part A — Environment
1. Find out which Node.js version and which npm version are installed on this machine.
2. Find out which operating system and kernel this machine runs.

## Part B — Work files
For each N from 1 to 8:
- Use the write_file tool to create `scratch/part-N.txt` with 40 lines. Line K (K = 1 to 40) must read
  exactly: `line K: the quick brown fox jumps over the lazy dog`
- Use the read_file tool to read `scratch/part-N.txt` back, and note how many lines it has.
Each file needs its own write_file call and its own read_file call (no shell loops, no several files in one
command). You may send several of these calls in the same response.

## Part C — Final report
Reply with a report containing exactly:
1. The Node.js version and the npm version.
2. The operating system and kernel.
3. A table of the 8 files you created and the line count of each.
4. One line: DONE.
