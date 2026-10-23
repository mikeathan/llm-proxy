// Package chat runs one chat turn from admission to answer: it registers the workspace's running turn (a new message
// replaces it, Stop cancels it), claims the run lane, waits for a cold model, opens the run's recording and hands the
// turn to the conversation service. The HTTP handlers and the connectors call it; neither carries any of this.
package chat

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/core/run"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// Deps is what a turn needs from the app: the conversation service's own needs, and the model clients, the run lane and
// the tool plumbing the lifecycle hands to it.
type Deps interface {
	assistant.ConversationDeps
	ClientProvider() proxy.LLMClientProvider
	Lane() *runlane.Scheduler
	LaneKeyFor(model string) runlane.LaneKey
	Engine() assistant.Engine
	ToolProvider() assistant.ToolProvider
	RootDir() string
	Persistence() *persistence.WorkspaceManager
}

// Service is the chat turn lifecycle.
type Service struct {
	deps    Deps
	exec    assistant.ConversationService
	lane    *runlane.Scheduler
	running sync.Map // workspace id -> *runningTurn
	// ownerZone returns the owner's configured IANA zone, or "" when unset; it outranks the zone a browser reports.
	ownerZone func() string
}

// runningTurn is a workspace's registered turn.
type runningTurn struct {
	cancel         context.CancelFunc
	done           chan struct{}
	conversationID string
	granted        atomic.Bool // true once the run lane admitted the turn
}

// New returns the lifecycle over the app's services. ownerZone may be nil.
func New(deps Deps, ownerZone func() string) *Service {
	return &Service{
		deps: deps, exec: assistant.NewConversationService(deps, deps.Persistence()), lane: deps.Lane(), ownerZone: ownerZone,
	}
}

// runExitWait bounds how long a cancel waits for the cancelled turn to publish its last events and exit before giving up.
const runExitWait = 2 * time.Second

// Run registers the workspace's turn (cancelling a prior in-flight one), admits it to the run scheduler (preempting a
// running automation in the same workload lane, or queueing behind other work), runs it, then unregisters it.
//
// The turn's context comes from context.Background(), NOT the caller's: a client disconnect (page refresh, tab close,
// aborted fetch) must not kill an in-flight turn. It outlives the request and is observed over the event bus; only
// Cancel stops it. ctx is kept for request-scoped values and does not cancel the turn. A turn that was cancelled
// returns its partial result and no error.
func (s *Service) Run(ctx context.Context, req *Request, log logging.Logger) (assistant.ExecuteResult, error) {
	// Resolve the conversation id up front so Running can report it (the UI marks the session as running after a
	// refresh); resolving it again later is a no-op.
	req.ConversationID = assistant.NormalizeConversationID(req.ConversationID)

	execCtx, cancel := context.WithCancel(context.Background())
	turn := &runningTurn{cancel: cancel, done: make(chan struct{}), conversationID: req.ConversationID}
	s.replaceTurn(req.WorkspaceID, turn, log)
	defer s.finishTurn(req.WorkspaceID, turn)

	runCtx := execCtx
	if s.lane != nil {
		// Every turn is admitted through the scheduler: an interactive chat preempts a running automation in its lane,
		// or queues FIFO behind other chats. The claim derives from execCtx, so cancelling the turn (Stop, or a
		// replacement) reaches it while it waits or runs. The model is recorded so the residency gate refuses evicting
		// this chat's model.
		claimed, release, err := s.lane.ClaimInteractive(execCtx, s.deps.LaneKeyFor(req.Model), req.WorkspaceID, s.modelFor(req.Model))
		if err != nil {
			// Cancelled while waiting, or the preempted automation did not stop in time: the same as an explicit
			// cancel, with no failed-run artifacts.
			log.Info("assistant run not admitted to the run lane", "workspace", req.WorkspaceID, "error", err)
			return assistant.ExecuteResult{}, nil
		}
		defer release()
		turn.granted.Store(true)
		runCtx = claimed
	}
	return s.execute(runCtx, req, log)
}

// stopTurn cancels a turn that has already been removed from the registry and waits (bounded) for it to exit, so none
// of its events can interleave with whatever starts next on the same workspace.
func (s *Service) stopTurn(workspaceID string, t *runningTurn, log logging.Logger) {
	t.cancel()
	select {
	case <-t.done:
	case <-time.After(runExitWait):
		log.Warn("assistant run did not exit after cancel; proceeding anyway", "workspace", workspaceID, "wait", runExitWait)
	}
}

