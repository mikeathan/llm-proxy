package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/vulndata/ubuntuoval"
)

type fakeUbuntuData struct {
	status         ubuntuoval.Status
	updateErr      error
	clearErr       error
	updates        int
	cleared        bool
	ctxHadDeadline bool
}

func (f *fakeUbuntuData) Status() ubuntuoval.Status { return f.status }
func (f *fakeUbuntuData) Update(ctx context.Context) error {
	f.updates++
	_, f.ctxHadDeadline = ctx.Deadline()
	return f.updateErr
}
func (f *fakeUbuntuData) Clear() error { f.cleared = true; return f.clearErr }

func callUbuntu(h http.HandlerFunc, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/admin/api/netaudit/ubuntu-data", nil))
	return rec
}

func TestUbuntuDataHandlers_GetReportsWhatIsHeld(t *testing.T) {
	f := &fakeUbuntuData{status: ubuntuoval.Status{Supported: true, HostRelease: "noble", Release: "noble", Notices: 981, Packages: 2218, ImportedAt: time.Now()}}
	rec := callUbuntu(NewUbuntuDataHandlers(f).Get, http.MethodGet)
	var got ubuntuoval.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || got.Notices != 981 || !got.Supported {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}
}

func TestUbuntuDataHandlers_UpdateRunsOnceWithATimeLimitAndAnswersWithTheStatus(t *testing.T) {
	f := &fakeUbuntuData{status: ubuntuoval.Status{Supported: true, Notices: 5}}
	rec := callUbuntu(NewUbuntuDataHandlers(f).Update, http.MethodPost)
	if rec.Code != 200 || f.updates != 1 || !f.ctxHadDeadline || !strings.Contains(rec.Body.String(), `"notices":5`) {
		t.Errorf("status %d, updates %d, deadline %v, body %s", rec.Code, f.updates, f.ctxHadDeadline, rec.Body)
	}
}

func TestUbuntuDataHandlers_UpdateSaysWhyInFixedWordsAndNeverTheErrorText(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
	}{
		"busy":       {ubuntuoval.ErrBusy, http.StatusConflict},
		"not ubuntu": {ubuntuoval.ErrNotUbuntu, http.StatusBadRequest},
		"failed":     {errors.New("dial tcp 10.1.2.3:443: secret detail"), http.StatusBadGateway},
	}
	for name, c := range cases {
		f := &fakeUbuntuData{updateErr: c.err, status: ubuntuoval.Status{Reason: "The notices could not be downloaded."}}
		rec := callUbuntu(NewUbuntuDataHandlers(f).Update, http.MethodPost)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", name, rec.Code, c.status)
		}
		if strings.Contains(rec.Body.String(), "secret detail") || strings.Contains(rec.Body.String(), "10.1.2.3") {
			t.Errorf("%s: the error text reached the answer: %s", name, rec.Body)
		}
	}
}

func TestUbuntuDataHandlers_ClearDeletesAndAConflictIsStated(t *testing.T) {
	f := &fakeUbuntuData{}
	if rec := callUbuntu(NewUbuntuDataHandlers(f).Clear, http.MethodDelete); rec.Code != 200 || !f.cleared {
		t.Errorf("status %d cleared %v", rec.Code, f.cleared)
	}
	busy := &fakeUbuntuData{clearErr: ubuntuoval.ErrBusy}
	if rec := callUbuntu(NewUbuntuDataHandlers(busy).Clear, http.MethodDelete); rec.Code != http.StatusConflict {
		t.Errorf("status %d", rec.Code)
	}
	broken := &fakeUbuntuData{clearErr: errors.New("disk")}
	if rec := callUbuntu(NewUbuntuDataHandlers(broken).Clear, http.MethodDelete); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d", rec.Code)
	}
}
