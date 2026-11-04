package handlers

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/proxy"
)

// What a summary asks of the model, and how long it waits. The answer is a few sentences; a thinking model is given room to
// reason and still answer, as the memory review does.
const (
	summaryTimeout     = 120 * time.Second
	summaryMaxTokens   = 2048
	summaryTemperature = 0.2
	summaryWindow      = 7 * 24 * time.Hour

	msgSummaryNotLocal = "The model for this workspace runs in the cloud, and network data may only go to a model on this machine. Choose a local model for the workspace."
	msgSummaryBusy     = "The model is busy right now. Try again in a moment."
	msgSummaryFailed   = "The model could not explain the activity. Try again."
	msgSummaryRunning  = "A summary is already being written."
	msgSummaryNoSpace  = "A summary needs the audit's workspace."
)

// TimelineModel leases a model for one short call, the way a chat turn is admitted: *chat.Service.
type TimelineModel interface {
	Lease(ctx context.Context, workspaceID string) (context.Context, proxy.Client, string, func(), error)
}

// NetauditSummaryHandlers write the Network timeline's "explain this week". It is a button's answer and nothing runs by
// itself: one short call to the workspace's model, only if that model is on this machine, with a prompt made of the
// program's own words and numbers and never a device name, an address or the text of an alert.
type NetauditSummaryHandlers struct {
	activity *NetauditActivityHandlers
	model    TimelineModel
	isLocal  func(model string) bool
	running  atomic.Bool
}

func NewNetauditSummaryHandlers(activity *NetauditActivityHandlers, model TimelineModel, isLocal func(model string) bool) *NetauditSummaryHandlers {
	return &NetauditSummaryHandlers{activity: activity, model: model, isLocal: isLocal}
}

type summaryResponse struct {
	// Summary is empty when there was nothing to explain; no model was asked then.
	Summary string `json:"summary"`
	Model   string `json:"model,omitempty"`
}

// Post handles POST /admin/api/netaudit/activity/summary?workspace_id=&automation=. The workspace is the audit's; its model
// writes the summary.
func (h *NetauditSummaryHandlers) Post(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	workspaceID := q.Get("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, msgSummaryNoSpace)
		return
	}
	activity, err := h.activity.build(q, netaudit.MaxActivityRows)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := h.activity.now()
	prompt := netaudit.SummaryPrompt(recentRows(activity.Rows, now), now)
	if prompt == "" {
		respondJSON(w, summaryResponse{})
		return
	}
	if !h.running.CompareAndSwap(false, true) {
		writeJSONError(w, http.StatusTooManyRequests, msgSummaryRunning)
		return
	}
	defer h.running.Store(false)

	ctx, cancel := context.WithTimeout(r.Context(), summaryTimeout)
	defer cancel()
	ctx, client, model, release, err := h.model.Lease(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, chat.ErrNotAdmitted) {
			writeJSONError(w, http.StatusServiceUnavailable, msgSummaryBusy)
			return
		}
		writeJSONError(w, http.StatusBadGateway, msgSummaryFailed)
		return
	}
	defer release()
	if !h.isLocal(model) {
		writeJSONError(w, http.StatusConflict, msgSummaryNotLocal)
		return
	}
	reply, err := chatCompleter{client: client, model: model, maxTokens: summaryMaxTokens, temperature: summaryTemperature}.Complete(ctx, prompts.NetworkTimelineSummaryPrompt, prompt)
	summary := netaudit.CleanSummary(reply)
	if err != nil || summary == "" {
		writeJSONError(w, http.StatusBadGateway, msgSummaryFailed)
		return
	}
	respondJSON(w, summaryResponse{Summary: summary, Model: model})
}

// recentRows keeps the rows of the last week, which is what "this week" means.
func recentRows(rows []netaudit.ActivityRow, now time.Time) []netaudit.ActivityRow {
	out := make([]netaudit.ActivityRow, 0, len(rows))
	for _, r := range rows {
		if now.Sub(r.At) <= summaryWindow {
			out = append(out, r)
		}
	}
	return out
}
