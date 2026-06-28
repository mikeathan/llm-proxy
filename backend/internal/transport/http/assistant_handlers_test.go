package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
	"os"
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
		{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name: models.ToolSubmitFinalAnswer,
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

	// Mock LLM Response to call tool
	clientMock := mockClient.Client.(*mocks.MockLLMClient)

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
					Role: proxy.AssistantRole,
					ToolCalls: []proxy.ToolCall{{
						ID:   "call_submit",
						Type: "function",
						Function: proxy.FunctionCall{
							Name:      models.ToolSubmitFinalAnswer,
							Arguments: `{"summary": "The lamp is on. It's working perfectly."}`,
						},
					}},
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

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body: %s", w.Code, w.Body.String())
	}

	// Check response body
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if reply, ok := resp["reply"].(string); !ok || !strings.Contains(reply, "The lamp is on.") {
		t.Errorf("expected reply 'The lamp is on.', got %v", resp)
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
	clientMock := mockClient.Client.(*mocks.MockLLMClient)
	clientMock.Responses = []proxy.ChatResponse{
		{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "# Initial Response\nHello workspace!"}}}},
	}

	reqBody := `{"conversation_id": "conv_init", "workspace_id": "test-jail", "message": "hi"}`
	req := httptest.NewRequest("POST", "/api/v1/assistant", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
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
	clientMock := mockClient.Client.(*mocks.MockLLMClient)
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body: %s", w.Code, w.Body.String())
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

	canceledIdx, canceledUserIdx := computeCancelIndices(history)

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

	canceledIdx, canceledUserIdx := computeCancelIndices(history)

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

	canceledIdx, _ := computeCancelIndices(history)

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

	filtered := filterCancelledTurns(history, []int{2})

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
	filtered := filterCancelledTurns(history, nil)

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

	filtered := filterCancelledTurns(history, []int{4})

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

	filtered := filterCancelledTurns(history, []int{0, 4})

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
	filtered := filterCancelledTurns(history, []int{-1, 5, 99})

	if len(filtered) != 3 {
		t.Errorf("expected history unchanged for invalid indices, got %d messages", len(filtered))
	}
}
