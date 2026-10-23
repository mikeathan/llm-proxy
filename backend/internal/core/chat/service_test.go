package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

func TestAssistant_CancelAgent_NotRunning(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	if svc.Cancel("ws-missing", logging.NewNopLogger()) {
		t.Error("CancelAgent should return false when no agent is running")
	}
}

func TestAssistant_CancelAgent_Running(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	canceled := make(chan struct{})
	done := make(chan struct{})
	svc.running.Store("ws-1", &runningTurn{
		cancel: func() { close(canceled); close(done) },
		done:   done,
	})

	if !svc.Cancel("ws-1", logging.NewNopLogger()) {
		t.Error("CancelAgent should return true when an agent is running")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Error("cancel func was not invoked")
	}

	if svc.Cancel("ws-1", logging.NewNopLogger()) {
		t.Error("CancelAgent should return false after the agent was canceled and removed")
	}
}

func TestAssistant_RunningConversationID(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	if got := svc.RunningConversation("ws-1"); got != "" {
		t.Errorf("RunningConversationID with no running agent = %q, want \"\"", got)
	}

	svc.running.Store("ws-1", &runningTurn{
		cancel:         func() {},
		done:           make(chan struct{}),
		conversationID: "conv_123",
	})
	if got := svc.RunningConversation("ws-1"); got != "conv_123" {
		t.Errorf("RunningConversationID = %q, want %q", got, "conv_123")
	}

	// A different workspace has no running agent.
	if got := svc.RunningConversation("ws-2"); got != "" {
		t.Errorf("RunningConversationID(ws-2) = %q, want \"\"", got)
	}
}

func TestAssistant_CancelAgent_WaitsForRunToExit(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	done := make(chan struct{})
	canceled := make(chan struct{})
	svc.running.Store("ws-1", &runningTurn{
		cancel: func() { close(canceled) },
		done:   done,
	})
	go func() {
		<-canceled
		time.Sleep(50 * time.Millisecond) // the run still winds down after cancel
		close(done)
	}()

	start := time.Now()
	if !svc.Cancel("ws-1", logging.NewNopLogger()) {
		t.Fatal("CancelAgent should report a running agent")
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("CancelAgent returned before the run exited; elapsed=%v", elapsed)
	}
}

func TestAssistant_FinishRun_LeavesNewerRunRegistered(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	old := &runningTurn{cancel: func() {}, done: make(chan struct{})}
	newer := &runningTurn{cancel: func() {}, done: make(chan struct{}), conversationID: "conv-new"}
	svc.running.Store("ws-1", old)
	svc.running.Store("ws-1", newer) // the new message took over the workspace

	svc.finishTurn("ws-1", old)

	if got := svc.RunningConversation("ws-1"); got != "conv-new" {
		t.Errorf("the finished old run unregistered the newer run: running conversation = %q", got)
	}
	select {
	case <-old.done:
	default:
		t.Error("finishRun must close the finished run's done channel")
	}

	svc.finishTurn("ws-1", newer)
	if svc.Running("ws-1") {
		t.Error("finishing the registered run should unregister it")
	}
}

func newTestRun() *runningTurn {
	return &runningTurn{cancel: func() {}, done: make(chan struct{})}
}

func TestAssistant_ReplaceRun_ConcurrentStartsLeaveOneRegistered(t *testing.T) {
	for i := 0; i < 50; i++ {
		svc := New(mocks.NewMockAssistantService(nil, nil, nil, nil), nil)
		var cancelled [2]atomic.Bool
		runs := [2]*runningTurn{}
		for k := range runs {
			done := make(chan struct{})
			runs[k] = &runningTurn{done: done, cancel: func() {
				if cancelled[k].CompareAndSwap(false, true) {
					close(done)
				}
			}}
		}
		var wg sync.WaitGroup
		for k := range runs {
			wg.Add(1)
			go func() { defer wg.Done(); svc.replaceTurn("ws-1", runs[k], logging.NewNopLogger()) }()
		}
		wg.Wait()

		v, ok := svc.running.Load("ws-1")
		if !ok {
			t.Fatal("no run registered")
		}
		kept := v.(*runningTurn)
		for k, ra := range runs {
			if ra != kept && !cancelled[k].Load() {
				t.Fatalf("iteration %d: run %d was replaced but never cancelled — it would run unreachable", i, k)
			}
			if ra == kept && cancelled[k].Load() {
				t.Fatalf("iteration %d: the registered run %d was cancelled", i, k)
			}
		}
	}
}

