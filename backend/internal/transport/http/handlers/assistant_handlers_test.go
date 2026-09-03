package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

// noopLogger for testing
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any)   {}
func (l *noopLogger) Info(msg string, args ...any)    {}
func (l *noopLogger) Warn(msg string, args ...any)    {}
func (l *noopLogger) Error(msg string, args ...any)   {}
func (l *noopLogger) With(args ...any) logging.Logger { return l }
func (l *noopLogger) SetLevel(logging.Level)          {}
func (l *noopLogger) Level() logging.Level            { return logging.LevelInfo }

func TestHandleAssistant_AgnosticFlow(t *testing.T) {
	// Setup
	logger := &noopLogger{}
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockClient := &mocks.MockLLMClientProvider{
		Client: &mocks.MockLLMClient{},
	}
	mockLimiter := &mocks.MockRateLimiter{}

	// Setup MCP resources
	mockMCP.SetSystemPrompt("System Prompt")

	// Setup MCP Tools
	mockMCP.SetToolsResult([]proxy.Tool{
		{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name: "query_device",
			},
		},
	})

	// Setup Engine
	engine := assistant.NewEngine(mockMCP, logger)

	// service
	service := mocks.NewMockAssistantService(
		mockClient,
		mockLimiter,
		engine,
		mockMCP,
	)

	// Setup Persistence
	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	handler := NewAssistantMessageHandler(service)

	// Gated client: holds the first Chat open so the detached run stays
	// registered in the running map while the test observes it (see
	// gatedLLMClient — an instant mock can finish the run between polls).
	inner := &mocks.MockLLMClient{}
	clientMock := inner
	release := make(chan struct{})
	mockClient.Client = &gatedLLMClient{inner: inner, release: release}
	releaseGate := releaseOnce(release)
	defer releaseGate()

	// 1. First LLM response: Call Tool
	toolArgs := map[string]any{"target_name": "lamp"}
	argsJSON, _ := json.Marshal(toolArgs)

	clientMock.Responses = []proxy.ChatResponse{
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role: proxy.AssistantRole,
					ToolCalls: []proxy.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: proxy.FunctionCall{
							Name:      "query_device",
							Arguments: string(argsJSON),
						},
					}},
				},
			}},
		},
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    proxy.AssistantRole,
					Content: "The lamp is on. It's working perfectly.",
				},
			}},
		},
	}

	// Setup Tool Execution Result
	mockMCP.SetCallToolResult(map[string]any{"state": "on"}, nil)

	// Request
	reqBody := `{"conversation_id": "conv1", "workspace_id": "test-ws", "message": "check lamp"}`
	req := httptest.NewRequest("POST", "/api/conversation/message", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// The message endpoint starts a detached background run and returns
	// immediately with 202 Accepted; the run is observed over SSE.
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body: %s", w.Code, w.Body.String())
	}

	// Wait for the detached run to register, then finish, before asserting.
	if !waitForRunning(t, handler, "test-ws", 5*time.Second) {
		t.Fatal("run did not start within 5s")
	}
	// Release the gated client and wait for completion before asserting on
	// the recorded requests (they are populated only after the gate opens).
	releaseGate()
	if !waitForNotRunning(t, handler, "test-ws", 5*time.Second) {
		t.Fatal("run did not complete within 5s")
	}

	// Check response body
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "running" {
		t.Errorf("expected status=running, got %v", resp["status"])
	}

	// Verify calls
	// Check that the second call to Chat included the tool result
	if len(clientMock.Requests) < 2 {
		t.Fatalf("expected 2 calls to Chat, got %d", len(clientMock.Requests))
	}
	secondCallReq := clientMock.Requests[1] // The call AFTER the tool execution
	msgs := secondCallReq.Messages

	// Expected history: System, User, Assistant (Tool Call), Tool (Result)
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages in history for second call, got %d", len(msgs))
		for i, m := range msgs {
			t.Logf("Message %d: Role=%s Content=%s ToolCalls=%d", i, m.Role, m.Content, len(m.ToolCalls))
		}
	} else {
		// Verify the sequence
		if msgs[2].Role != proxy.AssistantRole {
			t.Errorf("expected message 2 to be Assistant, got %s", msgs[2].Role)
		}
		if len(msgs[2].ToolCalls) == 0 {
			t.Errorf("expected message 2 to have tool calls")
		}

		if msgs[3].Role != proxy.ToolRole {
			t.Errorf("expected message 3 to be Tool, got %s", msgs[3].Role)
		}
		if msgs[3].ToolCallID != "call_1" {
			t.Errorf("expected message 3 ToolCallID to be 'call_1', got %s", msgs[3].ToolCallID)
		}

		// Verify content is the JSON result
		expectedContent := `{"state":"on"}`
		if msgs[3].Content != expectedContent { // approximate check, might depend on map ordering
			// Decode to map to compare if order varies
			var contentMap map[string]any
			if err := json.Unmarshal([]byte(msgs[3].Content), &contentMap); err != nil {
				t.Errorf("failed to unmarshal tool content: %v", err)
			}
			if s, ok := contentMap["state"].(string); !ok || s != "on" {
				t.Errorf("expected tool content state 'on', got %v", contentMap)
			}
		}
	}
}
func TestHandleAssistant_InitialSystemPrompt(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	mockClient := &mocks.MockLLMClientProvider{
		Client: &mocks.MockLLMClient{},
	}
	mockLimiter := &mocks.MockRateLimiter{}
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockMCP.SetSystemPrompt("BASE_PROMPT")
	mockMCP.SetToolsResult([]proxy.Tool{})

	engine := assistant.NewEngine(mockMCP, &noopLogger{})
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)
	service.PersistenceMgr = mgr

	handler := NewAssistantMessageHandler(service)

	// Gated client: keeps the run observable in the running map (see
	// gatedLLMClient — instant mocks can complete the run between polls).
	inner := &mocks.MockLLMClient{}
	clientMock := inner
	release := make(chan struct{})
	mockClient.Client = &gatedLLMClient{inner: inner, release: release}
	releaseGate := releaseOnce(release)
	defer releaseGate()
	clientMock.Responses = []proxy.ChatResponse{
		{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "# Initial Response\nHello workspace!"}}}},
	}

	reqBody := `{"conversation_id": "conv_init", "workspace_id": "test-jail", "message": "hi"}`
	req := httptest.NewRequest("POST", "/api/v1/assistant", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	// Wait for the detached run to register, then finish, before asserting on effects.
	if !waitForRunning(t, handler, "test-jail", 5*time.Second) {
		t.Fatal("run did not start within 5s")
	}
	releaseGate()
	if !waitForNotRunning(t, handler, "test-jail", 5*time.Second) {
		t.Fatal("run did not complete within 5s")
	}

	// Verify the request sent to LLM contains the jail prompt
	if len(clientMock.Requests) == 0 {
		t.Fatal("expected at least one request to LLM")
	}

	firstReq := clientMock.Requests[0]
	if len(firstReq.Messages) == 0 || firstReq.Messages[0].Role != proxy.SystemRole {
		t.Fatal("expected system message as first entry")
	}

	systemContent := firstReq.Messages[0].Content
	if !strings.Contains(systemContent, "STRICT WORKSPACE RULES:") {
		t.Errorf("system prompt missing jail rules: %s", systemContent)
	}

	if !strings.Contains(systemContent, "All file paths MUST be relative to the workspace root") {
		t.Errorf("system prompt missing expected relative path rule: %s", systemContent)
	}
}

