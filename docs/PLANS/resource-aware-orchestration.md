# Resource-Aware Orchestration Layer: Detailed Implementation Plan

## 1. Codebase Audit Findings

### 1.1 Request Flow (The Critical Path)

The actual LLM request path is NOT through `models.Provider.Generate()` (which is unimplemented for all providers). Instead:

```
agent.Execute() (agent.go:108)
  └─ agent.computeNextResponse() (agent.go:448)
       └─ a.client.Stream(ctx, req) (agent.go:486)  ← proxy.Client interface
            └─ LLMClient.Stream() (client.go:102)     ← raw HTTP to /chat/completions
                 └─ processStream() (agent.go:535)     ← SSE chunk reader
```

**Pre-flight injection point**: `agent.go:486` — immediately before `a.client.Stream(ctx, req)`.

**Stream interception point**: `agent.go:535-581` (`processStream`) — every SSE chunk passes through here. Current per-chunk latency is a single `select` + string append (~microseconds). Adding a token counter here costs <1µs per chunk.

**Reactive termination point**: `agent.go:109` (`execCtx`) and `agent.go:134` (`turnCtx`) — cancel the context to abort the stream. The existing goroutine at `client.go:139-142` already closes the response body on context cancel.

### 1.2 Configuration Gap Analysis

| Field | Where It Lives Now | Gap |
|---|---|---|
| `MaxSteps` | `ModelConfig` (registry) + `ModelOverride` (settings) | Present |
| `ContextBudget` | `ModelConfig` + `ModelOverride` | Present |
| `MaxTokens` | `AgentOptions` only (not persistable per-model) | **Missing from persistence** |
| `ToolCallFormat` | `ModelConfig` + `ModelOverride` | Present |
| `Prefill` | `ModelConfig` + `ModelOverride` | Present |
| `ReasoningBudget` | N/A | **Needs to be added** |
| `SlotTimeout` | N/A | **Needs to be added** |
| `ICUWeight` (Internal Credit Unit) | N/A | **Needs to be added** |

### 1.3 Reasoning Content Status

`agent.go:550-553` already accumulates `ReasoningContent` from SSE deltas, but merges it into `fullMsg.Content`. No provider-specific reasoning parsing exists. The `Message` struct (`models/llm_messages.go:21-27`) has a `ReasoningContent` field ready for use.

---

## 2. Technical Design Specification

### 2.1 New Package: `internal/core/orchestrator/`

```
internal/core/orchestrator/
├── orchestrator.go          # Main Orchestrator struct, wires BudgetManager + StorageEngine
├── budget_manager.go        # BudgetManager — pre-flight checks, ICU ledger
├── budget_squeezer.go       # DynamicGovernor — adaptive squeezing logic
├── stream_interceptor.go    # StreamInterceptor — counts tokens mid-stream, triggers termination
├── reasoning_normalizer.go  # ReasoningNormalizer — unifies Anthropic/OpenAI/Gemini thinking
├── slot_manager.go          # SlotManager — tracks llama.cpp slot IDs in SQLite
├── storage_engine.go        # StorageEngine — SQLite (WAL mode) with generic EntityMetadata
├── icu_ledger.go            # ICULedger — Internal Credit Unit transaction log
└── provider_weights.go      # ProviderNormalizationTable — ICU weights per model
```

### 2.2 Go Struct Definitions

```go
// models/config.go — additions to ModelConfig

type ModelConfig struct {
    // ... existing fields ...

    // Resource-aware orchestration (NEW)
    ReasoningBudget int `json:"reasoning_budget,omitempty"` // max thinking tokens (0 = provider default)
    SlotTimeout     int `json:"slot_timeout,omitempty"`     // seconds to keep slot alive (0 = no persistence)
}

// models/config.go — additions to ProviderConfig

type ProviderConfig struct {
    // ... existing fields ...

    InternalCreditWeight float64 `json:"internal_credit_weight,omitempty"` // ICU multiplier
}

// models/infrastructure.go — addition to ModelOverride

type ModelOverride struct {
    // ... existing fields ...

    ReasoningBudget int     `json:"reasoning_budget,omitempty" yaml:"reasoning_budget,omitempty"`
    SlotTimeout     int     `json:"slot_timeout,omitempty" yaml:"slot_timeout,omitempty"`
    ICUWeight       float64 `json:"icu_weight,omitempty" yaml:"icu_weight,omitempty"`
}

// internal/core/orchestrator/orchestrator.go

type Orchestrator struct {
    budget         *BudgetManager
    storage        *StorageEngine
    interceptor    *StreamInterceptor
    normalizer     *ReasoningNormalizer
    slotMgr        *SlotManager
    weightsTable   *ProviderNormalizationTable
}

type OrchestratorConfig struct {
    DefaultReasoningBudget  int             // fallback when model has no explicit budget
    DefaultSlotTimeout      int             // global default for local models
    GlobalICUCap            int64           // hard ceiling per workspace per day
    SQLitePath              string          // path to orchestrator.db
}
```

### 2.3 BudgetManager

