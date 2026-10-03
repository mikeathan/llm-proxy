package handlers

import (
	"context"
	"errors"
	"llm-proxy/internal/core/assistant/reasoning"
	"net/http"
	"strings"
	"time"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/memorycapture"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
)

const (
	memoryReviewTimeout     = 120 * time.Second
	memoryReviewMaxTokens   = 2048 // room for a thinking model to reason and still answer
	memoryReviewTemperature = 0.2

	msgReviewBusy   = "The model is busy right now. Try again in a moment."
	msgReviewFailed = "The model could not review this chat. Try again."
)

// memoryReviewItem is one suggestion as the UI shows it.
type memoryReviewItem struct {
	Content   string `json:"content"`
	Scope     string `json:"scope"`
	Mode      string `json:"mode"`
	Duplicate bool   `json:"duplicate"`
}

type memoryReviewResponse struct {
	Suggestions []memoryReviewItem `json:"suggestions"`
}

// ReviewMemories asks the chat's model, once, which facts in a saved conversation are worth remembering and returns
// them for the user to approve. It saves nothing and never touches the conversation. It waits for the model the way a
// chat does (the interactive lane claim), within a time limit.
func (h *AssistantMessageHandler) ReviewMemories(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and session are required", "workspace", "session")
	if !ok {
		return
	}
	workspaceID, sessionID := vals[0], vals[1]

	session, err := h.persistence.ReadSession(workspaceID, sessionID)
	if err != nil {
		h.logger.Error("memory review: failed to read session", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to read session")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.reviewTimeoutOrDefault())
	defer cancel()
	model := h.chatModel()
	if h.lane != nil {
		claimed, release, err := h.lane.ClaimInteractive(ctx, h.svc.LaneKeyFor(""), workspaceID, model)
		if err != nil {
			h.logger.Info("memory review not admitted to the run lane", "workspace", workspaceID, "error", err)
			writeJSONError(w, http.StatusServiceUnavailable, msgReviewBusy)
			return
		}
		defer release()
		ctx = claimed
	}

	client, err := h.svc.GetClientForModel(ctx, model)
	if err != nil {
		h.logger.Warn("memory review: no model client", "model", model, "error", err)
		writeJSONError(w, http.StatusBadGateway, msgReviewFailed)
		return
	}
	reviewer := memorycapture.NewReviewer(chatCompleter{client: client, model: model}, assistantPkg.NewCaptureLibrary(h.svc.MemoryStore()), prompts.MemoryReviewPrompt, tools.ContainsSecret)
	suggestions, err := reviewer.Review(ctx, workspaceID, assistantPkg.ReviewTurns(session.History))
	if err != nil {
		h.logger.Warn("memory review failed", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusBadGateway, msgReviewFailed)
		return
	}
	respondJSON(w, memoryReviewResponse{Suggestions: reviewItems(suggestions)})
}

func (h *AssistantMessageHandler) reviewTimeoutOrDefault() time.Duration {
	if h.reviewTimeout > 0 {
		return h.reviewTimeout
	}
	return memoryReviewTimeout
}

func reviewItems(suggestions []memorycapture.Suggestion) []memoryReviewItem {
	items := make([]memoryReviewItem, 0, len(suggestions))
	for _, s := range suggestions {
		items = append(items, memoryReviewItem{Content: s.Content, Scope: string(s.Scope), Mode: string(s.Mode), Duplicate: s.Duplicate})
	}
	return items
}

// chatCompleter adapts a model client to memorycapture.Completer: one non-streaming, tool-free completion.
type chatCompleter struct {
	client proxy.Client
	model  string
}

func (c chatCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	req := proxy.ChatRequest{
		Model:       c.model,
		Messages:    []proxy.Message{{Role: proxy.SystemRole, Content: system}, {Role: proxy.UserRole, Content: user}},
		MaxTokens:   memoryReviewMaxTokens,
		Temperature: memoryReviewTemperature,
	}
	// Extracting a few facts needs no thinking; a local thinking model would
	// spend the whole allowance on it and answer nothing.
	if c.client.ReasoningField() == proxy.ReasoningFieldThinkTokens {
		reasoning.DisableThinking(&req)
	}
	resp, err := c.client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("the model returned no choices")
	}
	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		// A thinking model can spend the whole allowance reasoning. Report it as
		// a failed review rather than as "no suggestions".
		return "", errors.New("the model returned no answer")
	}
	return content, nil
}