func TestHandleAssistant_HistoryTruncation(t *testing.T) {
	// Setup
	mockLimiter := &mocks.MockRateLimiter{}
	mockClient := &mocks.MockLLMClientProvider{Client: &mocks.MockLLMClient{}}
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockMCP.SetSystemPrompt("SYSTEM")
	mockMCP.SetToolsResult([]proxy.Tool{})

	engine := assistant.NewEngine(mockMCP, &noopLogger{})
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)
	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	handler := NewAssistantMessageHandler(service)
	// Gated client: keeps the run observable in the running map (see
	// gatedLLMClient — instant mocks can complete the run between polls).
	inner := &mocks.MockLLMClient{}
	clientMock := inner
	release := make(chan struct{})
	mockClient.Client = &gatedLLMClient{inner: inner, release: release}
	releaseGate := releaseOnce(release)
	defer releaseGate()
	clientMock.Responses = []proxy.ChatResponse{
		{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "# Response\nThis is a long enough response to avoid premature termination check."}}}},
	}

	// Create a very long history that should trigger truncation
	// maxHistoryChars is 128KB. Let's create 200KB of history.
	longContent := strings.Repeat("a", 200*1024)
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "SYSTEM"},
		{Role: proxy.UserRole, Content: longContent},
		{Role: proxy.AssistantRole, Content: "short"},
	}

	// Manually inject history into persistence
	wsID := "test-trunc"
	convID := "conv-trunc"
	service.PersistenceMgr.WriteSession(wsID, &models.AssistantSession{
		ID:      convID,
		History: history,
	})

	reqBody := `{"conversation_id": "conv-trunc", "workspace_id": "test-trunc", "message": "next"}`
	req := httptest.NewRequest("POST", "/api/v1/assistant", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body: %s", w.Code, w.Body.String())
	}

	// Wait for the detached run to register, then finish, before asserting.
	if !waitForRunning(t, handler, "test-trunc", 5*time.Second) {
		t.Fatal("run did not start within 5s")
	}
	releaseGate()
	if !waitForNotRunning(t, handler, "test-trunc", 5*time.Second) {
		t.Fatal("run did not complete within 5s")
	}

	// Check the request sent to LLM
	if len(clientMock.Requests) == 0 {
		t.Fatal("expected at least one request to LLM")
	}

	sentMsgs := clientMock.Requests[0].Messages

	// Verify that the very long message was removed
	for _, m := range sentMsgs {
		if len(m.Content) > 150*1024 {
			t.Errorf("history was not truncated, found message with length %d", len(m.Content))
		}
	}

	// Verify system prompt is still there
	if sentMsgs[0].Role != proxy.SystemRole || sentMsgs[0].Content != "SYSTEM" {
		t.Error("system prompt was incorrectly truncated or modified")
	}
}

