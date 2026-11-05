package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/nuclei"
)

type fakeWebTemplates struct {
	ok       bool
	reason   string
	st       nuclei.Status
	err      error
	download int
}

func (f *fakeWebTemplates) Ready() (bool, string) { return f.ok, f.reason }
func (f *fakeWebTemplates) Status() nuclei.Status { return f.st }
func (f *fakeWebTemplates) Download(context.Context) error {
	f.download++
	return f.err
}

func TestWebTemplatesHandlers_GetSaysWhatIsPresentAndWhyTheChecksCannotRun(t *testing.T) {
	f := &fakeWebTemplates{reason: "nuclei is not installed", st: nuclei.Status{Present: 0, Total: 24}}
	h := NewNetauditWebTemplatesHandlers(f)
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/web-templates", "")
	body := rec.Body.String()
	for _, want := range []string{`"present":0`, `"total":24`, `"ready":false`, "nuclei is not installed", `"tag":"` + nuclei.ReviewedTag + `"`} {
		if rec.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, body)
		}
	}
	if f.download != 0 {
		t.Fatal("reading the status must not download anything")
	}
}

func TestWebTemplatesHandlers_DownloadFetchesOnRequestAndAnswersWithTheNewStatus(t *testing.T) {
	f := &fakeWebTemplates{ok: true, st: nuclei.Status{Present: 24, Total: 24}}
	h := NewNetauditWebTemplatesHandlers(f)
	rec := do(h.Download, http.MethodPost, "/admin/api/netaudit/web-templates/download", "")
	if rec.Code != http.StatusOK || f.download != 1 || !strings.Contains(rec.Body.String(), `"ready":true`) {
		t.Fatalf("download = %d %s (%d calls)", rec.Code, rec.Body.String(), f.download)
	}
	f.err = errors.New("nuclei: these templates could not be downloaded as reviewed: a (it is not the file that was reviewed)")
	rec = do(h.Download, http.MethodPost, "/admin/api/netaudit/web-templates/download", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "not the file that was reviewed") {
		t.Fatalf("failed download = %d %s", rec.Code, rec.Body.String())
	}
}