```go
// internal/core/orchestrator/budget_manager.go

type BudgetManager struct {
    storage *StorageEngine
    mu      sync.RWMutex
    caps    map[string]BudgetCap // keyed by workspace_id
}

type BudgetCap struct {
    MaxICU      int64 // maximum Internal Credit Units
    CurrentICU  int64 // consumed so far (reset daily/weekly)
    ResetWindow time.Duration
    LastReset   time.Time
}

type PreFlightResult struct {
    Allowed       bool
    AllocatedICU  int64
    RemainingICU  int64
    SqueezeFactor float64  // 0.0–1.0, applied when near cap
    TransactionID string   // opaque ID for correlating refunds
    Reason         string
}

// PreFlightCheck evaluates if the request can proceed within budget.
// On success, debits ICU immediately and returns a TransactionID.
// Caller MUST call Refund() via defer if the downstream request fails.
// Call immediately before a.client.Stream() in agent.go:computeNextResponse().
func (b *BudgetManager) PreFlightCheck(ctx context.Context, workspaceID string, req PreFlightRequest) (PreFlightResult, error)

// Refund credits ICU back to the workspace balance for a previously allocated
// (but unconsumed) transaction.  Idempotent — double-refund is a no-op.
func (b *BudgetManager) Refund(ctx context.Context, transactionID string) error

type PreFlightRequest struct {
    ModelName       string
    ProviderType    string   // "local", "openai", "gemini", etc.
    ContextChars    int      // current history size in characters
    MaxTokens       int      // requested max_tokens
    ReasoningBudget int      // requested reasoning budget
}
```

### 2.4 StorageEngine (SQLite)

```go
// internal/core/orchestrator/storage_engine.go

type StorageEngine struct {
    db *sql.DB // WAL mode SQLite
}

func NewStorageEngine(path string) (*StorageEngine, error)

// Generic entity metadata — future-proof for Memory/Knowledge modules
func (s *StorageEngine) SetEntityMetadata(ctx context.Context, entityType, entityID, key string, value []byte) error
func (s *StorageEngine) GetEntityMetadata(ctx context.Context, entityType, entityID, key string) ([]byte, error)
func (s *StorageEngine) DeleteEntityMetadata(ctx context.Context, entityType, entityID, key string) error
func (s *StorageEngine) ListEntityMetadata(ctx context.Context, entityType, entityID string) (map[string][]byte, error)

// ICU Ledger
func (s *StorageEngine) RecordTransaction(ctx context.Context, tx ICUTransaction) error
func (s *StorageEngine) GetBalance(ctx context.Context, workspaceID string, window time.Time) (int64, error)
func (s *StorageEngine) ResetWindow(ctx context.Context, workspaceID string) error

// Slot persistence
func (s *StorageEngine) SaveSlot(ctx context.Context, slot SlotRecord) error
func (s *StorageEngine) GetActiveSlot(ctx context.Context, modelName string) (*SlotRecord, error)
func (s *StorageEngine) ExpireSlots(ctx context.Context, olderThan time.Time) error
```

### 2.5 StreamInterceptor

```go
// internal/core/orchestrator/stream_interceptor.go

type StreamInterceptor struct {
    budget       *BudgetManager
    normalizer   *ReasoningNormalizer
    inCodeBlock  bool           // toggled by triple-backtick detection
    recentChars  string         // rolling window for backtick scanning (max 200 chars)
}

type StreamInterceptResult struct {
    ShouldTerminate bool
    Reason          string
    TokensUsed      int
    ReasoningUsed   int
}

// InterceptChunk is called once per SSE chunk in processStream().
// Uses code-aware token estimation:
//   - Prose (default):          tokenRatio = 0.5 chars/token
//   - Code (inside ``` blocks):  tokenRatio = 1.0 chars/token
// The code-sniff detector toggles inCodeBlock when it encounters triple
// backticks in the rolling 200-char window, preventing premature termination
// of code-heavy tasks (e.g. write_file with large source files).
// Target: <5µs per call — just integer math + comparison + substring check.
func (s *StreamInterceptor) InterceptChunk(ctx context.Context, chunk StreamChunk) StreamInterceptResult

type StreamChunk struct {
    Content          string // regular text delta
    ReasoningContent string // thinking delta (Anthropic/OpenAI/Gemini)
    ProviderType     string
    ReasoningBudget  int
    MaxTokens        int
}
```

---

## 3. SQL DDL (Schema) + Connection String

```go
// DSN (Data Source Name) — passed to sql.Open("sqlite3", dsn)
dsn := sqlitePath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON"
```

```sql
-- orchestrator.db — WAL mode, stored at {data_dir}/orchestrator.db
-- PRAGMAs are set via DSN (above), not SQL statements.

-- Active Slot Tracking (llama.cpp context persistence)
CREATE TABLE IF NOT EXISTS active_slots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    model_name      TEXT NOT NULL,
    slot_id         INTEGER NOT NULL,          -- llama.cpp slot ID
    host            TEXT NOT NULL,             -- e.g., 127.0.0.1
    port            INTEGER NOT NULL,          -- e.g., 8081
    cache_key       TEXT NOT NULL,             -- SHA256(system_prompt + first_user_msg + temperature + top_p + presence_penalty)
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    last_used_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at      DATETIME NOT NULL,         -- slot_timeout from model config
    UNIQUE(model_name, host, port)
);

