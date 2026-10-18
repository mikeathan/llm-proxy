package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/reminders"
)

type fakeReminderLister struct {
	open, recent []reminders.Reminder
	events       []reminders.Reminder // newest first, as the store returns them
	eventsAfter  time.Time            // what EventsAfter was last asked for
	edited       *reminders.Reminder
	editID       int64
	editErr      error
	removed      []int64
	removeErr    error
	created      *reminders.Reminder
	createErr    error
	acted        string // "pause 3", "resume 3", "skip 3"
	actErr       error
}

func (f *fakeReminderLister) Create(_ context.Context, in reminders.Reminder) (reminders.Reminder, error) {
	f.created = &in
	if f.createErr != nil {
		return reminders.Reminder{}, f.createErr
	}
	in.ID, in.Status = 11, reminders.StatusScheduled
	return in, nil
}

func (f *fakeReminderLister) act(what string, id int64) (reminders.Reminder, error) {
	f.acted = fmt.Sprintf("%s %d", what, id)
	if f.actErr != nil {
		return reminders.Reminder{}, f.actErr
	}
	return reminders.Reminder{ID: id, Message: "standup", Status: reminders.StatusPaused, Timezone: "UTC"}, nil
}
func (f *fakeReminderLister) Pause(_ context.Context, id int64) (reminders.Reminder, error) {
	return f.act("pause", id)
}
func (f *fakeReminderLister) Resume(_ context.Context, id int64) (reminders.Reminder, error) {
	return f.act("resume", id)
}
func (f *fakeReminderLister) Skip(_ context.Context, id int64) (reminders.Reminder, error) {
	return f.act("skip", id)
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

func (f *fakeReminderLister) EventsAfter(_ context.Context, after time.Time, limit int) ([]reminders.Reminder, error) {
	f.eventsAfter = after
	var out []reminders.Reminder
	for _, r := range f.events {
		if r.EventAt.After(after) && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeReminderLister) LatestEventAt(context.Context) (time.Time, error) {
	if len(f.events) == 0 {
		return time.Time{}, nil
	}
	return f.events[0].EventAt, nil
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
	h := NewReminderHandlers(testZone, &fakeReminderLister{
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
	NewReminderHandlers(testZone, &fakeReminderLister{}).List(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Body.String(); got != "{\"open\":[],\"recent\":[]}\n" {
		t.Errorf("body = %q, want empty arrays (the UI iterates them)", got)
	}
}

func testZone(requested string) (*time.Location, error) {
	if requested == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(requested)
}

func request(method, body, id string) *http.Request {
	req := httptest.NewRequest(method, "/admin/api/reminders/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	return req
}

func TestReminderHandlers_ListExposesWhatTheEditFormNeeds(t *testing.T) {
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	h := NewReminderHandlers(testZone, &fakeReminderLister{open: []reminders.Reminder{
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
	NewReminderHandlers(testZone, f).Edit(rec, request(http.MethodPut,
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
	NewReminderHandlers(testZone, f).Edit(rec, request(http.MethodPut, `{"message":"standup","cron":"0 9 * * 1","until":"2027-01-01T00:00:00Z"}`, "3"))
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
		NewReminderHandlers(testZone, &fakeReminderLister{editErr: tc.err}).Edit(rec, request(http.MethodPut, tc.body, tc.id))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestReminderHandlers_Delete(t *testing.T) {
	f := &fakeReminderLister{}
	rec := httptest.NewRecorder()
	NewReminderHandlers(testZone, f).Delete(rec, request(http.MethodDelete, "", "5"))
	if rec.Code != http.StatusNoContent || len(f.removed) != 1 || f.removed[0] != 5 {
		t.Fatalf("status=%d removed=%v", rec.Code, f.removed)
	}
	rec = httptest.NewRecorder()
	NewReminderHandlers(testZone, &fakeReminderLister{removeErr: reminders.ErrNotFound}).Delete(rec, request(http.MethodDelete, "", "5"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	NewReminderHandlers(testZone, f).Delete(rec, request(http.MethodDelete, "", "x"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d", rec.Code)
	}
}

func TestReminderHandlers_Create(t *testing.T) {
	f := &fakeReminderLister{}
	rec := httptest.NewRecorder()
	NewReminderHandlers(testZone, f).Create(rec, request(http.MethodPost,
		`{"message":"call the bank","fire_at":"2026-10-12T09:00:00+01:00","connector":"telegram","timezone":"Europe/London"}`, ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	got := f.created
	if got.Message != "call the bank" || got.Timezone != "Europe/London" || got.Connector != "telegram" || got.CreatedBy != "ui" ||
		!got.FireAt.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("created = %+v", got)
	}
	var item map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &item)
	if item["id"] != float64(11) {
		t.Errorf("response = %+v", item)
	}
}

func TestReminderHandlers_CreateRecurringUsesTheOwnerZoneWhenNoneGiven(t *testing.T) {
	f := &fakeReminderLister{}
	owner := func(requested string) (*time.Location, error) { return time.LoadLocation("Asia/Tokyo") }
	rec := httptest.NewRecorder()
	NewReminderHandlers(owner, f).Create(rec, request(http.MethodPost, `{"message":"standup","cron":"0 9 * * 1"}`, ""))
	if rec.Code != http.StatusCreated || f.created.Timezone != "Asia/Tokyo" || f.created.Cron != "0 9 * * 1" {
		t.Fatalf("status=%d created=%+v", rec.Code, f.created)
	}
}

func TestReminderHandlers_CreateErrors(t *testing.T) {
	cases := []struct {
		name, body string
		err        error
		want       int
	}{
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad time", `{"message":"x","fire_at":"tomorrow"}`, nil, http.StatusBadRequest},
		{"unknown timezone", `{"message":"x","fire_at":"2030-01-01T00:00:00Z","timezone":"Mars/Base"}`, nil, http.StatusBadRequest},
		{"refused by rules", `{"message":"x","fire_at":"2020-01-01T00:00:00Z"}`, fmt.Errorf("%w: past", reminders.ErrInvalid), http.StatusBadRequest},
		{"storage failure", `{"message":"x","fire_at":"2030-01-01T00:00:00Z"}`, errors.New("disk"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		NewReminderHandlers(testZone, &fakeReminderLister{createErr: tc.err}).Create(rec, request(http.MethodPost, tc.body, ""))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestReminderHandlers_PauseResumeSkip(t *testing.T) {
	f := &fakeReminderLister{}
	h := NewReminderHandlers(testZone, f)
	for _, tc := range []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{{"pause", h.Pause}, {"resume", h.Resume}, {"skip", h.Skip}} {
		rec := httptest.NewRecorder()
		tc.call(rec, request(http.MethodPost, "", "3"))
		if rec.Code != http.StatusOK || f.acted != tc.name+" 3" {
			t.Errorf("%s: status=%d acted=%q", tc.name, rec.Code, f.acted)
		}
	}
	for name, err := range map[string]error{"refused": fmt.Errorf("%w: only repeating", reminders.ErrInvalid), "unknown": reminders.ErrNotFound} {
		rec := httptest.NewRecorder()
		NewReminderHandlers(testZone, &fakeReminderLister{actErr: err}).Pause(rec, request(http.MethodPost, "", "3"))
		want := map[string]int{"refused": http.StatusBadRequest, "unknown": http.StatusNotFound}[name]
		if rec.Code != want {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	h.Pause(rec, request(http.MethodPost, "", "x"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d", rec.Code)
	}
}

type eventsBody struct {
	Cursor int64 `json:"cursor"`
	Items  []struct {
		ID      int64  `json:"id"`
		Message string `json:"message"`
		Event   string `json:"event"`
		At      int64  `json:"at"`
		Channel string `json:"channel"`
	} `json:"items"`
}

func getEvents(t *testing.T, h *ReminderHandlers, query string) eventsBody {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Events(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reminders/events"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var body eventsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// The web UI asks what happened after the newest event it has seen. With no cursor it gets nothing but the baseline to
// start from, so opening the app announces only what fires from then on; a later call returns what came after, newest
// first, and only what the web UI should be told.
func TestReminderHandlers_EventsAreFetchedByCursor(t *testing.T) {
	at := time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC)
	h := NewReminderHandlers(testZone, &fakeReminderLister{events: []reminders.Reminder{
		{ID: 3, Message: "stand up", Event: reminders.EventFailed, EventAt: at.Add(2 * time.Minute), Connector: "telegram"},
		{ID: 2, Message: "to telegram only", Event: reminders.EventDue, EventAt: at.Add(time.Minute), Connector: "telegram"},
		{ID: 1, Message: "call bank", Event: reminders.EventDue, EventAt: at},
	}})

	baseline := getEvents(t, h, "")
	if len(baseline.Items) != 0 || baseline.Cursor != at.Add(2*time.Minute).UnixMilli() {
		t.Fatalf("baseline = %+v, want no items and the newest event time", baseline)
	}
	t.Run("what came after the cursor, newest first, without a delivery that went elsewhere", func(t *testing.T) {
		got := getEvents(t, h, "?after="+strconv.FormatInt(at.Add(-time.Second).UnixMilli(), 10))
		if len(got.Items) != 2 || got.Items[0].ID != 3 || got.Items[1].ID != 1 {
			t.Fatalf("items = %+v, want the failure (3) then the web UI delivery (1); 2 went to telegram only", got.Items)
		}
		if got.Items[0].Event != "failed" || got.Items[0].At != at.Add(2*time.Minute).UnixMilli() || got.Items[0].Message != "stand up" {
			t.Errorf("item = %+v", got.Items[0])
		}
		if got.Cursor != at.Add(2*time.Minute).UnixMilli() {
			t.Errorf("cursor = %d, want the newest event time", got.Cursor)
		}
	})
	t.Run("nothing after the newest event keeps the cursor", func(t *testing.T) {
		newest := at.Add(2 * time.Minute).UnixMilli()
		got := getEvents(t, h, "?after="+strconv.FormatInt(newest, 10))
		if len(got.Items) != 0 || got.Cursor != newest {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a cursor that is not a number is refused", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Events(rec, httptest.NewRequest(http.MethodGet, "/admin/api/reminders/events?after=soon", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}
