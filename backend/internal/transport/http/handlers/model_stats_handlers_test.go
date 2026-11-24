package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/platform/modelstats"
)

func TestModelStatsHandler_AnswersTheReportForTheDaysAsked(t *testing.T) {
	var asked int
	h := NewModelStatsHandlers(func(days int) modelstats.Report {
		asked = days
		return modelstats.Report{Days: []modelstats.DayTotal{{Date: "2026-11-24", Local: 3, Cloud: 1}}, Today: []modelstats.ModelUse{{Model: "a", Local: true, Calls: 3}}}
	})
	for query, want := range map[string]int{"": 7, "?days=14": 14, "?days=2": 2} {
		rec := httptest.NewRecorder()
		h.AdminModelStatsHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/api/model-stats"+query, nil))
		var got modelstats.Report
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Days[0].Local != 3 || got.Today[0].Model != "a" {
			t.Fatalf("%q: %d %s", query, rec.Code, rec.Body)
		}
		if asked != want {
			t.Errorf("%q asked for %d days, want %d", query, asked, want)
		}
	}
}

func TestModelStatsHandler_RefusesADaysValueThatIsNotAPositiveNumber(t *testing.T) {
	h := NewModelStatsHandlers(func(int) modelstats.Report { t.Fatal("asked for a report"); return modelstats.Report{} })
	for _, q := range []string{"?days=0", "?days=-3", "?days=abc"} {
		rec := httptest.NewRecorder()
		h.AdminModelStatsHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/api/model-stats"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, rec.Code)
		}
	}
}
