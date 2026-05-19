# Plan: Auto-Detect Native Tool Format & Jinja Template from GGUF

## Current State

The codebase already scans GGUF files on model discovery (`gguf_scanner.go:28-30`) using `gguf-parser-go v0.24.0`. However, it only extracts 7 basic fields:

| Extracted | Used For |
|-----------|----------|
| Name, Architecture, ContextLength, Parameters, Quantization, Author, Description | UI display + budget computation |

**What's missing:**
- `tokenizer.chat_template.jinja` — the Jinja template string (llama.cpp needs this for native tool calls)
- `tokenizer.ggml.chat_template` — alternative chat template key
- `tokenizer.ggml.pre` — tokenizer preprocessing config (e.g., "llama3", "mistral")
- `tokenizer.ggml.tokens` — tokenizer vocabulary used for template rendering

Currently, local models are **hardcoded to XML mode** (`LocalToolRegistry.UseNativeTools()` returns `false` at `registry.go:198`). The user must manually set `ToolCallFormat: "native"` in the model config — and if they do, llama.cpp still needs the correct Jinja template to render tool calls properly.

### The GGUF Metadata We Can Extract

llama.cpp embeds these keys in GGUF files (verified by `llama-gguf-split --info`):

| GGUF Key | Type | Purpose |
|----------|------|---------|
| `tokenizer.chat_template` | string | Full Jinja template for chat |
| `tokenizer.ggml.chat_template` | string | Alternative (older GGUF) |
| `tokenizer.ggml.pre` | string | Preprocessing config (e.g., "llama3") |
| `tokenizer.ggml.tokens` | array[string] | Tokenizer vocabulary |
| `tokenizer.ggml.add_bos` | bool | Auto-add BOS token |
| `tokenizer.ggml.add_eos` | bool | Auto-add EOS token |
| `tokenizer.chat_template.jinja` | string | Jinja-specific template |

The `gguf-parser-go` library exposes raw GGUF metadata — we just need to read these extra keys.

### Architecture of the Fix

```
GGUF Scanner (gguf_scanner.go)
    └── Extract chat template + tokenizer keys into ModelMetadata
          └── New fields: ChatTemplate, TokenizerPre
  
ModelConfig.ToolCallFormat (registry.json)
    └── Auto-set to "native" if:
        1. Model architecture supports function calling (gemma, llama3.1+, qwen2.5+, mistral-large, etc.)
        2. Chat template is present in GGUF metadata
    └── Keep "xml" default if either is missing
  
llama-server Launch (local_provider.go)
    └── If ChatTemplate is present, pass it via --chat-template flag to llama-server
    └── This is how llama.cpp gets the Jinja template at runtime
  
Agent Loop (agent.go)
    └── Already respects ToolCallFormat from ModelConfig
    └── Already passes useNativeTools flag correctly
```

### Implementation Steps

#### 1. Extend `ModelMetadata` struct (`models/config.go`)

Add two new fields:
```go
ChatTemplate   string `json:"chat_template,omitempty"`    // Jinja chat template from GGUF
TokenizerPre   string `json:"tokenizer_pre,omitempty"`    // tokenizer.ggml.pre value
```

#### 2. Update GGUF Scanner (`internal/core/llm/metadata/gguf_scanner.go`)

After parsing, extract the additional keys from the raw GGUF metadata. The `gguf-parser-go` library returns a struct with the parsed header — check if `gm.ChatTemplate` or `gm.RawMetadata` contains the Jinja template. If the library doesn't expose these directly, we may need to read raw GGUF string metadata via the library's raw accessor.

This is the critical step — need to verify what `gguf-parser-go v0.24.0` exposes. If it only exposes the named fields (Name, Architecture, etc.), we'll need to either:
- Use the library's raw metadata accessor if available
- Fall back to running `llama-gguf-split --info` as a subprocess and grep the output
- Add a small GGUF raw reader (GGUF format is simple: magic + version + key-value pairs)

#### 3. Auto-detect tool call format (`internal/core/orchestrator/budget_squeezer.go`)

Add a function `DetectToolCallFormat(cfg *ModelConfig) string` that returns:
- `"native"` if: architecture is in a known function-calling-capable list AND ChatTemplate is present
- `""` (empty, let existing logic handle it) otherwise

Known function-calling architectures (from llama.cpp docs):
- `gemma`, `gemma2` (gemma2+)
- `llama` (llama 3.1+)
- `qwen2` (qwen2.5+)
- `mistral` (mistral-large)
- `phi3` (phi-3.5)
- `mamba` (no — doesn't support function calling)
- `starcoder` (no)

#### 4. Pass chat template to llama-server (`internal/core/llm/providers/local_provider.go`)

When starting a local model with `ToolCallFormat: "native"` and `ChatTemplate` present, add `--chat-template "{{ .ChatTemplate }}"` to the llama-server launch args. This is how llama.cpp receives the Jinja template at runtime.

#### 5. Update frontend defaults (`frontend/src/utils/modelUtils.ts`)

Change the local model default from `tool_call_format: "xml"` to `tool_call_format: ""` (empty = auto-detect), since the backend will now set it based on GGUF metadata.

### Risks & Edge Cases

1. **Some GGUF files don't have chat templates** — Many community-converted GGUFs strip the template. We must fall back to XML if no template is found.

2. **gguf-parser-go may not expose chat template** — If the library doesn't surface these keys, we'll need a fallback strategy (subprocess or raw reader).

3. **llama-server `--chat-template` flag** — Need to verify the exact flag name. llama.cpp uses `--chat-template` or `--conversation` depending on version.

4. **Backward compatibility** — Existing models without `ChatTemplate` in metadata should continue working with XML format (existing behavior).

### What This Solves

- User adds a GGUF → metadata is scanned → if function-calling-capable + has Jinja template → `ToolCallFormat` auto-set to `"native"` → llama-server gets the template → agent loop uses native tool calls → no more wrong settings

### What This Doesn't Solve

- Models that have function calling but don't embed a Jinja template in GGUF (rare but possible)
- Cloud models (they already use native format via API)
- Manual override still works (user can always set `ToolCallFormat` manually)
