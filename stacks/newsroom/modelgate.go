package newsroom

import (
	"context"

	"llm-proxy-stacks/sdk"
)

// msgGateWaitEnded is the refusal of a caller whose wait for its turn ended (the caller's own time ran out, or the stack is stopping) before the
// model was free. The model was never asked.
const msgGateWaitEnded = "gave up waiting for another model call of the Newsroom to finish"

// modelGate makes the stack's model calls take turns. The core's model port admits one call in flight per stack and refuses a second with "already
// running"; with watches and briefs both asking, that would fail a watch run whenever an edition is being written. The gate holds the second caller
// until the first is answered instead. It adds no limit of its own: how long a holder takes is bounded by the core's own call timeout, so the wait is
// bounded too, and the caller's context ends it sooner. Every answer and every refusal (a spent daily limit, no model chosen) passes through
// unchanged, so callers keep treating them as before.
type modelGate struct {
	inner stacksdk.Model
	slot  chan struct{}
}

func newModelGate(inner stacksdk.Model) *modelGate {
	return &modelGate{inner: inner, slot: make(chan struct{}, 1)}
}

// Complete implements stacksdk.Model.
func (g *modelGate) Complete(ctx context.Context, req stacksdk.ModelRequest) (stacksdk.ModelResponse, error) {
	select {
	case g.slot <- struct{}{}:
	case <-ctx.Done():
		return stacksdk.ModelResponse{}, stacksdk.Errorf(stacksdk.CodeTimeout, msgGateWaitEnded)
	}
	defer func() { <-g.slot }()
	return g.inner.Complete(ctx, req)
}

var _ stacksdk.Model = (*modelGate)(nil)
