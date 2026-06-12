## Task: LLM Smoke Test — Multi-Tool Coverage

**ID:** `llm-smoke-test`
**Category:** system
TASK: You are an autonomous agent. Complete ALL steps below in order. Do not skip any step. Submit a final summary when done.

CRITICAL RULES:

- BE CONCISE. Do not narrate your plan — output the tool call directly.
- Each step is independent. If a step fails or is blocked by a guardrail, NOTE the failure and MOVE ON to the next step.
- NEVER restart from Step 1. You cannot change past results. Keep moving forward.
- If a directory or package already exists, skip that sub-step and move on.

# LLM Smoke Test — Multi-Tool Coverage

This test exercises every supported tool category: filesystem, terminal, dev workflow, and networking.

## Step 1 — Filesystem: List

- List the contents of the current directory. Note what files and directories exist.

## Step 2 — Filesystem: Write & Verify

- Write a file named `smoke-test-output.txt` with the content:
  ```
  Smoke test executed.
  Agent workspace operational.
  ```
- Read back `smoke-test-output.txt` to confirm the content was written correctly.

## Step 3 — Terminal: System Commands

Run each of these commands and capture the output:

- `uname -a`
- `date -u +%Y-%m-%dT%H:%M:%SZ`
- `echo "terminal-tool-works"`

## Step 4 — Terminal: File Operations

- Create a directory `smoke-test-dir` (ignore "File exists" errors).
- Create a file `smoke-test-dir/hello.txt` with content:
  ```
  #!/bin/sh
  echo "hello from smoke test"
  ```
- Make it executable with `chmod +x smoke-test-dir/hello.txt` and run it with `sh smoke-test-dir/hello.txt`. Capture the output.

## Step 5 — Dev Work: Package Management

- Run `npm init -y` in the current directory (ignore "already exists" errors).
- Run `npm install --save-dev typescript` (skip if already installed).
- Verify: run `npx tsc --version` and capture the output.

## Step 6 — Dev Work: Write & Compile Code

- Create a directory `dev-test` with a file `index.ts` containing:

  ```ts
  interface HealthCheck {
    service: string;
    status: "ok" | "degraded" | "down";
    uptime: number;
  }

  const report: HealthCheck = {
    service: "llm-proxy",
    status: "ok",
    uptime: Date.now(),
  };

  console.log(JSON.stringify(report, null, 2));
  ```

- Compile it: `npx tsc dev-test/index.ts --outDir dev-test --target ES2020 --module commonjs --strict --esModuleInterop`
- Run: `node dev-test/index.js` and capture the JSON result.
- Confirm the JSON has `"status": "ok"`.

## Step 7 — Dev Work: Edit & Iterate

- Edit `dev-test/index.ts`: change `status` from `"ok"` to `"degraded"` and add a `load` field (number, set to `0.87`).
- Re-compile and re-run. Capture the new output.
- Confirm the JSON now shows `"status": "degraded"` and `"load": 0.87`.

## Step 8 — Network: Basic Fetch

- Fetch `https://httpbin.org/headers`. Note the HTTP status and the `Content-Type` header. (Headers only, not the full body.)

## Step 9 — Network: Local Info

- Run `get_network_info`. Capture the hostname and the first non-loopback IP address.

## Step 10 — Final Submission

Submit a structured Markdown report with these sections:

1. **Filesystem** — What was listed, write/read verification result.
2. **Terminal** — Output from the three system commands, `hello.txt` execution result.
3. **Dev Work** — `tsc` version, initial `index.ts` output JSON, edited output JSON confirming `"degraded"` and `"load": 0.87`.
4. **Network** — HTTP status and Content-Type from httpbin, hostname and IP from `get_network_info`.
5. **Summary** — One sentence: "All X categories passed" or "Y categories passed, Z had issues".