func TestAssistant_RenameSession(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmpDir, tmpDir, tmpDir))
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.PersistenceMgr = mgr
	handler := NewAssistantMessageHandler(service)

	wsID := "test-ws"
	sessionID := "conv_rename"
	session := &models.AssistantSession{
		ID:          sessionID,
		WorkspaceID: wsID,
		History: []proxy.Message{
			{Role: proxy.UserRole, Content: "hello"},
			{Role: proxy.AssistantRole, Content: "hi there"},
		},
	}

	if err := mgr.WriteSession(wsID, session); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}

	body := `{"title":"My Renamed Chat"}`
	req := httptest.NewRequest("PATCH", "/admin/api/conversation/sessions/"+wsID+"/"+sessionID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("workspace", wsID)
	req.SetPathValue("session", sessionID)
	w := httptest.NewRecorder()

	handler.RenameSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result models.AssistantSession
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Metadata == nil || result.Metadata["title"] != "My Renamed Chat" {
		t.Errorf("expected metadata.title 'My Renamed Chat', got %v", result.Metadata)
	}

	read, err := mgr.ReadSession(wsID, sessionID)
	if err != nil {
		t.Fatalf("failed to read session after rename: %v", err)
	}
	if read.Metadata == nil || read.Metadata["title"] != "My Renamed Chat" {
		t.Errorf("expected persisted metadata.title 'My Renamed Chat', got %v", read.Metadata)
	}
}

