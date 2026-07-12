---
status: reference
last_reviewed: 2026-07-11
---

# llama.cpp Server — Setup, Args & Tuning

**Source docs:** `docs/llama_cpp_setup`, `docs/services/llm-proxy.service`, `docs/audits/memory-injection-investigation.md`

---

## Build

```bash
git clone https://github.com/ggml-org/llama.cpp
cd llama.cpp
cmake -B build -DGGML_VULKAN=ON
cmake --build build --config Release -j
```

For AMD Radeon 780M (integrated GPU), use Vulkan backend. Check available GPUs:
```bash
vulkaninfo --summary
```

## Recommended Server Args

```bash
--ctx-size 8192
--parallel 1
--n-gpu-layers 999
--flash-attn on
--threads 8
--batch-size 2048
--ubatch-size 512
--temp 1.0
--top-p 0.95
--top-k 64
--repeat-penalty 1.12
--repeat-last-n 256
--frequency-penalty 0.5
--presence-penalty 0.5
```

**Key points:**
- `--repeat-penalty 1.0` (default) disables repetition prevention — must set to 1.12+
- `--frequency-penalty` and `--presence-penalty` add extra guardrails against loops
- `--temp` is overridden by our API calls (automation sends 0.1 or model override) — server value is only the default
- `--ctx-size 8192` is small for Gemma 4 (native is 131072) — increase if you have the VRAM
- `--flash-attn on` enables flash attention for memory efficiency

## Performance on AMD Radeon 780M (Gemma 4 4B)

| Metric | Value |
|--------|-------|
| Prompt processing (cold) | ~500-650 tokens/s |
| Prompt processing (warm) | ~200-350 tokens/s |
| Generation | ~20-22 tokens/s |
| Reasoning budget per turn | 910 tokens (max_tokens / 3) |
| First-prompt eval | 6-7s (cold) → 0.3-0.6s (warm) |

## Provider vs Model Args

- **Model launch args** (the `Custom Arguments` field in the UI) — passed to llama.cpp CLI. Only applies to locally launched models.
- **Provider params** (temperature, max_tokens, reasoning_budget) — sent in the API request body to the LLM server. These override the server's defaults.
- When connecting to a remote llama.cpp server via OpenAI-compatible API, only the API request params reach the server. The Custom Arguments field is meaningless for remote models.

## systemd Service

```ini
[Unit]
Description=LLM Proxy
After=network.target

[Service]
Type=simple
User=mikeathan
WorkingDirectory=/home/mikeathan/dev/llm-proxy
ExecStart=/home/mikeathan/dev/llm-proxy/llm-proxy --data /home/mikeathan/dev/llm-proxy/data
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full
```

## Important Gotchas

- The server's `--temp` is ignored when our code sends `temperature` in the API request. Server-level penalty flags (`--repeat-penalty`, etc.) still apply because our `ChatRequest` has no fields for them.
- `--ctx-size 8192` → our `context_budget` of ~10924 chars (~0.75 chars/token ratio). If you increase ctx-size, adjust context_budget in the model config.
- The GGUF file's built-in chat template is used. If it's outdated, llama.cpp warns `detected an outdated gemma4 chat template, applying compatibility workarounds`. Update the GGUF if possible.
