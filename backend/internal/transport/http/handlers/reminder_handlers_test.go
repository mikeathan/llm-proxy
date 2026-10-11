package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/reminders"
)

type fakeReminderLister struct {
	open, recent []reminders.Reminder
	edited       *reminders.Reminder
	editID       int64
	editErr      error
	removed      []int64
	removeErr    error
}

func (f *fakeReminderLister) Edit(_ context.Context, id int64, in reminders.Reminder) (reminders.Reminder, error) {
	f.editID, f.edited = id, &in
	if f.editErr != nil {
		return reminders.Reminder{}, f.editErr
	}
	in.ID, in.Status, in.Timezone = id, reminders.StatusScheduled, "Europe/London"
	return in, nil
}

func (f *fakeReminderLister) Remove(_ context.Context, id int64) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeReminderLister) List(context.Context) ([]reminders.Reminder, error) { return f.open, nil }
func (f *fakeReminderLister) Recent(context.Context, int) ([]reminders.Reminder, error) {
	return f.recent, nil
}
func (f *fakeReminderLister) Channel(c string) string {
	if c == "" {
		return "telegram"
	}
	return c
}

func TestReminderHandlers_ListReturnsOpenAndRecent(t *testing.T) {
	at := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	h := NewReminderHandlers(&fakeReminderLister{
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
	NewReminderHandlers(&fakeReminderLister{}).List(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Body.String(); got != "{\"open\":[],\"recent\":[]}\n" {
		t.Errorf("body = %q, want empty arrays (the UI iterates them)", got)
	}
}

func request(method, body, id string) *http.Request {
	req := httptest.NewRequest(method, "/admin/api/reminders/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	return req
}

func TestReminderHandlers_ListExposesWhatTheEditFormNeeds(t *testing.T) {
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	h := NewReminderHandlers(&fakeReminderLister{open: []reminders.Reminder{
		{ID: 1, Message: "x", FireAt: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC), Cron: "0 9 * * 1", Until: &until, Connector: "", Status: reminders.StatusScheduled},
	}})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var body struct {
		Open []map[string]any `json:"open"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	got := body.Open[0]
	if got["until"] != "2027-01-01T00:00:00Z" || got["connector"] != "" || got["channel"] != "telegram" {
		t.Errorf("item = %+v, want until, the raw connector (empty = automatic) and the effective channel", got)
	}
}

func TestReminderHandlers_EditAppliesTheForm(t *testing.T) {
	f := &fakeReminderLister{}
	rec := httptest.NewRecorder()
	NewReminderHandlers(f).Edit(rec, request(http.MethodPut,
		`{"message":"call the bank","fire_at":"2026-10-12T09:00:00+01:00","connector":"telegram"}`, "7"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if f.editID != 7 || f.edited.Message != "call the bank" || f.edited.Connector != "telegram" ||
		!f.edited.FireAt.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("edit = %d %+v", f.editID, f.edited)
	}
	var item map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &item)
	if item["id"] != float64(7) || item["message"] != "call the bank" {
		t.Errorf("response = %+v", item)
	}
}

func TestReminderHandlers_EditRecurringWithEnd(t *testing.T) {
	f := &fakeReminderLister{}
	rec := httptest.NewRecorder()
	NewReminderHandlers(f).Edit(rec, request(http.MethodPut, `{"message":"standup","cron":"0 9 * * 1","until":"2027-01-01T00:00:00Z"}`, "3"))
	if rec.Code != http.StatusOK || f.edited.Cron != "0 9 * * 1" || f.edited.Until == nil || !f.edited.FireAt.IsZero() {
		t.Fatalf("status=%d edited=%+v", rec.Code, f.edited)
	}
}

func TestReminderHandlers_EditErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		id   string
		err  error
		want int
	}{
		{"bad id", `{"message":"x"}`, "abc", nil, http.StatusBadRequest},
		{"bad json", `{`, "1", nil, http.StatusBadRequest},
		{"bad time", `{"message":"x","fire_at":"tomorrow"}`, "1", nil, http.StatusBadRequest},
		{"bad until", `{"message":"x","cron":"0 9 * * *","until":"soon"}`, "1", nil, http.StatusBadRequest},
		{"refused by rules", `{"message":"x","fire_at":"2020-01-01T00:00:00Z"}`, "1", fmt.Errorf("%w: in the past", reminders.ErrInvalid), http.StatusBadRequest},
		{"unknown", `{"message":"x","fire_at":"2030-01-01T00:00:00Z"}`, "1", reminders.ErrNotFound, http.StatusNotFound},
		{"storage failure", `{"message":"x","fire_at":"2030-01-01T00:00:00Z"}`, "1", errors.New("disk"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		NewReminderHandlers(&fakeReminderLister{editErr: tc.err}).Edit(rec, request(http.MethodPut, tc.body, tc.id))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestReminderHandlers_Delete(t *testing.T) {
	f := &fakeReminderLister{}
	rec := httptest.NewRecorder()
	NewReminderHandlers(f).Delete(rec, request(http.MethodDelete, "", "5"))
	if rec.Code != http.StatusNoContent || len(f.removed) != 1 || f.removed[0] != 5 {
		t.Fatalf("status=%d removed=%v", rec.Code, f.removed)
	}
	rec = httptest.NewRecorder()
	NewReminderHandlers(&fakeReminderLister{removeErr: reminders.ErrNotFound}).Delete(rec, request(http.MethodDelete, "", "5"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	NewReminderHandlers(f).Delete(rec, request(http.MethodDelete, "", "x"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d", rec.Code)
	}
}
