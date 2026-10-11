package handlers

import (
	"context"
	"net/http"
	"time"

	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/platform/logging"
)

// recentReminderLimit bounds the finished-reminder list shown in the UI.
const recentReminderLimit = 20

// ReminderLister is what the reminders endpoint reads from the service.
type ReminderLister interface {
	List(ctx context.Context) ([]reminders.Reminder, error)
	Recent(ctx context.Context, limit int) ([]reminders.Reminder, error)
	Channel(connector string) string
}

// ReminderHandlers serves the reminder list (open and recently finished), which
// is the in-app notice of what was delivered, missed or failed.
type ReminderHandlers struct {
	svc ReminderLister
}

func NewReminderHandlers(svc ReminderLister) *ReminderHandlers { return &ReminderHandlers{svc: svc} }

type reminderView struct {
	ID        int64  `json:"id"`
	Message   string `json:"message"`
	FireAt    string `json:"fire_at"`
	Cron      string `json:"cron,omitempty"`
	Timezone  string `json:"timezone"`
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
}

// List handles GET /admin/api/reminders.
func (h *ReminderHandlers) List(w http.ResponseWriter, r *http.Request) {
	open, err := h.svc.List(r.Context())
	if err != nil {
		logging.Warn("reminders list failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list reminders")
		return
	}
	recent, err := h.svc.Recent(r.Context(), recentReminderLimit)
	if err != nil {
		logging.Warn("recent reminders failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list reminders")
		return
	}
	respondJSON(w, map[string][]reminderView{"open": h.views(open), "recent": h.views(recent)})
}

func (h *ReminderHandlers) views(rs []reminders.Reminder) []reminderView {
	out := make([]reminderView, 0, len(rs))
	for _, r := range rs {
		out = append(out, reminderView{
			ID: r.ID, Message: r.Message, FireAt: r.FireAt.UTC().Format(time.RFC3339), Cron: r.Cron,
			Timezone: r.Timezone, Channel: h.svc.Channel(r.Connector), Status: string(r.Status), LastError: r.LastError,
		})
	}
	return out
}
