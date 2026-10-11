package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func previewSchedule(t *testing.T, body string) (*httptest.ResponseRecorder, models.SchedulePreview) {
	t.Helper()
	h := NewDispatcherHandlers(&testDispatcher{}, nil, logging.NewNopLogger())
	req := httptest.NewRequest(http.MethodPost, "/admin/api/dispatcher/schedule/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.PreviewSchedule(rr, req)
	var got models.SchedulePreview
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
	}
	return rr, got
}

func TestPreviewSchedule_ListsTheNextRuns(t *testing.T) {
	rr, got := previewSchedule(t, `{"trigger":{"type":"cron","value":"0 7 * * 1-5"},"count":3}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if !got.Valid || len(got.NextRuns) != 3 || got.Timezone == "" {
		t.Errorf("preview = %+v", got)
	}
}

// An invalid schedule is an answer, not a failure: the form shows the reason
// while the user is still typing.
func TestPreviewSchedule_InvalidScheduleIsOKWithAReason(t *testing.T) {
	rr, got := previewSchedule(t, `{"trigger":{"type":"cron","value":"*/10 * * * * *"}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got.Valid || got.Error == "" || len(got.NextRuns) != 0 {
		t.Errorf("preview = %+v", got)
	}
}

func TestPreviewSchedule_RejectsAMalformedBody(t *testing.T) {
	rr, _ := previewSchedule(t, `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