func TestAssistant_ReplaceRun_WaitsForDone(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	done := make(chan struct{})
	canceled := make(chan struct{})
	svc.running.Store("ws-1", &runningTurn{
		cancel: func() { close(canceled) },
		done:   done,
	})

	// Simulate the prior agent finishing its cleanup work after cancel.
	go func() {
		<-canceled
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()

	start := time.Now()
	svc.replaceTurn("ws-1", newTestRun(), logging.NewNopLogger())
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("replaceRun returned before prior agent finished; elapsed=%v", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("replaceRun took too long; elapsed=%v", elapsed)
	}
}

func TestAssistant_ReplaceRun_TimesOut(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	// Prior never closes done; replaceRun should time out at 2s.
	svc.running.Store("ws-1", &runningTurn{
		cancel: func() {},
		done:   make(chan struct{}),
	})

	start := time.Now()
	svc.replaceTurn("ws-1", newTestRun(), logging.NewNopLogger())
	elapsed := time.Since(start)

	if elapsed < 2*time.Second {
		t.Errorf("expected to wait ~2s before timing out, got %v", elapsed)
	}
}

func TestAssistant_ReplaceRun_NoOp(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	svc := New(service, nil)

	// No prior; should return immediately.
	start := time.Now()
	svc.replaceTurn("ws-1", newTestRun(), logging.NewNopLogger())
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected immediate return, got %v", elapsed)
	}
}

func TestAssistant_RunWithCancel_RegistersInMap(t *testing.T) {
	logger := logging.NewNopLogger()
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockClient := &mocks.MockLLMClientProvider{Client: &mocks.MockLLMClient{}}
	mockLimiter := &mocks.MockRateLimiter{}
	mockMCP.SetSystemPrompt("System Prompt")
	mockMCP.SetToolsResult(nil)
	engine := assistant.NewEngine(mockMCP, logger)
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)

	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	svc := New(service, nil)

	// Blocking client: keeps the run in-flight so the running-map registration
	// is deterministically observable (an instant mock can complete the entire
	// run between two waitForRunning polls, making the registration invisible).
	release := make(chan struct{})
	mockClient.Client = &blockingLLMClient{release: release}

	payload := &Request{ConversationID: "runc-test", WorkspaceID: "test-ws", Message: "check lamp"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(context.Background(), payload, logger)
	}()

	if !waitForRunning(t, svc, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	svc.Cancel("test-ws", logging.NewNopLogger())
	<-done

	if svc.Running("test-ws") {
		t.Error("expected running map entry to be removed after cancel")
	}
}

func TestAssistant_RunWithCancel_CanBeCancelled(t *testing.T) {
	logger := logging.NewNopLogger()
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockClient := &mocks.MockLLMClientProvider{Client: &mocks.MockLLMClient{}}
	mockLimiter := &mocks.MockRateLimiter{}
	mockMCP.SetSystemPrompt("System Prompt")
	mockMCP.SetToolsResult(nil)
	engine := assistant.NewEngine(mockMCP, logger)
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)

	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	svc := New(service, nil)

	// Blocking client: keeps the run in-flight so the running-map registration
	// is deterministically observable (see RegistersInMap).
	release := make(chan struct{})
	mockClient.Client = &blockingLLMClient{release: release}

	payload := &Request{ConversationID: "cancel-test", WorkspaceID: "test-ws", Message: "hello"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(context.Background(), payload, logger)
	}()

	if !waitForRunning(t, svc, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	svc.Cancel("test-ws", logging.NewNopLogger())
	<-done
}

func waitForRunning(t *testing.T, svc *Service, workspaceID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if svc.Running(workspaceID) {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func waitForNotRunning(t *testing.T, svc *Service, workspaceID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if !svc.Running(workspaceID) {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestAssistant_RunWithCancel_SurvivesParentContextCancel(t *testing.T) {
	logger := logging.NewNopLogger()
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockLimiter := &mocks.MockRateLimiter{}
	mockMCP.SetSystemPrompt("System Prompt")
	mockMCP.SetToolsResult(nil)
	engine := assistant.NewEngine(mockMCP, logger)

	// Blocking client: Chat blocks until release is closed, keeping the run
	// in-flight so we can observe parent-context cancellation behaviour.
	release := make(chan struct{})
	blockingClient := &blockingLLMClient{release: release}
	mockClient := &mocks.MockLLMClientProvider{Client: blockingClient}
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)

	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	svc := New(service, nil)

	payload := &Request{ConversationID: "survive-test", WorkspaceID: "test-ws", Message: "hello"}

	// Use a cancellable parent ctx to simulate a client disconnect (r.Context()).
	parentCtx, cancelParent := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(parentCtx, payload, logger)
	}()

	if !waitForRunning(t, svc, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	// Simulate the browser closing the tab / aborting the fetch: cancel the
	// parent request context.
	cancelParent()

	// Give the goroutine a moment to observe the (now inert) parent cancel.
	time.Sleep(50 * time.Millisecond)

	// The run must still be registered — it must NOT have been killed by the
	// parent context cancellation.
	if !svc.Running("test-ws") {
		t.Fatal("run was killed by parent context cancel; it must survive client disconnect")
	}

	// Let the run finish, then clean up.
	close(release)
	svc.Cancel("test-ws", logging.NewNopLogger())
	<-done

	if svc.Running("test-ws") {
		t.Error("expected running map entry to be removed after explicit cancel")
	}
}

func releaseOnce(release chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(release) }) }
}

