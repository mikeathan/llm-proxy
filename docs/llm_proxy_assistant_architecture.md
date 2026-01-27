# LLM-Proxy Assistant Architecture

## High-Level Flow

```text
User
  │
  ▼
Assistant API
  │
  ├─ load device context
  ├─ build system prompt
  ├─ call LLM
  │
  ├─┐ if tool call requested
  │ │
  │ ▼
  │ Engine
  │   ├─ parse + normalize args
  │   ├─ resolve device
  │   ├─ execute metrics query
  │   ├─ adaptive lookback retry
  │   └─ return tool result
  │
  └─ feed tool result back to LLM
       repeat until no tools

Final reply returned to user
```

---

## Core Components

### 1. Assistant API Handler

Orchestrates the full agent loop:

1. Load device context
2. Build system + user prompt
3. Call LLM
4. Execute tool calls
5. Feed tool results back
6. Return final response

---

### 2. LLM

Responsible for:

- Understanding user intent
- Deciding when tools are required
- Choosing aggregation + time window
- Producing final natural language reply

---

### 3. Engine

Single responsibility: **execute tool requests safely and deterministically**

Steps:

1. Parse tool arguments
2. Normalize (time windows, aggregation, defaults)
3. Resolve device from context
4. Build metrics query
5. Execute against NodeHerder
6. Adaptive retry for `last` queries

---

### 4. NodeHerder

Provides:

- Device context
- Metrics storage
- Query execution
- Auth + rate limiting

---

## Key Guarantees

- **LLM never sees raw metrics DB**
- **LLM cannot fabricate history**
- **All time-based answers originate from metrics**
- **Engine enforces correctness + normalization**

---

## Failure & Recovery

| Failure          | Handled By    | Behavior                  |
| ---------------- | ------------- | ------------------------- |
| Device ambiguity | Pending state | Ask user to clarify       |
| No recent data   | Engine        | Adaptive lookback retry   |
| Auth expired     | NodeHerder    | Refresh token + retry     |
| Rate limited     | NodeHerder    | Return error to assistant |
| Model starting   | Assistant API | Retry with backoff        |

---

## Why This Works

This architecture prevents hallucinations by separating:

- **Reasoning (LLM)**
- **Validation & execution (Engine)**
- **Data ownership (NodeHerder)**

The LLM decides *what* to ask. The Engine decides *how* it is executed. NodeHerder decides *what data exists*.

No layer is allowed to violate the others.