func TestAssistant_RenameSession_NotFound(t *testing.T) {
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(t.TempDir(), t.TempDir(), t.TempDir()))
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.PersistenceMgr = mgr
	handler := NewAssistantMessageHandler(service)

	body := `{"title":"Rename"}`
	req := httptest.NewRequest("PATCH", "/admin/api/conversation/sessions/missing-ws/missing-session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("workspace", "missing-ws")
	req.SetPathValue("session", "missing-session")
	w := httptest.NewRecorder()

	handler.RenameSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAssistant_RenameSession_MissingTitle(t *testing.T) {
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(t.TempDir(), t.TempDir(), t.TempDir()))
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.PersistenceMgr = mgr
	handler := NewAssistantMessageHandler(service)

	body := `{"title":""}`
	req := httptest.NewRequest("PATCH", "/admin/api/conversation/sessions/test/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("workspace", "test")
	req.SetPathValue("session", "test")
	w := httptest.NewRecorder()

	handler.RenameSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAssistant_DeleteAllSessions(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmpDir, tmpDir, tmpDir))
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.PersistenceMgr = mgr
	handler := NewAssistantMessageHandler(service)

	wsID := "test-ws"
	for i := range 3 {
		if err := mgr.WriteSession(wsID, &models.AssistantSession{
			ID: fmt.Sprintf("conv_%d", i),
		}); err != nil {
			t.Fatalf("failed to write session: %v", err)
		}
	}

	req := httptest.NewRequest("DELETE", "/admin/api/conversation/sessions/"+wsID, nil)
	req.SetPathValue("workspace", wsID)
	w := httptest.NewRecorder()

	handler.DeleteAllSessions(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	sessions, err := mgr.ListSessions(wsID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestAssistant_DeleteAllSessions_NoWorkspace(t *testing.T) {
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(t.TempDir(), t.TempDir(), t.TempDir()))
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.PersistenceMgr = mgr
	handler := NewAssistantMessageHandler(service)

	req := httptest.NewRequest("DELETE", "/admin/api/conversation/sessions/", nil)
	req.SetPathValue("workspace", "")
	w := httptest.NewRecorder()

	handler.DeleteAllSessions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAssistant_CancelAgent_NotRunning(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	if handler.CancelAgent("ws-missing", "conv-1") {
		t.Error("CancelAgent should return false when no agent is running")
	}
}

func TestAssistant_CancelAgent_Running(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	canceled := make(chan struct{})
	handler.running.Store("ws-1", &runningAgent{
		cancel: func() { close(canceled) },
		done:   make(chan struct{}),
	})

	if !handler.CancelAgent("ws-1", "conv-1") {
		t.Error("CancelAgent should return true when an agent is running")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Error("cancel func was not invoked")
	}

	if handler.CancelAgent("ws-1", "conv-1") {
		t.Error("CancelAgent should return false after the agent was canceled and removed")
	}
}

func TestAssistant_RunningConversationID(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	if got := handler.RunningConversationID("ws-1"); got != "" {
		t.Errorf("RunningConversationID with no running agent = %q, want \"\"", got)
	}

	handler.running.Store("ws-1", &runningAgent{
		cancel:         func() {},
		done:           make(chan struct{}),
		conversationID: "conv_123",
	})
	if got := handler.RunningConversationID("ws-1"); got != "conv_123" {
		t.Errorf("RunningConversationID = %q, want %q", got, "conv_123")
	}

	// A different workspace has no running agent.
	if got := handler.RunningConversationID("ws-2"); got != "" {
		t.Errorf("RunningConversationID(ws-2) = %q, want \"\"", got)
	}
}

func TestAssistant_CancelPriorForWorkspace_WaitsForDone(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	done := make(chan struct{})
	canceled := make(chan struct{})
	handler.running.Store("ws-1", &runningAgent{
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
	handler.cancelPriorForWorkspace("ws-1", &noopLogger{})
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("cancelPriorForWorkspace returned before prior agent finished; elapsed=%v", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("cancelPriorForWorkspace took too long; elapsed=%v", elapsed)
	}
}

func TestAssistant_CancelPriorForWorkspace_TimesOut(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	// Prior never closes done; cancelPriorForWorkspace should time out at 2s.
	handler.running.Store("ws-1", &runningAgent{
		cancel: func() {},
		done:   make(chan struct{}),
	})

	start := time.Now()
	handler.cancelPriorForWorkspace("ws-1", &noopLogger{})
	elapsed := time.Since(start)

	if elapsed < 2*time.Second {
		t.Errorf("expected to wait ~2s before timing out, got %v", elapsed)
	}
}

func TestAssistant_CancelPriorForWorkspace_NoOp(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	// No prior; should return immediately.
	start := time.Now()
	handler.cancelPriorForWorkspace("ws-1", &noopLogger{})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected immediate return, got %v", elapsed)
	}
}

func TestAssistant_CancelHandler_MissingFields(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	req := httptest.NewRequest("POST", "/admin/api/conversation/cancel", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	handler.CancelAssistantHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAssistant_CancelHandler_EmptyConvIDAllowed(t *testing.T) {
	// conversation_id is optional — the frontend may not have one when the
	// user cancels before the first response returns a session_id.  Cancel
	// by workspace in that case.
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	body := `{"workspace_id":"ws-1"}`
	req := httptest.NewRequest("POST", "/admin/api/conversation/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.CancelAssistantHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if convID, _ := resp["conversation_id"].(string); convID != "" {
		t.Errorf("expected empty conversation_id in response, got %q", convID)
	}
}

func TestAssistant_CancelHandler_NotRunningReturns200(t *testing.T) {
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	handler := NewAssistantMessageHandler(service)

	body := `{"workspace_id":"ws-1","conversation_id":"conv-missing"}`
	req := httptest.NewRequest("POST", "/admin/api/conversation/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.CancelAssistantHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if canceled, _ := resp["canceled"].(bool); canceled {
		t.Error("expected canceled=false when no agent running")
	}
	if convID, _ := resp["conversation_id"].(string); convID != "conv-missing" {
		t.Errorf("expected conversation_id echoed back, got %q", convID)
	}
}

func TestComputeCancelIndices_CancelDuringThinking(t *testing.T) {
	// History: prior turn completed, current turn has only a user message
	// (cancel happened before any assistant content was produced).
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "first question"},
		{Role: proxy.AssistantRole, Content: "first answer"},
		{Role: proxy.UserRole, Content: "second question"},
	}

	canceledIdx, canceledUserIdx := assistant.ComputeCancelIndices(history)

	if canceledIdx != -1 {
		t.Errorf("expected canceledIdx=-1 (no assistant content in current turn), got %d", canceledIdx)
	}
	if canceledUserIdx != 2 {
		t.Errorf("expected canceledUserIdx=2 (the current turn's user message), got %d", canceledUserIdx)
	}
}

func TestComputeCancelIndices_CancelMidStream(t *testing.T) {
	// History: prior turn completed, current turn has user + assistant
	// (cancel happened mid-stream after some assistant content).
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "first question"},
		{Role: proxy.AssistantRole, Content: "first answer"},
		{Role: proxy.UserRole, Content: "second question"},
		{Role: proxy.AssistantRole, Content: "partial response..."},
	}

	canceledIdx, canceledUserIdx := assistant.ComputeCancelIndices(history)

	if canceledIdx != 3 {
		t.Errorf("expected canceledIdx=3 (current turn's assistant message), got %d", canceledIdx)
	}
	if canceledUserIdx != 2 {
		t.Errorf("expected canceledUserIdx=2 (the user message that started the cancelled turn), got %d", canceledUserIdx)
	}
}

func TestComputeCancelIndices_NoLeakIntoPriorTurn(t *testing.T) {
	// Regression test: cancel during thinking must NOT mark the prior
	// turn's assistant message as canceled.  Previously the search found
	// the last assistant message in history regardless of which turn it
	// belonged to, leaking the cancel marker into prior turns.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "first question"},
		{Role: proxy.AssistantRole, Content: "first answer — this was NOT cancelled"},
		{Role: proxy.UserRole, Content: "second question"},
	}

	canceledIdx, _ := assistant.ComputeCancelIndices(history)

	if canceledIdx == 1 {
		t.Error("REGRESSION: cancel during thinking leaked into prior turn's assistant message (index 1)")
	}
}

func TestFilterCancelledTurns_StripsCancelledTurn(t *testing.T) {
	// History has a prior successful turn and a cancelled turn.
	// The cancelled turn (user "tell me a joke" + partial assistant) should
	// be stripped.  The new "list files" user message appended after the cancel is
	// preserved.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "first question"},
		{Role: proxy.AssistantRole, Content: "first answer"},
		{Role: proxy.UserRole, Content: "tell me a joke"},
		{Role: proxy.AssistantRole, Content: "partial joke text..."},
		{Role: proxy.UserRole, Content: "list files"},
	}

	filtered := assistant.FilterCancelledTurns(history, []int{2})

	// The cancelled turn's user message and assistant message should be gone.
	// The new "list files" user message should remain.
	if len(filtered) != 3 {
		t.Fatalf("expected 3 messages after filter, got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].Content != "first question" {
		t.Errorf("expected first message to be 'first question', got %q", filtered[0].Content)
	}
	if filtered[1].Content != "first answer" {
		t.Errorf("expected second message to be 'first answer', got %q", filtered[1].Content)
	}
	if filtered[2].Content != "list files" {
		t.Errorf("expected third message to be 'list files', got %q", filtered[2].Content)
	}
}

func TestFilterCancelledTurns_NoCancelMarkerReturnsUnchanged(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "q1"},
		{Role: proxy.AssistantRole, Content: "a1"},
	}
	filtered := assistant.FilterCancelledTurns(history, nil)

	if len(filtered) != 2 {
		t.Errorf("expected unfiltered history when no cancel marker, got %d messages", len(filtered))
	}
}

