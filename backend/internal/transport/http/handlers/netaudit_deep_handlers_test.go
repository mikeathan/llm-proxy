package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

type fakeDeep struct {
	check   models.DeepCheck
	err     error
	listed  []models.DeepCheck
	gotWS   string
	gotAuto string
	gotDev  string
	calls   int
}

func (f *fakeDeep) Run(_ context.Context, ws, automation, device string) (models.DeepCheck, error) {
	f.calls++
	f.gotWS, f.gotAuto, f.gotDev = ws, automation, device
	return f.check, f.err
}
func (f *fakeDeep) List(string) ([]models.DeepCheck, error) { return f.listed, f.err }

func TestDeepHandler_RunsOneDeviceAndAnswersWithTheCheckAndItsGuides(t *testing.T) {
	deep := &fakeDeep{check: models.DeepCheck{
		Device: "10.0.0.5", At: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Ports:    []models.DeepPortResult{{Port: 22, SSH: true}, {Port: 3389}},
		Findings: []models.ScanFinding{{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: netaudit.CheckSSHPasswordLogin, Severity: "medium", Evidence: "The SSH server accepts password login"}},
	}}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithDeepChecks(deep)
	rec := do(h.RunDeepCheck, http.MethodPost, "/admin/api/netaudit/deep", `{"workspace_id":"ws","automation":"nightly","device":"10.0.0.5"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if deep.gotWS != "ws" || deep.gotAuto != "nightly" || deep.gotDev != "10.0.0.5" {
		t.Errorf("the request names a workspace, an audit and a device: %s %s %s", deep.gotWS, deep.gotAuto, deep.gotDev)
	}
	var view struct {
		Device   string `json:"device"`
		Ports    []models.DeepPortResult
		Findings []struct {
			Check      string `json:"check"`
			CheckTitle string `json:"check_title"`
			Fix        string `json:"fix"`
			Guide      *struct {
				Steps []string `json:"steps"`
			} `json:"guide"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Device != "10.0.0.5" || len(view.Findings) != 1 || view.Findings[0].CheckTitle == "" || view.Findings[0].Fix == "" || view.Findings[0].Guide == nil || len(view.Findings[0].Guide.Steps) == 0 {
		t.Errorf("each finding carries its title, fix and guide: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"tls_answered":false`) || !strings.Contains(rec.Body.String(), `"ssh_answered":true`) {
		t.Errorf("which services answered is in the answer: %s", rec.Body.String())
	}
}

func TestDeepHandler_TakesNothingThatSteersTheScan(t *testing.T) {
	deep := &fakeDeep{}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithDeepChecks(deep)
	for _, body := range []string{`{"workspace_id":"ws","device":"10.0.0.5"}`, `{"automation":"a","device":"10.0.0.5"}`, `{"workspace_id":"ws","automation":"a"}`, `{bad`} {
		if rec := do(h.RunDeepCheck, http.MethodPost, "/", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
	}
	// Unknown fields are not an error but are never read: there is nowhere for a port list, a script or a flag to go.
	do(h.RunDeepCheck, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","device":"10.0.0.5","ports":"1-65535","scripts":"vuln","args":"-A"}`)
	if deep.calls != 1 || deep.gotDev != "10.0.0.5" {
		t.Errorf("calls %d device %q", deep.calls, deep.gotDev)
	}
}

func TestDeepHandler_RefusalsSayWhy(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{netaudit.ErrBadDevice, http.StatusBadRequest},
		{netaudit.ErrBadPorts, http.StatusBadRequest},
		{netaudit.ErrNoPorts, http.StatusConflict},
		{netaudit.ErrScanBusy, http.StatusConflict},
		{netaudit.ErrIntrusiveRefused, http.StatusForbidden},
		{scanscope.ErrOutOfScope, http.StatusBadRequest},
		{scanscope.ErrScanningOff, http.StatusServiceUnavailable},
		{netaudit.ErrNmapRequired, http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{errors.New("disk full"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		deep := &fakeDeep{err: c.err}
		rec := do(NewNetauditHandlers(nil, nil, nil, nil).WithDeepChecks(deep).RunDeepCheck, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","device":"10.0.0.5"}`)
		if rec.Code != c.want {
			t.Errorf("%v = %d, want %d", c.err, rec.Code, c.want)
		}
	}
	cool := &fakeDeep{err: &netaudit.CooldownError{Remaining: 4*time.Minute + 10*time.Second}}
	rec := do(NewNetauditHandlers(nil, nil, nil, nil).WithDeepChecks(cool).RunDeepCheck, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","device":"10.0.0.5"}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "250" {
		t.Errorf("a cool-down = %d, Retry-After %q (seconds left)", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := do(NewNetauditHandlers(nil, nil, nil, nil).RunDeepCheck, http.MethodPost, "/", `{"workspace_id":"ws","automation":"a","device":"10.0.0.5"}`); rec.Code != http.StatusNotFound {
		t.Errorf("without the service the endpoint is not there: %d", rec.Code)
	}
}

func TestDeepHandler_ListsTheStoredChecks(t *testing.T) {
	h := NewNetauditHandlers(nil, nil, nil, nil).WithDeepChecks(&fakeDeep{})
	rec := do(h.ListDeepChecks, http.MethodGet, "/admin/api/netaudit/deep?workspace_id=ws", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("none yet is an empty array: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(h.ListDeepChecks, http.MethodGet, "/admin/api/netaudit/deep", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a missing workspace = %d", rec.Code)
	}
}
