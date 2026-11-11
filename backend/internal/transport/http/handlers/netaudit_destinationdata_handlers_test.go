package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/ipowners"
)

type fakeOwners struct {
	status   ipowners.Status
	err      error
	download int
	removed  int
}

func (f *fakeOwners) Status() ipowners.Status { return f.status }
func (f *fakeOwners) Download(context.Context) error {
	f.download++
	if f.err == nil {
		f.status = ipowners.Status{Ranges: 579453, LoadedAt: time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)}
	}
	return f.err
}
func (f *fakeOwners) Remove() error { f.removed++; f.status = ipowners.Status{}; return nil }

func TestDestinationDataSaysWhatIsHeldAndWhereItComesFrom(t *testing.T) {
	h := NewNetauditDestinationDataHandlers(&fakeOwners{status: ipowners.Status{Ranges: 12}}, "iptoasn.com")
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/destination-data", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"ranges":12`) || !strings.Contains(body, `"source":"iptoasn.com"`) {
		t.Fatalf("%d %s", rec.Code, body)
	}
}

func TestDestinationDataDownloadAnswersWithTheNewStatusAndAFailureIsAGatewayError(t *testing.T) {
	f := &fakeOwners{}
	h := NewNetauditDestinationDataHandlers(f, "iptoasn.com")
	if rec := do(h.Download, http.MethodPost, "/admin/api/netaudit/destination-data/download", ""); rec.Code != http.StatusOK || f.download != 1 || !strings.Contains(rec.Body.String(), `"ranges":579453`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	f.err = errors.New("iptoasn.com answered 503")
	rec := do(h.Download, http.MethodPost, "/x", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "503") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestDestinationDataRemoveForgetsTheTable(t *testing.T) {
	f := &fakeOwners{status: ipowners.Status{Ranges: 5}}
	h := NewNetauditDestinationDataHandlers(f, "iptoasn.com")
	if rec := do(h.Delete, http.MethodDelete, "/x", ""); rec.Code != http.StatusOK || f.removed != 1 || !strings.Contains(rec.Body.String(), `"ranges":0`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
