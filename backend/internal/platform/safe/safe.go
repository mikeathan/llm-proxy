// Package safe provides panic-containment for goroutine boundaries so a single
// failing task can never crash the whole service. Every goroutine that executes
// agent/automation work should be spawned through safe.Go instead of a bare
// `go fn()`.
package safe

import (
	"fmt"
	"runtime/debug"

	"llm-proxy/internal/platform/logging"
)

// Go runs fn in a new goroutine, recovering any panic and logging it with a
// stack trace. The panic is contained: the service keeps running and the
// failure is observable in the logs instead of taking the process down.
func Go(what string, fn func()) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				logging.Error("panic in "+what,
					"error", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}
