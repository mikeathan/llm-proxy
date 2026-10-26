# Event Streaming Patterns — SSE, Observers & Lifecycle Events — Heartbeat Goroutine Cleanup

Reference for the [`event-streaming-patterns`](../SKILL.md) skill, loaded on demand. You are touching stream heartbeats or chasing a goroutine leak.

## Heartbeat Goroutine Cleanup

`computeNextResponseNonStreaming` spawns a heartbeat goroutine that ticks every
`nonStreamHeartbeatInterval` to emit `fallback_waiting` events. The goroutine
must exit cleanly when the stream ends:

```go
heartbeatDone := make(chan struct{})

go func() {
    defer close(heartbeatDone)
    for {
        select {
        case <-ctx.Done():
            return
        case <-streamDone:
            return
        case <-time.After(interval):
            notifyLifecycle("fallback_waiting", elapsed)
        }
    }
}()

// When computeNextResponseNonStreaming returns:
close(streamDone)
<-heartbeatDone  // wait for goroutine to exit
```

The `streamDone` channel ensures the goroutine exits when `processStream` returns
for ANY reason (stuck detection, stream EOF, errors), not just context cancellation.
Always pair `select { case <-ctx.Done(): case <-streamDone: }` in new goroutines.