// replaceTurn registers t as the workspace's running turn and stops whatever it replaced. Registration is one atomic
// Swap, so two turns starting at once cannot both miss each other: exactly one stays registered and the other is
// cancelled and waited for before the new turn proceeds.
func (s *Service) replaceTurn(workspaceID string, t *runningTurn, log logging.Logger) {
	prev, loaded := s.running.Swap(workspaceID, t)
	if !loaded {
		return
	}
	log.Info("canceled prior in-flight assistant request for workspace", "workspace", workspaceID)
	s.stopTurn(workspaceID, prev.(*runningTurn), log)
}

// finishTurn ends a turn's registration. It removes the workspace entry only if it still belongs to this turn: a newer
// one may already have replaced it, and must stay cancellable and visible as running.
func (s *Service) finishTurn(workspaceID string, t *runningTurn) {
	s.running.CompareAndDelete(workspaceID, t)
	t.cancel()
	close(t.done)
}

// Cancel stops the workspace's running turn and returns once it has exited (bounded by runExitWait), so a message sent
// right after Stop never shares the event stream with the cancelled turn's stale output. It reports whether a turn
// was running.
func (s *Service) Cancel(workspaceID string, log logging.Logger) bool {
	if workspaceID == "" {
		return false
	}
	v, ok := s.running.LoadAndDelete(workspaceID)
	if !ok {
		return false
	}
	s.stopTurn(workspaceID, v.(*runningTurn), log)
	return true
}

// Running reports whether a turn is registered for the workspace. It does not remove or cancel it.
func (s *Service) Running(workspaceID string) bool {
	_, ok := s.running.Load(workspaceID)
	return ok
}

// Queued reports whether the workspace has a turn registered but not yet admitted to the run lane, so the UI can show
// "waiting" instead of a running glow (Running stays true while waiting).
func (s *Service) Queued(workspaceID string) bool {
	if s.lane == nil {
		return false
	}
	if v, ok := s.running.Load(workspaceID); ok {
		if t, ok := v.(*runningTurn); ok {
			return !t.granted.Load()
		}
	}
	return false
}

// RunningConversation is the conversation id of the workspace's running turn, "" when none is running. It backs
// /active-runs, so the UI can reconcile per-session running state after a refresh without trusting client-side flags.
func (s *Service) RunningConversation(workspaceID string) string {
	if v, ok := s.running.Load(workspaceID); ok {
		if t, ok := v.(*runningTurn); ok {
			return t.conversationID
		}
	}
	return ""
}

func (s *Service) execute(ctx context.Context, req *Request, log logging.Logger) (assistant.ExecuteResult, error) {
	client, err := s.acquireClient(ctx, log, req)
	if err != nil {
		// Surface on the event bus so the client renders a visible error instead of hanging.
		s.publishRunError(req, err.Error())
		return assistant.ExecuteResult{}, err
	}

	// The conversation id is already resolved (Run did it), so the recording's run directory is created under a
	// well-formed {model}/{conversation} path and the session uses the same id.
	runID := models.NewRunID()
	// The run id rides in the context so the conversation service reuses it instead of generating a second one: a
	// mismatched id left a recording file, descriptor and sync goroutine open forever (CloseRun never found the
	// orphaned state under the service-generated id).
	ctx = models.WithRunID(ctx, runID)
	procLog := s.deps.ProcessLogger(req.WorkspaceID)
	eventSink := run.OpenRecording(client, run.RecordingSpec{
		Enabled: s.deps.RunLoggingEnabled(), Root: s.deps.RootDir(), WorkspaceID: req.WorkspaceID,
		Task: req.ConversationID, Model: s.modelName(req), RunID: runID, Log: procLog,
	}).Events
	defer func() {
		if eventSink != nil {
			eventSink.Close()
		}
		if rcl, ok := client.(interface{ CloseRun(string) }); ok {
			rcl.CloseRun(runID)
		}
	}()

	var recorder assistant.EventRecorder
	if eventSink != nil {
		recorder = eventSink
	}

	ctx = models.WithHelpersRequest(models.WithProfileRequest(models.WithModelOverride(ctx, req.Model), req.Profile), req.UseHelpers)
	result, execErr := s.exec.Execute(ctx, assistant.ExecuteRequest{
		WorkspaceID: req.WorkspaceID, ConversationID: req.ConversationID, Message: req.Message, ContextVersion: req.ContextVersion,
		Timezone: s.zone(req.Timezone), ExcludeTools: req.ExcludeTools, Log: procLog, Provider: s.deps.ToolProvider(),
		Client: client, Engine: s.deps.Engine(), Events: s.deps.Events(), Recorder: recorder,
	})
	if execErr != nil {
		if errors.Is(execErr, context.Canceled) {
			return result, nil
		}
		return assistant.ExecuteResult{}, execErr
	}
	return result, nil
}

