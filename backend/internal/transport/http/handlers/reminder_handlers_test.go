package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/internal/core/reminders"
)

type fakeReminderLister struct{ open, recent []reminders.Reminder }

func (f fakeReminderLister) List(context.Context) ([]reminders.Reminder, error) { return f.open, nil }
func (f fakeReminderLister) Recent(context.Context, int) ([]reminders.Reminder, error) {
	return f.recent, nil
}
func (f fakeReminderLister) Channel(c string) string {
	if c == "" {
		return "telegram"
	}
	return c
}

func TestReminderHandlers_ListReturnsOpenAndRecent(t *testing.T) {
	at := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	h := NewReminderHandlers(fakeReminderLister{
		open:   []reminders.Reminder{{ID: 1, Message: "call bank", FireAt: at, Timezone: "Europe/London", Status: reminders.StatusScheduled}},
		recent: []reminders.Reminder{{ID: 2, Message: "old", FireAt: at.Add(-time.Hour), Status: reminders.StatusFailed, LastError: "connector down"}},
	})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reminders", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Open   []map[string]any `json:"open"`
		Recent []map[string]any `json:"recent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Open) != 1 || body.Open[0]["message"] != "call bank" || body.Open[0]["channel"] != "telegram" ||
		body.Open[0]["fire_at"] != "2026-10-12T08:00:00Z" {
		t.Errorf("open = %+v", body.Open)
	}
	if len(body.Recent) != 1 || body.Recent[0]["status"] != "failed" || body.Recent[0]["last_error"] != "connector down" {
		t.Errorf("recent = %+v", body.Recent)
	}
}

func TestReminderHandlers_EmptyListsAreArrays(t *testing.T) {
	rec := httptest.NewRecorder()
	NewReminderHandlers(fakeReminderLister{}).List(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Body.String(); got != "{\"open\":[],\"recent\":[]}\n" {
		t.Errorf("body = %q, want empty arrays (the UI iterates them)", got)
	}
}
