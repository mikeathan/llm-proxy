package llm

import (
	"context"
	"errors"
	"fmt"

	"llm-proxy/internal/core/runlane"
)

// ErrLocalModelBusy reports that serving the requested model would evict a local
// model an admitted run or inbound caller is still using. The transport maps it
// to a "busy" answer so the caller can wait or retry instead of a live run being
// killed.
var ErrLocalModelBusy = errors.New("local model busy")

// ResidencyGuard answers "may this local-model eviction happen?". It is
// implemented by the run scheduler — the single decision point for model
// residency — and installed at the composition root. A nil guard disables the
// check (the previous unconditional-eviction behaviour).
type ResidencyGuard interface {
	// BlockedBy returns "" when switching active -> requested is allowed for
	// callerKey, or a label naming the user that must finish first. It must not
	// block.
	BlockedBy(active, requested, callerKey string) string
}

// SetResidencyGuard installs the model-residency guard. Called once at startup,
// before the server accepts traffic.
func (m *LLMRuntimeManager) SetResidencyGuard(g ResidencyGuard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.residency = g
}

// checkLocalEvictionLocked refuses a local-model switch that would stop a model
// someone is still using. It is a no-op when the requested model is already
// served, nothing is active, or no guard is installed. Callers hold m.mu; the
// guard itself never blocks, so it is safe to consult here.
func (m *LLMRuntimeManager) checkLocalEvictionLocked(ctx context.Context, requested string) error {
	if m.residency == nil || m.activeModel == nil {
		return nil
	}
	active := m.activeModel.Cfg.Name
	if active == requested {
		return nil
	}
	if label := m.residency.BlockedBy(active, requested, runlane.CallerKey(ctx)); label != "" {
		return fmt.Errorf("%w: %s is using %s", ErrLocalModelBusy, label, active)
	}
	return nil
}