func TestFilterCancelledTurns_DoesNotLeakPriorTurns(t *testing.T) {
	// Two prior successful turns + a cancelled turn.  Only the cancelled
	// turn should be stripped.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "q1"},
		{Role: proxy.AssistantRole, Content: "a1"},
		{Role: proxy.UserRole, Content: "q2"},
		{Role: proxy.AssistantRole, Content: "a2"},
		{Role: proxy.UserRole, Content: "q3 CANCELLED"},
		{Role: proxy.AssistantRole, Content: "partial a3"},
	}

	filtered := assistant.FilterCancelledTurns(history, []int{4})

	if len(filtered) != 4 {
		t.Fatalf("expected 4 messages (q1, a1, q2, a2), got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].Content != "q1" || filtered[1].Content != "a1" ||
		filtered[2].Content != "q2" || filtered[3].Content != "a2" {
		t.Errorf("expected only the two prior successful turns, got %+v", filtered)
	}
}

func TestFilterCancelledTurns_StripsAllCancelledTurns(t *testing.T) {
	// Two cancelled turns + one successful turn in the middle.  Both
	// cancelled turns should be stripped, the successful one kept.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "q1 CANCELLED"},
		{Role: proxy.AssistantRole, Content: "partial a1"},
		{Role: proxy.UserRole, Content: "q2"},
		{Role: proxy.AssistantRole, Content: "a2"},
		{Role: proxy.UserRole, Content: "q3 CANCELLED"},
		{Role: proxy.AssistantRole, Content: "partial a3"},
		{Role: proxy.UserRole, Content: "q4"},
	}

	filtered := assistant.FilterCancelledTurns(history, []int{0, 4})

	if len(filtered) != 3 {
		t.Fatalf("expected 3 messages (q2, a2, q4), got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].Content != "q2" {
		t.Errorf("expected first message 'q2', got %q", filtered[0].Content)
	}
	if filtered[1].Content != "a2" {
		t.Errorf("expected second message 'a2', got %q", filtered[1].Content)
	}
	if filtered[2].Content != "q4" {
		t.Errorf("expected third message 'q4', got %q", filtered[2].Content)
	}
}

func TestFilterCancelledTurns_IgnoresInvalidIndices(t *testing.T) {
	// Negative or out-of-range indices are silently ignored.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "q1"},
		{Role: proxy.AssistantRole, Content: "a1"},
		{Role: proxy.UserRole, Content: "q2"},
	}
	filtered := assistant.FilterCancelledTurns(history, []int{-1, 5, 99})

	if len(filtered) != 3 {
		t.Errorf("expected history unchanged for invalid indices, got %d messages", len(filtered))
	}
}

func TestHandleAssistant_PublishesSessionLifecycleEvents(t *testing.T) {
	logger := &noopLogger{}
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockClient := &mocks.MockLLMClientProvider{
		Client: &mocks.MockLLMClient{},
	}
	mockLimiter := &mocks.MockRateLimiter{}
	mockMCP.SetSystemPrompt("System Prompt")
	mockMCP.SetToolsResult([]proxy.Tool{
		{
			Type: "function",
			Function: proxy.FunctionSchema{Name: "query_device"},
		},
	})
	engine := assistant.NewEngine(mockMCP, logger)

	eventBus := automation.NewEventBus()
	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mockMCP)
	service.EventBusRef = eventBus

	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpWorkspaces))
	defer os.RemoveAll(tmpWorkspaces)

	handler := NewAssistantMessageHandler(service)

	clientMock := mockClient.Client.(*mocks.MockLLMClient)
	toolArgs := map[string]any{"target_name": "lamp"}
	argsJSON, _ := json.Marshal(toolArgs)
	clientMock.Responses = []proxy.ChatResponse{
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role: proxy.AssistantRole,
					ToolCalls: []proxy.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: proxy.FunctionCall{
							Name:      "query_device",
							Arguments: string(argsJSON),
						},
					}},
				},
			}},
		},
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    proxy.AssistantRole,
					Content: "The lamp is currently turned on and working well.",
				},
			}},
		},
	}
	mockMCP.SetCallToolResult(map[string]any{"state": "on"}, nil)

	eventCh, _ := eventBus.Subscribe("test-ws", assistant.ChannelAssistant)

	reqBody := `{"conversation_id": "lifecycle-test", "workspace_id": "test-ws", "message": "check lamp"}`
	req := httptest.NewRequest("POST", "/api/conversation/message", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var started, completed bool
	var progressCount int
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case ev := <-eventCh:
			if ev.Type != assistant.EventLifecycle {
				continue
			}
			p, ok := ev.Payload.(map[string]any)
			if !ok {
				continue
			}
			phase, _ := p["phase"].(string)
			switch phase {
			case assistant.PhaseSessionStarted:
				started = true
			case assistant.PhaseSessionProgress:
				progressCount++
			case assistant.PhaseSessionCompleted:
				completed = true
				break loop
			}
		case <-timeout:
			t.Fatal("timed out waiting for session_completed lifecycle event")
		}
	}

	if !started {
		t.Error("expected session_started lifecycle event")
	}
	if progressCount == 0 {
		t.Error("expected at least one session_progress lifecycle event (tool_call)")
	}
	if !completed {
		t.Error("expected session_completed lifecycle event")
	}
}

