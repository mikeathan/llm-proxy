# Plan: Record-and-Replay LLM Testing Framework

## Goal
Allow automated testing of agent behavior against real LLM responses without running live models. Capture streaming and non-streaming responses during real proxy/agent executions via a command-line argument, and replay them in tests.

---

## 1. Recording Layer (`internal/core/proxy/recorder/recorder.go`)

### Trigger
Enabled via a CLI command line flag on `main.go` (e.g., `--record-dir`) or via test arguments, rather than an environment variable. 

### Separate Package & Decorator Pattern
To isolate concerns and avoid dependency cycles, all recording logic resides in a dedicated `internal/core/proxy/recorder` package. We wrap the `proxy.Client` returned by the factory in a `RecordingClient` decorator rather than adding hooks to the production `LLMClient` class:

```go
type RecordingClient struct {
    underlying proxy.Client
    recordDir  string
    sessionID  string
    mu         sync.Mutex
}
```

This decorator intercepts `Chat()` and `Stream()`, writing the interactions to a session-level JSONL file before forwarding the responses to the caller.

### Session-Level JSONL Format (Multi-Turn support)
An agent task execution usually spans **multiple turns** (up to 25–35 steps). To avoid generating dozens of disconnected files per session, all calls in a session are recorded sequentially into a single JSONL file:

```
{record-dir}/{model-name}/{timestamp}_{session-id}.jsonl
```

#### Line Types in JSONL:
1. **Request Metadata Line** (logged when a call begins):
   ```json
   {"type": "request", "model": "gemma4", "tool_format": "native", "messages": [...], "tools": [...]}
   ```
2. **Response Chunk Line** (for streaming responses):
   ```json
   {"type": "chunk", "choices": [{"delta": {"content": "text chunk"}}]}
   ```
3. **Response Final Line** (for non-streaming or completed stream responses):
   ```json
   {"type": "response", "choices": [{"message": {"content": "full text answer"}}]}
   ```
4. **Error Line** (to capture LLM provider failures like rate limits or timeouts):
   ```json
   {"type": "error", "status_code": 429, "message": "rate limit exceeded"}
   ```
5. **Stream Complete Line**:
   ```json
   {"type": "done", "total_chunks": 47}
   ```

### Ensuring Complete & Exact Response Capture
To guarantee that no responses, chunks, or errors are missed during recording:
* **Wrap All Client Methods:** The decorator wraps both `Chat()` and `Stream()` methods to capture both streaming and non-streaming behaviors.
* **Immediate Write-Through (Sync):** To prevent losing data if the server crashes or is force-killed midway through a task, each JSONL line is serialized and flushed to disk immediately using direct writes or `file.Sync()`.
* **Preserve Raw Chunk Structure:** Stream chunks can contain empty deltas, finish reasons (e.g., `stop`, `length`), tool call objects, or reasoning content. The recorder logs the raw unmarshaled `ChatResponse` structure exactly as returned by the downstream LLM, ensuring the replayed stream matches the live model's behavior.
* **Capture Network & Context Errors:** If the connection drops or the request times out mid-stream, the decorator catches the error from the stream channel and appends an `error` line in the JSONL before closing, ensuring the mock replayer can simulate the failure.

---

## 2. CLI Invocation for the Proxy Server

To run the LLM proxy server in recording mode, pass the `--record-dir` flag:

```bash
go run main.go --data ./data --record-dir=testdata/recordings
```

### Server Integration (`internal/app/bootstrap.go` & `main.go`)
1. **Flag Definition:** Declare `recordDir` flag in `main.go` and parse it alongside `--data`.
2. **Propagation:** Pass `recordDir` to the bootstrap sequence.
3. **Client Wrapper:** Wrap the `proxy.Client` instantiated in the factory inside `bootstrap.go`:
   ```go
   factory := func(baseURL string, model string, headers http.Header) proxy.Client {
       client := proxy.NewLLMClient(baseURL, model, nil, headers)
       if recordDir != "" {
           return proxy.NewRecordingClient(client, recordDir)
       }
       return client
   }
   ```