CREATE INDEX IF NOT EXISTS idx_slots_model ON active_slots(model_name);
CREATE INDEX IF NOT EXISTS idx_slots_expiry ON active_slots(expires_at);

-- ICU Ledger (Internal Credit Unit transactions)
CREATE TABLE IF NOT EXISTS icu_ledger (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id  TEXT NOT NULL UNIQUE,       -- opaque UUID for refund correlation
    workspace_id    TEXT NOT NULL,
    model_name      TEXT NOT NULL,
    provider_type   TEXT NOT NULL,
    icu_debit       INTEGER NOT NULL,           -- positive = consumption
    icu_credit      INTEGER NOT NULL DEFAULT 0, -- positive = refund
    refund_status   TEXT NOT NULL DEFAULT 'none', -- 'none' | 'partial' | 'full'
    request_tokens  INTEGER NOT NULL DEFAULT 0, -- tokens in request body
    response_tokens INTEGER NOT NULL DEFAULT 0, -- tokens in response
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ledger_workspace ON icu_ledger(workspace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_window ON icu_ledger(workspace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_txn ON icu_ledger(transaction_id);

-- Window Balances (cached aggregate for fast pre-flight checks)
CREATE TABLE IF NOT EXISTS icu_balances (
    workspace_id    TEXT NOT NULL,
    window_start    DATETIME NOT NULL,           -- e.g., start of today
    total_icu       INTEGER NOT NULL DEFAULT 0,
    last_updated    DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (workspace_id, window_start)
);

-- Generic Entity Metadata (future: Memory, Knowledge, User Profiles)
CREATE TABLE IF NOT EXISTS entity_metadata (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type     TEXT NOT NULL,              -- "memory", "knowledge", "user_profile"
    entity_id       TEXT NOT NULL,              -- e.g., workspace_id or user_id
    key             TEXT NOT NULL,              -- metadata key
    value           BLOB NOT NULL,              -- JSON-encoded value
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(entity_type, entity_id, key)
);

CREATE INDEX IF NOT EXISTS idx_meta_lookup ON entity_metadata(entity_type, entity_id);
```

---

## 4. Adaptive Algorithm Pseudocode

### 4.1 Budget Squeezer (Dynamic Governor)

```
FUNCTION squeezeBudget(request, remainingICU):
    // Calculate base ICU cost
    baseICU = (request.contextChars / 2 + request.maxTokens + request.reasoningBudget) * providerWeight

    IF baseICU <= remainingICU:
        RETURN {allowed: true, squeeze: 1.0}

    // Context density: ratio of system+history to max context window
    density = min(request.contextChars / request.modelContextWindow, 1.0)

    // Squeeze curve: higher density → more aggressive squeeze
    // At 50% density: squeeze to 75% of budget
    // At 90% density: squeeze to 40% of budget
    squeezeFactor = 1.0 - (density * density * 0.75)

    // HARD FLOOR: never squeeze below 20% of original budget.
    // Without this floor the orchestrator could reduce max_tokens to 0
    // on extremely dense contexts, producing empty model responses.
    squeezeFactor = max(squeezeFactor, 0.2)

    squeezedReasoning = floor(request.reasoningBudget * squeezeFactor)
    squeezedMaxTokens = floor(request.maxTokens * squeezeFactor)

    squeezedICU = (request.contextChars / 2 + squeezedMaxTokens + squeezedReasoning) * providerWeight

    IF squeezedICU <= remainingICU:
        return {allowed: true, squeeze: squeezeFactor,
                adjustedReasoningBudged: squeezedReasoning,
                adjustedMaxTokens: squeezedMaxTokens}

    // Even after squeezing, not enough — hard reject
    RETURN {allowed: false, reason: "ICU cap exceeded"}
```

### 4.2 Stream Interceptor (Token Counter + Reactive Termination)

```
FUNCTION processStreamWithBudget(ctx, ch, fullMsg, interceptor):
    tokenCount = 0
    reasoningCount = 0
    contentWindow = ""      // rolling 200-char buffer for code-sniff
    inCodeBlock = false

    FOR EACH chunk IN ch:
        IF ctx.Done():
            RETURN ctx.Err()

        // Code-sniff: update rolling window and detect triple-backtick fences
        contentWindow += chunk.content
        IF len(contentWindow) > 200:
            contentWindow = contentWindow[len(contentWindow)-200:]
        IF contains("```", contentWindow):
            inCodeBlock = !inCodeBlock

        // Adaptive token ratio:
        //   Prose:  0.5 chars/token  (English text ~4 chars/word, ~0.75 tokens/word)
        //   Code:   1.0 chars/token  (operators/punctuation are 1 token each)
        tokenRatio = 0.5
        IF inCodeBlock:
            tokenRatio = 1.0

        textTokens = floor(len(chunk.content) * tokenRatio)
        reasoningTokens = floor(len(chunk.reasoningContent) * 0.5)
        tokenCount += textTokens
        reasoningCount += reasoningTokens

        result = interceptor.interceptChunk(chunk)

        IF result.shouldTerminate:
            // Record partial consumption for fairness
            interceptor.budget.recordPartial(tokenCount, reasoningCount)
            RETURN fmt.Errorf("STREAM_TERMINATED: %s", result.reason)

        appendToFullMsg(fullMsg, chunk)

        // Check reasoning budget separately for models that charge for thinking
        IF reasoningCount > reasoningBudget:
            // Truncate reasoning — stop accumulating, but let completion finish
            chunk.reasoningContent = ""
            reasoningBudget = 0

    RETURN nil
```

### 4.3 Pre-Flight Injection Point (agent.go modification) — with Atomic ICU Refund

```
// In computeNextResponse(), BEFORE a.client.Stream():

var txnID string

IF agent.orchestrator != nil:
    preflight, err := agent.orchestrator.PreFlightCheck(ctx, agent.workspaceID, PreFlightRequest{
        ModelName:       modelName,
        ProviderType:    providerType,
        ContextChars:    totalChars(history),
        MaxTokens:       a.maxTokens,
        ReasoningBudget: a.reasoningBudget,
    })
    IF err != nil:
        return Message{}, fmt.Errorf("BUDGET_ERROR: %w", err)

    IF !preflight.allowed:
        return Message{}, fmt.Errorf("BUDGET_EXCEEDED: %s", preflight.reason)

    txnID = preflight.TransactionID

    // Atomic refund: if the LLM call fails (network timeout, 5xx, etc.),
    // credit the debited ICU back to the workspace balance so the user
    // doesn't lose budget on transient infrastructure errors.
    defer func() {
        if txnID != "" && streamErr != nil {
            agent.orchestrator.Refund(ctx, txnID)
            agent.logger.Warn("ICU refunded due to stream failure", "txn", txnID)
        }
    }()

    // Apply squeeze factors
    IF preflight.squeezeFactor < 1.0:
        a.maxTokens = preflight.adjustedMaxTokens
        a.reasoningBudget = preflight.adjustedReasoningBudget
        a.logger.Warn("budget squeeze applied", "factor", preflight.squeezeFactor)
```

---

## 5. ICU Weight Resolution

Internal Credit Units normalize the "cost" of using different models. 1 ICU = 1 input token on a local 7B model.

```
ResolveICUWeight(cfg ModelConfig) float64:
    1. ProviderConfig.InternalCreditWeight > 0  → use it (user/admin override wins)
    2. Local model with Metadata.Parameters     → derive from parameter count:
         < 4B    → 0.5    4-8B    → 1.0
         8-20B   → 1.5    20-40B  → 2.5    > 40B  → 4.0
    3. Fallback                                  → 1.0 (safe default)
```

### How cloud model weights are set

For OpenRouter (returns pricing in its `/models` API), weight is auto-computed once at registration time:

```
ComputeICUWeightFromPricing(pricing):
    return (prompt + completion) / 0.000001
```

Called in `registry_handlers.go:handleAddModel`. Any model's weight can be overridden in Settings.

### Context Length Resolution (4-Tier)

Determines `max_tokens`, `context_budget`, and `reasoning_budget` for every model.

```
resolveContextLength(cfg):
    1. Metadata.ContextLength          → GGUF scanner (local models, exact per-model)
    2. knownCtx fragment match          → 8 exceptions (deepseek-v3=64K, o3=200K, etc.)
    3. providerCtxDefaults[Provider]    → one line per provider covers all future models:
         gemini/vertex = 1M    openai = 128K
         openrouter/mulerouter/nvidia = 128K    local = 8K
    4. 0                                → ProviderTiers fallback (2048-4096 tokens, safe for any model)
```

`ApplyMetadataDefaults()` runs at model registration time. Only sets fields that haven't been explicitly configured. User overrides always win.

### Meta Parsing from Any OpenAI-Compatible Endpoint

llama.cpp's `/v1/models` returns `meta.n_ctx_train` and `meta.n_params`. These are parsed through `ProviderModelInfo.Meta` and flow to `Metadata.ContextLength` / `Metadata.Parameters` at registration time. No static table needed — the server provides exact per-model data.

### Why no static cost manifest

Static manifests go stale the day after you commit. The system uses three self-updating sources:
- OpenRouter API (pricing + limits for 300+ models, always current)
- GGUF metadata (context length + parameters for local models, exact per-file)
- llama.cpp API (`n_ctx_train` + `n_params` returned directly by the server)

Per-model config override gives precise control without code changes. A ~20 entry fragment map handles the few models whose context differs from their provider default (deepseek-v3=64K, claude=200K, o3/o4=200K, mistral-small=32K, gemini-1.5-pro=2M).

---

## 6. Slot Persistence for llama.cpp (Local Models)

llama.cpp exposes slot management via its HTTP API:

```
GET  /slots              — list all slots and their state
POST /slots/{id}?action=save — snapshot slot KV cache to disk
POST /slots/{id}?action=restore — load slot KV cache from disk
```

**SlotManager workflow**:

1. **On model start**: Poll `GET /slots` until a slot is `idle`
2. **On each request**: Check if a cached slot exists in SQLite with matching `cache_key` (see below)
3. **If match found**: Issue `POST /slots/{n}?action=restore` with the saved cache, skip prompt re-processing
4. **After response**: Issue `POST /slots/{n}?action=save` to persist the KV cache
5. **On idle timeout**: If `slot_timeout` expires without activity, delete slot from SQLite and llama.cpp
6. **Cache key**: `SHA256(system_prompt || first_user_message || temperature || top_p || presence_penalty)`
   Including sampling parameters prevents state corruption: the KV cache encodes attention patterns
   that depend on temperature-driven token selection. Restoring a slot saved at temperature=0.0
   into a session running temperature=1.0 would produce gibberish. The sampling-aware key
   ensures each parameter combination gets its own isolated slot.

This saves **computation budget** by avoiding re-processing the system prompt and initial context on every request. For a ~2000 token system prompt, this saves ~500ms–2s of prefill time depending on model size.

---

## 7. Integration Points (Exact File Locations)

| What | Where | Change |
|---|---|---|
| **New orchestrator package** | `internal/core/orchestrator/` | 8 source + 5 test = 13 files |
| **New ledger package** | `internal/platform/ledger/` | `store.go` + `store_test.go` (SQLite WAL) |
| **ModelConfig extensions** | `models/config.go:303-320` | Add `ReasoningBudget`, `SlotTimeout`, `MaxTokens` |
| **ProviderConfig extensions** | `models/config.go:332-338` | Add `InternalCreditWeight` |
| **ProviderModelInfo + ModelPricing** | `models/llm.go:23-31` | Types for model list with pricing |
| **Provider.ListModels signature** | `models/llm.go:40` | Returns `[]ProviderModelInfo` instead of `[]string` |
| **ModelOverride extensions** | `models/infrastructure.go:31-36` | Add `ReasoningBudget`, `SlotTimeout`, `ICUWeight`, `MaxTokens` |
| **Pre-flight hook** | `internal/core/assistant/agent.go:486` | Call `orchestrator.PreFlightCheck()` before `Stream()` |
| **Stream interception** | `internal/core/assistant/agent.go:535-581` | Call `interceptor.InterceptChunk()` per chunk |
| **Orchestrator wiring** | `internal/app/app_context.go:20-30` | Add `orchestrator *Orchestrator` field |
| **Bootstrap** | `internal/app/bootstrap.go` | Initialize SQLite + orchestrator |
| **RuntimeManager additions** | `internal/core/llm/manager.go:60-83` | Add `GetModelICUWeight()`, `GetModelReasoningBudget()` |
| **MockManager** | `internal/testing/mocks/manager.go` | Add new methods to satisfy updated interface |
| **Tiers update** | `internal/core/assistant/tiers.go` | Add `ReasoningBudget` to `ProviderTuningDefaults` |
| **Frontend model form** | `frontend/src/components/` | Add reasoning budget and slot timeout fields |
| **Admin handlers** | `internal/transport/http/` | Add ICU balance endpoint, reasoning fields |
| **Registry persistence** | `internal/app/app_context.go:424-462` | Add reasoning_budget, slot_timeout to `PersistModel` |
| **Settings persistence** | `internal/app/app_context.go:143-149` | ModelOverride already handles via `ApplyModelOverrides` |
| **go.mod** | `backend/go.mod` | Add `github.com/mattn/go-sqlite3` |

---

## 8. Test Suite Definitions

### 8.1 Unit Tests (`internal/core/orchestrator/*_test.go`)

| Test | What It Verifies |
|---|---|
| `TestPreFlightCheck_UnderBudget` | Request within cap → allowed, squeeze=1.0 |
| `TestPreFlightCheck_OverBudgetSqueeze` | Request exceeds cap → squeeze applied, allowed |
| `TestPreFlightCheck_HardReject` | Even post-squeeze exceeds cap → rejected |
| `TestBudgetSqueezer_HardFloor` | Squeeze factor never drops below 0.2, even at 100% density |
| `TestBudgetSqueezer_HighDensity` | 90% context density → ≥40% squeeze (but ≥20%) |
| `TestBudgetSqueezer_LowDensity` | 20% context density → minimal squeeze |
| `TestBudgetManager_AtomicRefund` | Failed stream → defer refund credits ICU back |
| `TestBudgetManager_DoubleRefundIdempotent` | Calling Refund twice on same txn → no-op on second call |
| `TestStreamInterceptor_CountsTokens` | 100 chunks of 10 chars → ~50 tokens counted (prose mode) |
| `TestStreamInterceptor_CodeSniffRatio` | Content inside ``` blocks → tokenRatio=1.0, not 0.5 |
| `TestStreamInterceptor_CodeSniffToggle` | Triple backticks toggle inCodeBlock correctly |
| `TestStreamInterceptor_TerminateOnOverage` | Exceed cap mid-stream → SHouldTerminate=true |
| `TestReasoningNormalizer_Anthropic` | Thinking block → unified reasoning count |
| `TestReasoningNormalizer_OpenAI` | reasoning_tokens in usage → unified count |
| `TestReasoningNormalizer_Gemini` | thought tokens → unified count |
| `TestICULedger_RecordAndBalance` | Transaction recorded → balance updated |
| `TestSlotManager_SaveAndRestore` | Save slot → restore on next session |
| `TestSlotManager_Expiry` | Slot timeout → expired, deleted |
| `TestEntityMetadata_CRUD` | Set → Get → List → Delete works |
| `TestProviderWeights_AllProviders` | All entries have valid weight > 0 |

### 8.2 Integration Tests

| Test | What It Verifies |
|---|---|
| `TestOrchestratedAgent_WithBudget` | Agent runs within orchestrator budget, completes |
| `TestOrchestratedAgent_BudgetExhausted` | Agent hits cap → graceful error, no panic |
| `TestOrchestratedAgent_SqueezeReducesOutput` | High-density context → reduced max_tokens |
| `TestOrchestratedAgent_ReasoningBudgetEnforced` | Reasoning model → truncated when budget hit |
| `TestOrchestrator_ZeroLatencyImpact` | Standard completion → same latency ±1ms |
| `TestOrchestrator_SQLiteWALConcurrency` | 10 concurrent agents → no SQLITE_BUSY |
| `TestSlotManager_RealLlamaCPP` | Actual llama-server slot save/restore (integration) |

### 8.3 Regression Gates (Phase IV)

| Criterion | Threshold | Measurement |
|---|---|---|
| Standard completion latency | ≤ +1ms vs baseline | `go test -bench` |
| Token counting accuracy | 100% for streaming | Compare estimated vs actual usage |
| Existing agent tests pass | 100% | `go test ./internal/core/assistant/...` |
| Existing proxy tests pass | 100% | `go test ./internal/core/proxy/...` |
| Build succeeds | Zero errors | `go build ./...` |
| SQLite WAL writes | ≤ 100µs per write | Benchmark with 10 concurrent writers |
| MockManager interface complete | All methods implemented | Compiler check |

### 8.4 Model Baseline (Before Change)

Run and record before implementing any changes:

```bash
cd backend
go test ./internal/core/assistant/... -v -count=1 > /tmp/baseline_assistant.txt
go test ./internal/core/proxy/... -v -count=1 > /tmp/baseline_proxy.txt
go test ./... -count=1 > /tmp/baseline_full.txt
go build ./...  # record timing
```

### 8.5 ProviderConnection Verification Matrix

| Model | Provider Type | Test | Expected |
|---|---|---|---|
| GPT Oss 20b | local | `TestLLMClient_Stream` | Streaming works, no budgets |
| Gemma 4 | local/openrouter | `TestLLMClient_Chat` | Chat endpoint responds |
| Qwen 3.5 4b | local | `TestSlotManager_Restore` | Slot save/restore after idle |
| GPT-4o-mini | openai | `TestPreFlightCheck_CloudModel` | ICU weight applied |
| Gemini 2.0 flash | gemini | `TestReasoningNormalizer_Gemini` | Reasoning budget respected |

---

## 9. Phase 0: Testing & Regression Baseline

Before a single line of orchestrator code is written, this phase establishes the performance
baseline for the three local models (Gemma 4-4b, GPT-Oss-20b, Qwen 3.5-4b-instruct) and
builds a cloud mocking framework to validate token counting across all 2025+ provider protocols.

### 9.1 Performance Benchmarking

Capture TTFT (Time to First Token) and TPS (Tokens Per Second) for each local model.
These baselines are compared against post-implementation benchmarks to verify the
stream interceptor adds zero measurable latency.

```bash
#!/bin/bash
# scripts/benchmark_baseline.sh — run from backend/

MODELS=("gemma-4-4b" "gpt-oss-20b" "qwen3.5-4b-instruct")
PROMPT="Write a Go function that sorts a slice of integers in 10 words or fewer."
WARMUP_RUNS=2
MEASURE_RUNS=5

RESULTS_DIR="../docs/PLANS/baselines"
mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT="$RESULTS_DIR/baseline_${TIMESTAMP}.json"

echo "{" > "$OUTPUT"
echo '  "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'",' >> "$OUTPUT"
echo '  "models": {' >> "$OUTPUT"

FIRST=true
for MODEL in "${MODELS[@]}"; do
    $FIRST || echo ',' >> "$OUTPUT"
    FIRST=false

    echo "  Benchmarking $MODEL..."

    # Warmup
    for i in $(seq 1 $WARMUP_RUNS); do
        curl -s -X POST "http://localhost:4001/admin/api/chat" \
            -H "Content-Type: application/json" \
            -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT\"}],\"stream\":true}" \
            > /dev/null
        sleep 2
    done

    # Measurement runs
    echo "    \"$MODEL\": [" >> "$OUTPUT"
    FIRST_RUN=true
    for run in $(seq 1 $MEASURE_RUNS); do
        $FIRST_RUN || echo ',' >> "$OUTPUT"
        FIRST_RUN=false

        START=$(python3 -c 'import time; print(time.time())')
        FIRST_TOKEN_TIME=""
        TOKEN_COUNT=0

        while IFS= read -r line; do
            if [[ "$line" == data:* ]] && [[ "$line" != "data: [DONE]" ]]; then
                if [[ -z "$FIRST_TOKEN_TIME" ]]; then
                    FIRST_TOKEN_TIME=$(python3 -c "import time; print(time.time() - $START)")
                fi
                TOKEN_COUNT=$((TOKEN_COUNT + 1))
            fi
        done < <(curl -s -N -X POST "http://localhost:4001/admin/api/chat" \
            -H "Content-Type: application/json" \
            -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT\"}],\"stream\":true}" 2>/dev/null)

        END=$(python3 -c 'import time; print(time.time())')
        TOTAL_TIME=$(python3 -c "print($END - $START)")
        TPS=$(python3 -c "print($TOKEN_COUNT / max($TOTAL_TIME - ${FIRST_TOKEN_TIME:-0}, 0.001))")

        echo "      {\"run\": $run, \"ttft_s\": ${FIRST_TOKEN_TIME:-0}, \"total_tokens\": $TOKEN_COUNT, \"total_time_s\": $TOTAL_TIME, \"tps\": $TPS}" >> "$OUTPUT"
    done
    echo "    ]" >> "$OUTPUT"
done

echo "  }" >> "$OUTPUT"
echo "}" >> "$OUTPUT"

echo "Baseline written to $OUTPUT"
```

**Acceptance criteria**: Post-implementation TTFT and TPS must be within ±1ms and ±1% of baseline respectively, as measured by `go test -bench` against the orchestrator-wrapped stream.

### 9.2 Cloud Mocking Framework

A set of mock SSE stream producers that simulate provider-specific protocol quirks,
enabling validation of the `StreamInterceptor` and `ReasoningNormalizer` before
real API integration.

```go
// internal/core/orchestrator/mock_streams_test.go (NEW)

// MockProvider produces canned SSE streams matching real provider wire formats.
type MockProvider string

const (
    MockOpenAI    MockProvider = "openai"
    MockAnthropic MockProvider = "anthropic"
    MockGemini    MockProvider = "gemini"
    MockDeepSeek  MockProvider = "deepseek"
)

// MockStreamConfig defines the stream shape for a test case.
type MockStreamConfig struct {
    Provider        MockProvider
    TextChunks      []string  // regular content deltas
    ReasoningChunks []string  // thinking/reasoning deltas (empty = no reasoning)
    UsageTokens     int       // tokens reported in final usage chunk
    ReasoningTokens int       // reasoning tokens in final usage
}

// ProduceStream returns a channel of proxy.ChatResponse that mimics the
// SSE stream for the given provider's wire format.
func ProduceStream(cfg MockStreamConfig) <-chan *proxy.ChatResponse
```

**Mock SSE flow diagrams**:

#### OpenAI (GPT-4o / o-series)
```
SSE: data: {"choices":[{"delta":{"content":"Hello"}}]}
SSE: data: {"choices":[{"delta":{"content":" world"}}]}
SSE: data: {"choices":[{"delta":{"content":"!"}}]}
SSE: data: {"choices":[{"delta":{},"usage":{"prompt_tokens":10,"completion_tokens":3,"reasoning_tokens":0}}]}
SSE: data: [DONE]
```
Distinguishing quirk: `usage` object appears in the final delta alongside an empty content delta. Interceptor must NOT count `usage` as content tokens.

#### Anthropic (Claude 3.5/4 via OpenRouter)
```
SSE: data: {"choices":[{"delta":{"content":null,"reasoning_content":"Let me think"}}]}
SSE: data: {"choices":[{"delta":{"content":null,"reasoning_content":" about this..."}}]}
SSE: data: {"choices":[{"delta":{"content":"The answer is"}}]}
SSE: data: {"choices":[{"delta":{"content":" 42."}}]}
SSE: data: [DONE]
```
Distinguishing quirk: Reasoning blocks arrive BEFORE content in the same stream. `content: null` signals thinking is still active. Interceptor must track `reasoning_content` even when `content` is null. No usage field in stream — token totals must be estimated.

#### Gemini (thinking models via OpenAI-compatible endpoint)
```
SSE: data: {"choices":[{"delta":{"content":"Here's "}}]}
SSE: data: {"choices":[{"delta":{"content":"the result."}}]}
SSE: data: {"usage":{"prompt_tokens":20,"completion_tokens":5,"thoughts_tokens":8}}}
SSE: data: [DONE]
```
Distinguishing quirk: Reasoning tokens are reported as `thoughts_tokens` in a usage chunk that appears as a separate line after the content stream ends. No reasoning content in deltas.

#### DeepSeek R1
```
SSE: data: {"choices":[{"delta":{"reasoning_content":"Step 1: analyze"}}]}
SSE: data: {"choices":[{"delta":{"reasoning_content":"Step 2: conclude"}}]}
SSE: data: {"choices":[{"delta":{"content":"Final answer."}}]}
SSE: data: [DONE]
```
Distinguishing quirk: Reasoning deltas stream BEFORE content deltas. No usage chunk. `reasoning_content` is a separate delta field.

**Mock test cases**:

| Test | Mock Provider | Asserts |
|---|---|---|
| `TestMockStream_OpenAI_Basic` | openai | Token count matches `usage.completion_tokens` |
| `TestMockStream_OpenAI_UsageIgnored` | openai | `usage` chunk contributes zero content tokens |
| `TestMockStream_Anthropic_ReasoningFirst` | anthropic | Reasoning counted separately, content counted after |
| `TestMockStream_Anthropic_NullContent` | anthropic | `content: null` blocks don't crash interceptor |
| `TestMockStream_Gemini_ThoughtsTokens` | gemini | `thoughts_tokens` mapped to reasoning count |
| `TestMockStream_DeepSeek_ReasoningThenContent` | deepseek | Reasoning deltas counted before content begins |
| `TestMockStream_CodeSniff_InsideFence` | openai + ``` blocks | Token ratio switches to 1.0 inside code fences |
| `TestMockStream_CodeSniff_OutsideFence` | openai | Token ratio returns to 0.5 after closing fence |
| `TestMockStream_HardFloorActive` | openai | Squeeze never drops factor below 0.2 |

### 9.3 Table-Driven Regression Suite

```go
// internal/core/orchestrator/regression_test.go (NEW)

func TestRegression_StandardCompletionsIdentical(t *testing.T) {
    tests := []struct {
        name       string
        model      string
        prompt     string
        withBudget bool
    }{
        {"gemma_no_budget", "gemma-4-4b", "What is 2+2?", false},
        {"gptoss_no_budget", "gpt-oss-20b", "List 3 colors.", false},
        {"qwen_no_budget", "qwen3.5-4b-instruct", "Say hello.", false},
        {"gemma_with_budget", "gemma-4-4b", "What is 2+2?", true},
        {"gptoss_with_budget", "gpt-oss-20b", "List 3 colors.", true},
        {"qwen_with_budget", "qwen3.5-4b-instruct", "Say hello.", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 1. Run with baseline (no orchestrator)
            baselineOutput, baselineLatency := runCompletion(t, tt.model, tt.prompt, nil)

            // 2. Run with orchestrator (budget may be nil = no cap)
            var orch *Orchestrator
            if tt.withBudget {
                orch = newTestOrchestrator(t, 1_000_000) // huge cap — no squeeze
            }
            orchOutput, orchLatency := runCompletion(t, tt.model, tt.prompt, orch)

            // 3. Assert output identical
            if baselineOutput != orchOutput {
                t.Errorf("output diverged:\n  baseline: %s\n  orch:     %s", baselineOutput, orchOutput)
            }

            // 4. Assert latency within ±1ms
            delta := orchLatency - baselineLatency
            if delta > 1*time.Millisecond || delta < -1*time.Millisecond {
                t.Errorf("latency delta %.3fms exceeds ±1ms threshold", float64(delta)/float64(time.Millisecond))
            }
        })
    }
}
```

**Regression gate**: `go test ./internal/core/orchestrator/... -run TestRegression -count=5` — all 5 runs must pass before any orchestrator code merges.

---

## 10. Implementation Status

| Phase | Status |
|---|---|
| **A** — `models/config.go` + `models/infrastructure.go` extensions | Done |
| **B** — `internal/platform/ledger/store.go` (SQLite WAL, decoupled) | Done |
| **C** — ICU weight resolution (pricing-based, GGUF-based, no static table) | Done |
| **D** — `budget_manager.go` (pre-flight + refund, 4-tier context resolution) | Done |
| **E** — `budget_squeezer.go` (adaptive squeeze + hard floor) | Done |
| **F** — `stream_interceptor.go` (code-aware token counter) | Done |
| **G** — Wire orchestrator into agent.go with defer refund | Done |
| **H** — `reasoning_normalizer.go` (stream-mode detection, no provider-name mapping) | Done |
| **I** — `slot_manager.go` (llama.cpp slot save/restore, sampling-aware keys) | Done |
| **J** — Bootstrap in AppContext + AppServices | Done |
| **K** — MockManager, tiers, handlers, admin views updated | Done |
| **L** — 80+ tests, all 23 packages pass | Done |
| **M** — Frontend Vue fields (reasoning budget, slot timeout, meta/passing pricing/limits) | Done |
| **N** — Baseline comparison | Pending |

### Package Layout

```
internal/platform/ledger/     (2 files)  — store.go + store_test.go (decoupled SQLite)
internal/core/orchestrator/    (13 files) — budget + squeezer + stream + normalizer + slots + tests

Context length resolution: 4 tiers (metadata → fragment → provider default → tiers fallback)
ICU weight resolution:    3 tiers (config override → GGUF params → default 1.0)
Reasoning detection:      stream-mode from data (not provider name mapping)
```

---

## 11. Risk Assessment

| Risk | Mitigation |
|---|---|
| SQLite write contention under high-frequency loops | WAL mode + NORMAL synchronous; benchmarked at 10 concurrent writers. If contention: batch writes with in-memory ring buffer + periodic flush |
| Stream interception adds latency | Interceptor does only integer math + one comparison (<5µs). Pre-allocation of counters. No allocations in hot path |
| llama.cpp slot API changes | Abstract behind `SlotManager` interface; feature-gate with `SlotTimeout > 0` check |
| ICU weight / context length wrong for new models | 3-tier resolution with auto-compute from OpenRouter API + GGUF metadata + llama.cpp meta. Provider-level defaults cover all future models without maintenance. ~8 fragment overrides for known exceptions. User-correctable via Settings. |
| Breaking existing agent tests | Orchestrator is optional (nil check before use). Agent tests without orchestrator work unchanged |
| Reasoning token counting inaccurate | Use conservative char/2 estimation for text, char for code blocks. Provider-reported actuals (when available) take precedence |
