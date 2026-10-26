# Testing — Patterns, Tools & Strategies — Record-Replay Testing

Reference for the [`testing-guide`](../SKILL.md) skill, loaded on demand. You are recording a real run or replaying one in a test.

## Record-Replay Testing

### Recording

Every LLM call (Chat or Stream) is written to `data/runs/<model>/<task>/<timestamp>_<session>.jsonl`:

```bash
go run main.go --record
```

Hit different LLMs with different prompts through the proxy or agent API — each model gets its own subdirectory.

### JSONL Fixture Format

One JSON object per line, with these event types:

| Type | When | Fields |
|------|------|--------|
| `request` | Before LLM call | `model`, `messages[]`, `tools[]` |
| `response` | Non-streaming response | `choices[{message}]` |
| `chunk` | Stream delta | `choices[{delta: {content, tool_calls, reasoning}}]` |
| `error` | HTTP/connection error | Error details |
| `done` | Stream completion | `total_chunks` |

```json
{"type":"request","model":"gemma4","messages":[...],"tools":[...]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"answer"}}]}
{"type":"done","total_chunks":1}
```

For streaming sessions, lines alternate `chunk`/`response`/`done` following the initial `request` line.

### Replay

Replay tests are opt-in via the `recordreplay` build tag. The test runner (`llmprofiles.RunAgainstFixtures`) loads all `.jsonl` files, wraps each in a `FixtureClient` that implements `proxy.Client`, and runs the agent against it — identical to a live run:

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

Fixture `.jsonl` files live in `internal/core/assistant/testdata/recordings/`, **but**
`TestAgent_Execute_AgainstRecordings` reads `../../../testdata/recordings` (→ backend/testdata/recordings,
which does not exist) and `t.Skip`s when it finds no `.jsonl` files there. The replay test therefore
**silently skips** today — do not trust a green result; confirm it actually ran, or point the test at
the fixtures directory.

See `docs/PLANS/ARCHIVE/cross-cutting/record-replay-test-framework.md` for the full design.
