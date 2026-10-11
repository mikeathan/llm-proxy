package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/platform/logging"
)

// recentReminderLimit bounds the finished-reminder list shown in the UI.
const recentReminderLimit = 20

// ReminderManager is what the reminder endpoints use of the service.
type ReminderManager interface {
	List(ctx context.Context) ([]reminders.Reminder, error)
	Recent(ctx context.Context, limit int) ([]reminders.Reminder, error)
	Channel(connector string) string
	Edit(ctx context.Context, id int64, in reminders.Reminder) (reminders.Reminder, error)
	Remove(ctx context.Context, id int64) error
}

// ReminderHandlers serves the reminder list (open and recently finished), which
// is the in-app notice of what was delivered, missed or failed.
type ReminderHandlers struct {
	svc ReminderManager
}

func NewReminderHandlers(svc ReminderManager) *ReminderHandlers { return &ReminderHandlers{svc: svc} }

type reminderView struct {
	ID       int64  `json:"id"`
	Message  string `json:"message"`
	FireAt   string `json:"fire_at"`
	Cron     string `json:"cron,omitempty"`
	Until    string `json:"until,omitempty"`
	Timezone string `json:"timezone"`
	// Connector is the channel as chosen ("" = automatic, "in-app", or a connector
	// type); Channel is where it will actually go.
	Connector string `json:"connector"`
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
		out = append(out, h.view(r))
	}
	return out
}

func (h *ReminderHandlers) view(r reminders.Reminder) reminderView {
	v := reminderView{
		ID: r.ID, Message: r.Message, FireAt: r.FireAt.UTC().Format(time.RFC3339), Cron: r.Cron,
		Timezone: r.Timezone, Connector: r.Connector, Channel: h.svc.Channel(r.Connector),
		Status: string(r.Status), LastError: r.LastError,
	}
	if r.Until != nil {
		v.Until = r.Until.UTC().Format(time.RFC3339)
	}
	return v
}

// editRequest is the edit form: the whole editable state of a reminder. Give
// fire_at for a one-off or cron (with optional until) for a recurring one.
type editRequest struct {
	Message   string `json:"message"`
	FireAt    string `json:"fire_at"`
	Cron      string `json:"cron"`
	Until     string `json:"until"`
	Connector string `json:"connector"`
}

// Edit handles PUT /admin/api/reminders/{id}.
func (h *ReminderHandlers) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := reminderID(w, r)
	if !ok {
		return
	}
	var req editRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	in, err := req.toReminder()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	got, err := h.svc.Edit(r.Context(), id, in)
	if !h.respondFailure(w, err, "edit") {
		return
	}
	respondJSON(w, h.view(got))
}

// Delete handles DELETE /admin/api/reminders/{id}.
func (h *ReminderHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := reminderID(w, r)
	if !ok {
		return
	}
	if !h.respondFailure(w, h.svc.Remove(r.Context(), id), "delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondFailure writes the status for err and reports whether the call succeeded.
func (h *ReminderHandlers) respondFailure(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, reminders.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "reminder not found")
	case errors.Is(err, reminders.ErrInvalid):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		logging.Warn("reminder "+what+" failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to "+what+" reminder")
	}
	return false
}

func (e editRequest) toReminder() (reminders.Reminder, error) {
	in := reminders.Reminder{Message: e.Message, Cron: e.Cron, Connector: e.Connector}
	var err error
	if e.FireAt != "" {
		if in.FireAt, err = parseReminderTime("fire_at", e.FireAt); err != nil {
			return in, err
		}
	}
	if e.Until != "" {
		until, err := parseReminderTime("until", e.Until)
		if err != nil {
			return in, err
		}
		in.Until = &until
	}
	return in, nil
}

func parseReminderTime(field, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(field + " must be RFC 3339, for example 2026-10-12T09:00:00+01:00")
	}
	return t.UTC(), nil
}

func reminderID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	vals, ok := requirePathParams(w, r, "id")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(vals[0], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