func TestPublishSessionLifecycle_SkipsEmptyIDs(t *testing.T) {
	eventBus := automation.NewEventBus()
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.EventBusRef = eventBus

	ch, _ := eventBus.Subscribe("any-ws", assistant.ChannelAssistant)

	// Empty workspace should skip publishing
	assistant.PublishSessionLifecycle(eventBus, "", "conv1", "hello", assistant.PhaseSessionStarted)
	assistant.PublishSessionLifecycle(eventBus, "ws1", "", "hello", assistant.PhaseSessionStarted)

	// Verify nothing was published
	select {
	case <-ch:
		t.Error("expected no events for empty workspace/conversation IDs")
	case <-time.After(10 * time.Millisecond):
		// good
	}
}

func TestPublishSessionLifecycle_PublishesWithCorrectPayload(t *testing.T) {
	eventBus := automation.NewEventBus()
	service := mocks.NewMockAssistantService(nil, nil, nil, nil)
	service.EventBusRef = eventBus

	ch, _ := eventBus.Subscribe("ws1", assistant.ChannelAssistant)

	assistant.PublishSessionLifecycle(eventBus, "ws1", "conv1", "hello world", assistant.PhaseSessionProgress)

	select {
	case ev := <-ch:
		if ev.Type != assistant.EventLifecycle {
			t.Errorf("expected lifecycle type, got %s", ev.Type)
		}
		p, ok := ev.Payload.(map[string]any)
		if !ok {
			t.Fatal("expected map payload")
		}
		if p["phase"] != assistant.PhaseSessionProgress {
			t.Errorf("expected phase %q, got %v", assistant.PhaseSessionProgress, p["phase"])
		}
		if p["conversation_id"] != "conv1" {
			t.Errorf("expected conversation_id 'conv1', got %v", p["conversation_id"])
		}
		if p["snippet"] != "hello world" {
			t.Errorf("expected snippet 'hello world', got %v", p["snippet"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle event")
	}
}

func TestAssistant_RunWithCancel_RegistersInMap(t *testing.T) {
	logger := &noopLogger{}
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

	handler := NewAssistantMessageHandler(service)

	// Blocking client: keeps the run in-flight so the running-map registration
	// is deterministically observable (an instant mock can complete the entire
	// run between two waitForRunning polls, making the registration invisible).
	release := make(chan struct{})
	mockClient.Client = &blockingLLMClient{release: release}

	payload := &AssistantMessage{ConversationID: "runc-test", WorkspaceID: "test-ws", Message: "check lamp"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.RunWithCancel(context.Background(), "test-ws", payload, logger)
	}()

	if !waitForRunning(t, handler, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	handler.CancelAgent("test-ws", "")
	<-done

	if handler.RunningExists("test-ws") {
		t.Error("expected running map entry to be removed after cancel")
	}
}

func TestAssistant_RunWithCancel_CanBeCancelled(t *testing.T) {
	logger := &noopLogger{}
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

	handler := NewAssistantMessageHandler(service)

	// Blocking client: keeps the run in-flight so the running-map registration
	// is deterministically observable (see RegistersInMap).
	release := make(chan struct{})
	mockClient.Client = &blockingLLMClient{release: release}

	payload := &AssistantMessage{ConversationID: "cancel-test", WorkspaceID: "test-ws", Message: "hello"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.RunWithCancel(context.Background(), "test-ws", payload, logger)
	}()

	if !waitForRunning(t, handler, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	handler.CancelAgent("test-ws", "")
	<-done
}

// waitForRunning polls runningExists up to timeout to check if the running
// map entry exists.  Unlike CancelAgent it does not destroy the entry.
func waitForRunning(t *testing.T, handler *AssistantMessageHandler, workspaceID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if handler.RunningExists(workspaceID) {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// waitForNotRunning polls until the running map entry is gone (the detached run
// finished or was cancelled).  Used by tests that trigger a run via ServeHTTP
// and then need to observe its asynchronous completion.
func waitForNotRunning(t *testing.T, handler *AssistantMessageHandler, workspaceID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if !handler.RunningExists(workspaceID) {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestAssistant_RunWithCancel_SurvivesParentContextCancel verifies the core
// fix for the "refresh kills the run" regression: cancelling the *parent*
// context passed to RunWithCancel (which previously was r.Context(), killed by
// a client disconnect) must NOT cancel the detached run. The run is derived
// from context.Background() and must keep going until CancelAgent is called.
//
// It uses a blocking mock LLM client so the run stays in-flight, letting us
// assert the run survives an explicit parent-context cancellation (the signal a
// disconnected client delivers).
func TestAssistant_RunWithCancel_SurvivesParentContextCancel(t *testing.T) {
	logger := &noopLogger{}
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

	handler := NewAssistantMessageHandler(service)

	payload := &AssistantMessage{ConversationID: "survive-test", WorkspaceID: "test-ws", Message: "hello"}

	// Use a cancellable parent ctx to simulate a client disconnect (r.Context()).
	parentCtx, cancelParent := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.RunWithCancel(parentCtx, "test-ws", payload, logger)
	}()

	if !waitForRunning(t, handler, "test-ws", time.Second) {
		t.Fatal("RunWithCancel did not register in running map within 1s")
	}

	// Simulate the browser closing the tab / aborting the fetch: cancel the
	// parent request context.
	cancelParent()

	// Give the goroutine a moment to observe the (now inert) parent cancel.
	time.Sleep(50 * time.Millisecond)

	// The run must still be registered — it must NOT have been killed by the
	// parent context cancellation.
	if !handler.RunningExists("test-ws") {
		t.Fatal("run was killed by parent context cancel; it must survive client disconnect")
	}

	// Let the run finish, then clean up.
	close(release)
	handler.CancelAgent("test-ws", "")
	<-done

	if handler.RunningExists("test-ws") {
		t.Error("expected running map entry to be removed after explicit cancel")
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

// gatedLLMClient blocks each Chat call until the gate is released, then
// delegates to an inner MockLLMClient. Unlike blockingLLMClient it supports
// scripted multi-response flows; recording happens only after release, so
// the test can safely read inner.Requests once the run has completed.
// Rationale: the run is detached and an instant mock can complete the entire
// run between two waitForRunning polls on a loaded CI runner, making the
// running-map registration invisible (2026-09 CI flake,
// TestHandleAssistant_InitialSystemPrompt).
type gatedLLMClient struct {
	inner   *mocks.MockLLMClient
	release chan struct{}
}

func (c *gatedLLMClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.release:
		return c.inner.Chat(ctx, req)
	}
}

func (c *gatedLLMClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return c.inner.Stream(ctx, req)
}

func (c *gatedLLMClient) ReasoningField() string { return c.inner.ReasoningField() }

// releaseOnce returns an idempotent gate-release func: safe to call both
// mid-test (to unblock the run) and via defer (for early-fail paths).
func releaseOnce(release chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(release) }) }
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

// TestAssistant_ServeHTTP_Returns202AndKeepsRunning verifies that the message
// endpoint returns immediately (202 Accepted) and starts a detached run that
// outlives the HTTP request, instead of blocking until the agent finishes.
func TestAssistant_ServeHTTP_Returns202AndKeepsRunning(t *testing.T) {
	logger := &noopLogger{}
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

	handler := NewAssistantMessageHandler(service)

	// Blocking client: keeps the detached run in-flight so its registration in
	// the running map is deterministically observable (see RegistersInMap).
	release := make(chan struct{})
	mockClient.Client = &blockingLLMClient{release: release}

	body := `{"workspace_id":"test-ws","conversation_id":"svc-test","message":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/conversation/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if resp["status"] != "running" {
		t.Errorf("expected status=running, got %v", resp["status"])
	}

	// The run must be registered and still alive immediately after the handler
	// returns — the response does not wait for the agent to finish.
	if !waitForRunning(t, handler, "test-ws", time.Second) {
		t.Fatal("ServeHTTP did not start a detached run that outlives the request")
	}

	// Clean up the run. Cancel first, then release the blocking client and wait
	// for the detached goroutine to fully exit — otherwise it may still write to
	// the TempDir after RemoveAll (flaky "directory not empty" cleanup error).
	handler.CancelAgent("test-ws", "")
	if !waitForNotRunning(t, handler, "test-ws", time.Second) {
		t.Fatal("run did not exit after cancel")
	}
	close(release)
}

// TestHandleAssistant_NoTargetModelPublishesErrorEvent verifies that an early
// client-acquisition failure (no primary model configured) is surfaced on the
// SSE bus as an EventError rather than silently discarded. This is the backend
// half of the "UI does nothing with no error" fix.
func TestHandleAssistant_NoTargetModelPublishesErrorEvent(t *testing.T) {
	logger := &noopLogger{}
	mockClient := &mocks.MockLLMClientProvider{
		GetClientErr: errors.New("no target model available"),
	}
	mockLimiter := &mocks.MockRateLimiter{}
	engine := assistant.NewEngine(mocks.NewMockNodeHerder(nil), logger)

	eventBus := automation.NewEventBus()
	eventCh, _ := eventBus.Subscribe("ws-no-model", assistant.ChannelAssistant)

	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mocks.NewMockNodeHerder(nil))
	service.EventBusRef = eventBus
	// SelectModels() defaults to "" so the actionable "no primary model" path fires.

	handler := NewAssistantMessageHandler(service)

	payload := &AssistantMessage{
		WorkspaceID:    "ws-no-model",
		ConversationID: "conv-no-model",
		Message:        "do something",
	}

	_, herr := handler.RunWithCancel(context.Background(), payload.WorkspaceID, payload, logger)
	if herr == nil {
		t.Fatal("expected a handlerError for no target model, got nil")
	}
	if herr.Status != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", herr.Status)
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

// TestHandleAssistant_NoTargetModelPublishesErrorEvent_EmptyConversation verifies
// the regression from the "added a provider but no primary model" scenario: a
// brand-new conversation has an empty ConversationID, and the error event must
// still reach the workspace-scoped SSE bus (it must not be silently dropped).
// The conversation ID is resolved up front in RunWithCancel so the error event
// carries the same ID the session would use — letting the frontend associate
// the failure with a concrete session row.
func TestHandleAssistant_NoTargetModelPublishesErrorEvent_EmptyConversation(t *testing.T) {
	logger := &noopLogger{}
	mockClient := &mocks.MockLLMClientProvider{
		GetClientErr: errors.New("no target model available"),
	}
	mockLimiter := &mocks.MockRateLimiter{}
	engine := assistant.NewEngine(mocks.NewMockNodeHerder(nil), logger)

	eventBus := automation.NewEventBus()
	eventCh, _ := eventBus.Subscribe("ws-empty-conv", assistant.ChannelAssistant)

	service := mocks.NewMockAssistantService(mockClient, mockLimiter, engine, mocks.NewMockNodeHerder(nil))
	service.EventBusRef = eventBus

	handler := NewAssistantMessageHandler(service)

	payload := &AssistantMessage{
		WorkspaceID:    "ws-empty-conv",
		ConversationID: "", // first send: no session id yet
		Message:        "do something",
	}

	_, herr := handler.RunWithCancel(context.Background(), payload.WorkspaceID, payload, logger)
	if herr == nil {
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

// TestGuardrailDecisionHandler_LateOverridePersists proves that when a user
// submits an "allow & remember" decision AFTER the approval wait already
// expired (the agent moved on), the override is still persisted so future calls
// are not re-blocked — the current run's tool stays skipped (SPEC guardrails:
// persist override, tool skipped).
func TestGuardrailDecisionHandler_LateOverridePersists(t *testing.T) {
	service := mocks.NewMockAssistantService(
		&mocks.MockLLMClientProvider{},
		&mocks.MockRateLimiter{},
		assistant.NewEngine(mocks.NewMockNodeHerder(nil), &noopLogger{}),
		mocks.NewMockNodeHerder(nil),
	)
	service.LoggerRef = &noopLogger{}

	// Real persistence so PersistOverride can write the workspace config.
	tmp := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))

	// A store carrying a retained tombstone for an already-timed-out decision.
	store := assistant.NewGuardrailDecisionStore()
	store.Retain(assistant.GuardrailBlockedPayload{
		DecisionID:  "gr_expired",
		Tool:        "execute_terminal_command",
		Args:        `{"command":"wc -l f.txt"}`,
		Category:    "terminal",
		WorkspaceID: "ws-late",
	})
	service.GuardrailStore = store

	// Engine wired to the same persistence, with a no-op readConfig so the
	// persist path writes without needing a pre-existing config file.
	service.GuardrailEng = guardrails.NewGuardrailEngine(
		func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} },
		storage.NewPathResolver(tmp, tmp, tmp),
		service.PersistenceMgr,
		func(workspaceID string) (*models.WorkspaceConfig, error) {
			return &models.WorkspaceConfig{}, nil
		},
	)
	defer service.GuardrailEng.Stop()

	handler := NewAssistantMessageHandler(service)

	body := `{"decision_id":"gr_expired","allow":true,"persist":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/conversation/guardrail-decision", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.GuardrailDecisionHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for late override persist, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"late":"true"`) {
		t.Errorf("expected late=true in response, got %s", rr.Body.String())
	}
}

// TestGuardrailDecisionHandler_LateDenyOrNoPersistRejected proves that a late
// decision only persists an override when the user both allows AND asks to
// remember; a deny or non-persist late decision is a 404 (nothing to apply).
func TestGuardrailDecisionHandler_LateDenyOrNoPersistRejected(t *testing.T) {
	service := mocks.NewMockAssistantService(
		&mocks.MockLLMClientProvider{},
		&mocks.MockRateLimiter{},
		assistant.NewEngine(mocks.NewMockNodeHerder(nil), &noopLogger{}),
		mocks.NewMockNodeHerder(nil),
	)
	service.LoggerRef = &noopLogger{}
	tmp := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))

	store := assistant.NewGuardrailDecisionStore()
	store.Retain(assistant.GuardrailBlockedPayload{
		DecisionID:  "gr_expired",
		Tool:        "execute_terminal_command",
		Args:        `{"command":"wc -l f.txt"}`,
		Category:    "terminal",
		WorkspaceID: "ws-late",
	})
	service.GuardrailStore = store

	handler := NewAssistantMessageHandler(service)

	for _, tc := range []struct {
		name   string
		body   string
		expect int
	}{
		{"deny", `{"decision_id":"gr_expired","allow":false,"persist":true}`, http.StatusNotFound},
		{"no-persist", `{"decision_id":"gr_expired","allow":true,"persist":false}`, http.StatusNotFound},
		{"unknown", `{"decision_id":"gr_missing","allow":true,"persist":true}`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admin/api/conversation/guardrail-decision", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.GuardrailDecisionHandler(rr, req)
			if rr.Code != tc.expect {
				t.Errorf("expected %d, got %d: %s", tc.expect, rr.Code, rr.Body.String())
			}
		})
	}
}
