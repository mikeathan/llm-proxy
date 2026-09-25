package app

import (
	"context"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

// laneGate is the single adapter between the run scheduler and the subsystems
// that must respect model residency: the model manager (which refuses an
// eviction) and the proxy handler (which admits external /v1 callers). One type
// satisfies both consumer interfaces, so a residency claim is built in exactly
// one place and host policy is read in exactly one place.
type laneGate struct {
	lane     *runlane.Scheduler
	settings func() models.SchedulerConfig
}

var (
	_ llm.ResidencyGuard   = laneGate{}
	_ handlers.InboundGate = laneGate{}
)

// BlockedBy implements llm.ResidencyGuard: it names the user that would be
// stopped by the switch, or "" when the switch is harmless.
func (g laneGate) BlockedBy(active, requested, callerKey string) string {
	res := g.lane.CheckModelSwitch(runlane.ModelClaim{Active: active, Requested: requested, Key: callerKey})
	if res.Allowed || res.BlockedBy == nil {
		return ""
	}
	if res.BlockedBy.Label != "" {
		return res.BlockedBy.Label
	}
	return res.BlockedBy.Key
}

// WaitSeconds implements handlers.InboundGate: it clamps a caller's requested
// wait to host policy — 0 refuses immediately, -1 waits without a budget,
// otherwise at most that many seconds.
func (g laneGate) WaitSeconds(requested int) int {
	limit := g.settings().InboundWait()
	if limit == 0 || requested <= 0 {
		return 0
	}
	if limit < 0 || requested < limit {
		return requested
	}
	return limit
}

// Cancel implements handlers.InboundGate: the caller's own out-of-band cancel,
// and the operator's dismiss.
func (g laneGate) Cancel(key string) bool { return g.lane.CancelModelWait(key) }

// Wait implements handlers.InboundGate. It admits a caller immediately when the
// switch is harmless, otherwise by queueing it behind the run using the model. A
// caller with no budget is refused rather than held — an ordinary OpenAI client
// must never have a connection parked on its behalf. Queue depth is enforced
// here so an unlimited wait cannot become unbounded held connections; the
// caller's own ctx bounds the rest.
func (g laneGate) Wait(ctx context.Context, req handlers.InboundRequest) (func(), error) {
	cfg := g.settings()
	claim := runlane.ModelClaim{Active: req.Active, Requested: req.Model, Key: req.Key, Label: req.Label}
	waiting := req.Wait != 0

	if !waiting && !g.lane.CheckModelSwitch(claim).Allowed {
		return nil, handlers.ErrInboundBusy
	}
	if waiting && g.lane.ModelWaiterCount() >= cfg.InboundQueueDepth() {
		return nil, handlers.ErrInboundQueueFull
	}
	// Policy: inbound may serve by cancelling the run holding the model. It still
	// waits for that run to unwind — promotion never swaps a model under a live
	// run — and the held lane keeps the model from being re-taken first.
	if cfg.InboundMayPreempt() {
		g.lane.PreemptModelHolder(claim.Active)
	}
	return g.lane.WaitForModel(ctx, claim)
}

// gate is this app's residency adapter: the model manager's guard and the proxy
// handler's inbound seam share one instance.
func (s AppServices) gate() laneGate {
	return laneGate{lane: s.runLane, settings: s.schedulerConfig}
}

// Lane returns the run scheduler every agent run is admitted through.
func (s AppServices) Lane() *runlane.Scheduler { return s.runLane }

// schedulerConfig is the live run-scheduler policy, defaulted when unset — the
// gates read it per request, so a settings save applies to the next caller
// without a restart.
func (s AppServices) schedulerConfig() models.SchedulerConfig {
	if cfg := s.AppCtx.GetSettings().Scheduler; cfg != nil {
		return *cfg
	}
	return models.DefaultSchedulerConfig()
}

// LaneKeyFor resolves the workload-class lane for a model. An empty model
// resolves the registry primary — chat runs the primary/fallback model and
// AssistantMessage carries no model of its own. Unresolvable models serialize on
// the local lane (protects the GPU; documented in the run-lane plan).
func (s AppServices) LaneKeyFor(model string) runlane.LaneKey {
	if model == "" {
		if primary, _ := s.SelectModels(); primary != "" {
			model = primary
		}
	}
	if cfg, ok := s.ModelConfig(model); ok && s.workloadClassifier.Classify(cfg) == models.WorkloadCloud {
		return runlane.LaneCloud
	}
	return runlane.LaneLocal
}
