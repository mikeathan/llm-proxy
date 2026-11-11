package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
)

type fakeRecheck struct {
	res netaudit.RecheckResult
	err error
	got [3]string
}

func (f *fakeRecheck) Check(_ context.Context, ws, automation, key string) (netaudit.RecheckResult, error) {
	f.got = [3]string{ws, automation, key}
	return f.res, f.err
}

func TestRecheckFinding_AnswersTheOutcome(t *testing.T) {
	f := &fakeRecheck{res: netaudit.RecheckResult{Outcome: netaudit.RecheckFixed, Reason: "gone"}}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithRecheck(f)
	rec := do(h.RecheckFinding, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","key":"k"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"outcome":"fixed"`) || f.got != [3]string{"ws", "a", "k"} {
		t.Errorf("%d %s %v", rec.Code, rec.Body.String(), f.got)
	}
}

func TestRecheckFinding_StatusOfEachRefusal(t *testing.T) {
	cases := map[error]int{
		netaudit.ErrNoSuchFinding:  http.StatusNotFound,
		netaudit.ErrScanBusy:       http.StatusConflict,
		netaudit.ErrUseDeepCheck:   http.StatusBadRequest,
		netaudit.ErrNotRecheckable: http.StatusBadRequest,
		errors.New("disk"):         http.StatusInternalServerError,
	}
	for err, want := range cases {
		h := NewNetauditHandlers(nil, nil, nil, nil).WithRecheck(&fakeRecheck{err: err})
		if rec := do(h.RecheckFinding, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","key":"k"}`); rec.Code != want {
			t.Errorf("%v = %d, want %d", err, rec.Code, want)
		}
	}
}

func TestRecheckFinding_NeedsTheFieldsAndTheService(t *testing.T) {
	h := NewNetauditHandlers(nil, nil, nil, nil)
	if rec := do(h.RecheckFinding, http.MethodPost, "/", `{"workspace_id":"ws"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing fields = %d", rec.Code)
	}
	if rec := do(h.RecheckFinding, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","key":"k"}`); rec.Code != http.StatusNotFound {
		t.Errorf("no service = %d", rec.Code)
	}
}