func TestHandleAssistant_NoTargetModelPublishesErrorEvent(t *testing.T) {
	logger := logging.NewNopLogger()
	mockClient := &mocks.MockLLMClientProvider{
		GetClientErr: errors.New("no target model available"),
	}
	mockLimiter := &mocks.MockRateLimiter{}
	engine := assistant.NewEngine(mocks.NewMockNodeHerder(nil), logger)

	eventBus := eventbus.NewBus()
	eventCh, _ := eventBus.Subscribe("ws-no-model", assistant.ChannelAssistant)

	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mocks.NewMockNodeHerder(nil))
	service.EventBusRef = eventBus
	// SelectModels() defaults to "" so the actionable "no primary model" path fires.

	svc := New(service, nil)

	payload := &Request{
		WorkspaceID:    "ws-no-model",
		ConversationID: "conv-no-model",
		Message:        "do something",
	}

	_, err := svc.Run(context.Background(), payload, logger)
	if err == nil {
		t.Fatal("expected an error for no target model, got nil")
	}
	if !strings.Contains(err.Error(), "No primary model is configured") {
		t.Errorf("error = %q, want the actionable no-primary-model message", err)
	}

	select {
	case ev := <-eventCh:
		if ev.Type != assistant.EventError {
			t.Fatalf("expected EventError on the bus, got %q", ev.Type)
		}
		if ev.ConversationID != "conv-no-model" {
			t.Errorf("event conversation_id = %q, want conv-no-model", ev.ConversationID)
		}
		payloadErr, _ := ev.Payload.(map[string]any)["error"].(string)
		if payloadErr == "" {
			t.Error("event payload missing error message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventError on the bus")
	}
}

func TestHandleAssistant_NoTargetModelPublishesErrorEvent_EmptyConversation(t *testing.T) {
	logger := logging.NewNopLogger()
	mockClient := &mocks.MockLLMClientProvider{
		GetClientErr: errors.New("no target model available"),
	}
	mockLimiter := &mocks.MockRateLimiter{}
	engine := assistant.NewEngine(mocks.NewMockNodeHerder(nil), logger)

	eventBus := eventbus.NewBus()
	eventCh, _ := eventBus.Subscribe("ws-empty-conv", assistant.ChannelAssistant)

	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mocks.NewMockNodeHerder(nil))
	service.EventBusRef = eventBus

	svc := New(service, nil)

	payload := &Request{
		WorkspaceID:    "ws-empty-conv",
		ConversationID: "", // first send: no session id yet
		Message:        "do something",
	}

	_, err := svc.Run(context.Background(), payload, logger)
	if err == nil {
		t.Fatal("expected a handlerError for no target model, got nil")
	}

	select {
	case ev := <-eventCh:
		if ev.Type != assistant.EventError {
			t.Fatalf("expected EventError on the bus, got %q", ev.Type)
		}
		// RunWithCancel resolves the conversation ID up front, so the error
		// event is scoped to the resolved conversation instead of empty.
		if ev.ConversationID == "" {
			t.Error("event conversation_id should be resolved to a generated id")
		}
		payloadErr, _ := ev.Payload.(map[string]any)["error"].(string)
		if payloadErr == "" {
			t.Error("event payload missing error message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventError on the bus (empty conversation_id)")
	}
}

