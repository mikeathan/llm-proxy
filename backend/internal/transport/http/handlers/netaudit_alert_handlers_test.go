package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeAlerts struct {
	items []models.NetworkAlert // newest first
}

func (f fakeAlerts) After(id int64, limit int) ([]models.NetworkAlert, error) {
	out := []models.NetworkAlert{}
	for _, a := range f.items {
		if a.ID > id && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f fakeAlerts) Latest() (int64, error) {
	if len(f.items) == 0 {
		return 0, nil
	}
	return f.items[0].ID, nil
}

func alertFixture() fakeAlerts {
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	return fakeAlerts{items: []models.NetworkAlert{
		{ID: 30, Kind: models.NetworkAlertDecoy, Message: "decoy m", At: at, Delivery: models.NetworkAlertNoConnector, Count: 3},
		{ID: 20, Kind: models.NetworkAlertPresence, Message: "presence m", At: at, Delivery: models.NetworkAlertSent, Count: 1},
		{ID: 10, Kind: models.NetworkAlertSummary, Message: "summary m", At: at, Delivery: models.NetworkAlertFailed, Count: 1},
	}}
}

func TestAlertHandlers_Events(t *testing.T) {
	h := NewNetauditAlertHandlers(alertFixture())
	t.Run("without a cursor it returns only where the log stands, so a fresh page announces nothing old", func(t *testing.T) {
		rec := do(h.Events, http.MethodGet, "/admin/api/netaudit/alerts", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cursor":30`) || !strings.Contains(rec.Body.String(), `"items":[]`) {
			t.Errorf("got %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("with a cursor it returns newer entries newest first with the delivery and count", func(t *testing.T) {
		rec := do(h.Events, http.MethodGet, "/admin/api/netaudit/alerts?after=10", "")
		body := rec.Body.String()
		for _, want := range []string{`"cursor":30`, `"id":30`, `"kind":"decoy"`, `"delivery":"no_connector"`, `"count":3`, `"id":20`, `"message":"presence m"`} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %q in %s", want, body)
			}
		}
		if strings.Contains(body, `"id":10`) {
			t.Errorf("the entry at the cursor is not new: %s", body)
		}
	})
	t.Run("limit bounds the answer", func(t *testing.T) {
		rec := do(h.Events, http.MethodGet, "/admin/api/netaudit/alerts?after=0&limit=1", "")
		if strings.Contains(rec.Body.String(), `"id":20`) || !strings.Contains(rec.Body.String(), `"id":30`) {
			t.Errorf("got %s", rec.Body.String())
		}
	})
	t.Run("a bad cursor or limit is a 400", func(t *testing.T) {
		for _, q := range []string{"after=x", "after=-1", "after=1&limit=0", "after=1&limit=abc"} {
			if rec := do(h.Events, http.MethodGet, "/admin/api/netaudit/alerts?"+q, ""); rec.Code != http.StatusBadRequest {
				t.Errorf("%s = %d", q, rec.Code)
			}
		}
	})
}