// zone is the timezone this turn reports to the agent for a browser-reported zone.
func (s *Service) zone(browser string) string {
	owner := ""
	if s.ownerZone != nil {
		owner = s.ownerZone()
	}
	return reminders.ChatZone(owner, browser)
}

// modelName is the model the request runs: the one it names, else the primary.
func (s *Service) modelName(req *Request) string {
	if req.Model != "" {
		return req.Model
	}
	primary, _ := s.deps.SelectModels()
	return primary
}

// Model is the model a chat message that names none will use: the registry primary, else the fallback.
func (s *Service) Model() string { return s.modelFor("") }

// modelFor resolves the model a turn will use: the one it names, else the registry primary, else the fallback. It
// mirrors the resolution LaneKeyFor applies, so the lane key and the holder's model agree.
func (s *Service) modelFor(chosen string) string {
	if chosen != "" {
		return chosen
	}
	primary, fallback := s.deps.SelectModels()
	if primary != "" {
		return primary
	}
	return fallback
}

// acquireClient returns the turn's model client. A local model that is still loading (after an idle unload or a
// restart) is waited for, bounded by models.ModelStartWaitTimeout and cancelled by Stop, with one "model is starting"
// notice so the chat shows why it is waiting — instead of failing and losing the message.
func (s *Service) acquireClient(ctx context.Context, log logging.Logger, req *Request) (proxy.Client, error) {
	// A model chosen for this message is used as is: no silent switch to the fallback.
	acquire := func() (proxy.Client, error) { return s.deps.ClientProvider().GetClient(ctx) }
	if req.Model != "" {
		acquire = func() (proxy.Client, error) { return s.deps.ClientProvider().GetClientForModel(ctx, req.Model) }
	}
	client, err := proxy.WaitForClient(ctx, acquire, proxy.ModelWait{
		ModelName:    s.modelName(req),
		PollInterval: models.ModelStartPollInterval,
		Timeout:      models.ModelStartWaitTimeout,
		OnStarting:   func() { s.publishModelStarting(req) },
	})
	if err == nil {
		return client, nil
	}
	if errors.Is(err, models.ErrModelStarting) {
		return nil, fmt.Errorf("the model did not finish loading within %v; try again", models.ModelStartWaitTimeout)
	}
	log.Error("get LLM client failed", "error", err)
	// An actionable message when no model is selected, so the user gets a clear next step instead of a silent failure.
	if primary, _ := s.deps.SelectModels(); primary == "" {
		return nil, errors.New("No primary model is configured. Add and select a model in Settings → Providers, then retry.")
	}
	return nil, errors.New("failed to get LLM client")
}

// publishModelStarting tells the chat its model is loading, with the same upstream notice the UI already renders for a
// model that is starting.
func (s *Service) publishModelStarting(req *Request) {
	if req.WorkspaceID == "" {
		return
	}
	s.deps.Events().Publish(req.WorkspaceID, assistant.AgentEvent{
		ID:             fmt.Sprintf("model_starting_%d", time.Now().UnixNano()),
		Type:           assistant.EventUpstream,
		Channel:        assistant.ChannelAssistant,
		ConversationID: req.ConversationID,
		Payload:        assistant.UpstreamEventPayload{Event: assistant.UpstreamEventRetry, Reason: assistant.UpstreamReasonModelStarting},
		Timestamp:      time.Now(),
	})
}

// publishRunError emits an error event for the conversation. A failure before the conversation service runs happens
// where its own events are published, so it must be surfaced here to avoid a silent hang. The event goes on the
// workspace channel: the stream is workspace-scoped, so only the workspace id is needed; the conversation id is kept
// when present and is never a delivery prerequisite.
func (s *Service) publishRunError(req *Request, message string) {
	if req.WorkspaceID == "" {
		return
	}
	s.deps.Events().Publish(req.WorkspaceID, assistant.AgentEvent{
		ID:             fmt.Sprintf("run_err_%d", time.Now().UnixNano()),
		Type:           assistant.EventError,
		Channel:        assistant.ChannelAssistant,
		ConversationID: req.ConversationID,
		Payload:        map[string]any{"error": message},
		Timestamp:      time.Now(),
	})
}