---

## 3. Replay MockClient (`internal/testing/llmprofiles/`)

A `proxy.Client` implementation (`FixtureClient`) that loads a session-level JSONL file and plays it back:

```go
type FixtureClient struct {
    recordedCalls []RecordedCall
    currentIndex  int
    mu            sync.Mutex
}

type RecordedCall struct {
    Request      proxy.ChatRequest
    Response     *proxy.ChatResponse
    Chunks       []*proxy.ChatResponse
    Error        error
}
```

### Request Matching & Fuzzy Logic
When `FixtureClient` receives a request, it maps it to the recorded interactions:
* **Fuzzy Prompt Matcher:** To prevent tests breaking due to dynamic prompt content (e.g., current timestamps, temporary file paths, UUIDs, or random tokens), matching should compare key components (like the last user message, the list of tool definitions, or the roles in the message list) rather than performing an exact byte comparison.
* **Sequential Playback Fallback:** If request matching is disabled or fails, playback falls back to sequence order (i.e. first request gets the first recorded interaction).

---

## 4. Integration into Existing Tests

To keep testing lightweight, local, and fully integrated with existing Go toolchains, the replay framework uses **standard Go Table-Driven Tests** as behavioral specifications. This allows direct mocking of Go interfaces (like mock shell executors and tool providers) without the overhead or flakiness of an external process.

Tests load recorded profiles offline and verify the agent's behavior:

```go
func TestAgent_Execute_AgainstRecordings(t *testing.T) {
    llmprofiles.RunAgainstFixtures(t, "smoke-test", func(t *testing.T, client proxy.Client, fixture string) {
        provider := &MockProvider{Tools: defaultTools}
        engine := &MockEngine{Result: "ok"}
        agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 35})
        _, _, err := agent.Execute(context.Background(), initialHistory)
        if err != nil && !strings.Contains(err.Error(), "max steps") {
            t.Errorf("unexpected error for %s: %v", fixture, err)
        }
    })
}
```

---

## 5. Thread Safety, Cancellation, and Tool schemas

### Thread Safety
Both `RecordingClient` and `FixtureClient` must use a `sync.Mutex` when performing file I/O or advancing indices to prevent race conditions during concurrent test executions.

### Context Cancellation
The `RecordingClient` must intercept context cancellation (`ctx.Done()`) during streams and write a partial/cancelled record or error line, ensuring the JSONL file remains syntactically valid.

### Tool Schema Compatibility
During replay, the agent's active `ToolProvider` must export schemas that align with the tool calls expected by the recorded LLM response. If tool schemas change, the replay might result in parsing/validation errors.

---

## 6. File Changes Summary

| File | Change | Description |
|---|---|---|
| `main.go` | MODIFY | Expose `--record-dir` CLI flag |
| `internal/app/bootstrap.go` | MODIFY | Propagate `--record-dir` and decorate client with recorder |
| `internal/core/proxy/recorder/recorder.go` | NEW | `RecordingClient` decorator and session JSONL logger |
| `internal/core/proxy/recorder/recorder_test.go` | NEW | Session-level round-trip and validation tests |
| `internal/testing/llmprofiles/profiles.go` | NEW | `FixtureClient` with fuzzy matching and error/stream playback |
| `internal/testing/llmprofiles/profiles_test.go` | NEW | Playback verification tests |
| `internal/core/assistant/agent_test.go` | ADD | Offline integration test against recorded fixtures |

---

## 7. Open Questions & Resolutions

* **How to identify sessions?**
  Generate a unique UUID or timestamp-based session ID upon the server's first LLM call of an agent task, or group them by workspace ID.
* **How are tool schemas handled?**
  Include tool definition manifests in the `"request"` metadata block of the JSONL so the mock client can verify tool compatibility on playback.