func TestAssistant_RunningQueued(t *testing.T) {
	newLane := func() *runlane.Scheduler { return runlane.New(runlane.Limits{Local: 1, Cloud: 1}, true) }
	tests := []struct {
		name    string
		lane    *runlane.Scheduler
		store   bool
		granted bool
		want    bool
	}{
		{"no lane means never queued", nil, true, false, false},
		{"registered but not granted is queued", newLane(), true, false, true},
		{"granted is not queued", newLane(), true, true, false},
		{"no registered run is not queued", newLane(), false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Service{running: sync.Map{}, lane: tt.lane}
			if tt.store {
				ra := &runningTurn{}
				ra.granted.Store(tt.granted)
				h.running.Store("ws", ra)
			}
			if got := h.Queued("ws"); got != tt.want {
				t.Fatalf("RunningQueued = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssistant_GetLLMClient_WaitsForAColdModel(t *testing.T) {
	restore := models.ModelStartPollInterval
	models.ModelStartPollInterval = time.Millisecond
	defer func() { models.ModelStartPollInterval = restore }()

	bus := eventbus.NewBus()
	defer bus.Stop()
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	provider := &mocks.ColdStartProvider{Starting: 3}
	service.Client = provider
	service.EventBusRef = bus
	svc := New(service, nil)
	events, _ := bus.Subscribe("ws-1", assistant.ChannelAssistant)

	client, err := svc.acquireClient(context.Background(), logging.NewNopLogger(), &Request{WorkspaceID: "ws-1", ConversationID: "c1"})
	if err != nil || client == nil {
		t.Fatalf("acquireClient = %v, %+v; want a client once the model has loaded", client, err)
	}
	if provider.Calls() != 4 {
		t.Errorf("calls = %d, want 3 starting + 1 ready", provider.Calls())
	}

	var notices []assistant.AgentEvent
	for len(events) > 0 {
		ev := <-events
		if ev.Type == assistant.EventUpstream {
			notices = append(notices, ev)
		}
	}
	if len(notices) != 1 {
		t.Fatalf("model-starting notices = %d, want exactly one", len(notices))
	}
	if p, ok := notices[0].Payload.(assistant.UpstreamEventPayload); !ok || p.Reason != "model_starting" || notices[0].ConversationID != "c1" {
		t.Errorf("notice = %+v, want reason model_starting for conversation c1", notices[0])
	}
}

func TestAssistant_GetLLMClient_UsesTheChosenModel(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	provider := &namedProvider{}
	service.Client = provider
	svc := New(service, nil)

	for _, chosen := range []string{"", "nemotron"} {
		if _, err := svc.acquireClient(context.Background(), logging.NewNopLogger(), &Request{WorkspaceID: "ws", Model: chosen}); err != nil {
			t.Fatalf("acquireClient(%q): %+v", chosen, err)
		}
	}
	if got := strings.Join(provider.asked, ","); got != "<primary>,nemotron" {
		t.Errorf("clients asked for = %s, want <primary>,nemotron", got)
	}
	if got := svc.modelFor("nemotron"); got != "nemotron" {
		t.Errorf("chatModelFor = %q, want nemotron", got)
	}
}

func TestAssistant_UseHelpersReachesTheRun(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"asked", `{"workspace_id":"ws","message":"compare A and B","use_helpers":true}`, true},
		{"not asked", `{"workspace_id":"ws","message":"compare A and B"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := mocks.NewMockAssistantService(&mocks.MockLLMClientProvider{Client: &mocks.MockLLMClient{}}, nil, nil, nil)
			svc := New(service, nil)
			capture := &ctxService{}
			svc.exec = capture
			var payload Request
			if err := json.Unmarshal([]byte(tc.body), &payload); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.execute(context.Background(), &payload, logging.NewNopLogger()); err != nil {
				t.Fatalf("execute: %+v", err)
			}
			if capture.asked != tc.want {
				t.Errorf("the run's context asks for helpers = %v, want %v", capture.asked, tc.want)
			}
		})
	}
}

// blockingLLMClient implements proxy.Client but blocks Chat until release is
// closed, so a run stays in-flight for deterministic cancellation tests.
type blockingLLMClient struct {
	release chan struct{}
}

func (c *blockingLLMClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.release:
		return &proxy.ChatResponse{
			Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "ok"}}},
		}, nil
	}
}

func (c *blockingLLMClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	ch := make(chan *proxy.ChatResponse)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
		case <-c.release:
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "ok"}}}}
		}
	}()
	return ch, nil
}

func (c *blockingLLMClient) ReasoningField() string { return proxy.ReasoningFieldBudget }

// ctxService is a ConversationService that records whether the run's context says the message asked for helpers.
type ctxService struct{ asked bool }

func (s *ctxService) Execute(ctx context.Context, _ ExecuteRequest) (ExecuteResult, error) {
	s.asked = models.HelpersRequested(ctx)
	return ExecuteResult{}, nil
}

// namedProvider records which model each client request asked for.
type namedProvider struct{ asked []string }

func (p *namedProvider) GetClient(context.Context) (proxy.Client, error) {
	p.asked = append(p.asked, "<primary>")
	return &mocks.MockLLMClient{}, nil
}

func (p *namedProvider) GetClientForModel(_ context.Context, name string) (proxy.Client, error) {
	p.asked = append(p.asked, name)
	return &mocks.MockLLMClient{}, nil
}
