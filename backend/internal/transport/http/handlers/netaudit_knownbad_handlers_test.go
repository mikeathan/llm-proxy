package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/knownbad"
)

type fakeKnownBad struct {
	st       knownbad.Status
	address  string
	ready    bool
	reason   string
	err      error
	download int
}

func (f *fakeKnownBad) Status() knownbad.Status { return f.st }
func (f *fakeKnownBad) HasAddress() bool        { return f.address != "" }
func (f *fakeKnownBad) SetAddress(a string) error {
	f.address = a
	return f.err
}
func (f *fakeKnownBad) ClearAddress() error { f.address = ""; return f.err }
func (f *fakeKnownBad) Download(context.Context) error {
	f.download++
	return f.err
}
func (f *fakeKnownBad) Ready() (bool, string) { return f.ready, f.reason }

const kbAddress = "https://urlhaus-api.abuse.ch/v2/files/exports/0123456789abcdef/hostfile.txt"

func TestKnownBadHandlers_GetNeverReturnsTheAddressAndSaysWhatIsMissing(t *testing.T) {
	f := &fakeKnownBad{address: kbAddress, reason: "the list is not downloaded yet"}
	rec := do(NewNetauditKnownBadHandlers(f).Get, http.MethodGet, "/admin/api/netaudit/known-bad", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"has_address":true`) || !strings.Contains(body, "not downloaded yet") {
		t.Fatalf("get = %d %s", rec.Code, body)
	}
	if strings.Contains(body, "0123456789abcdef") || strings.Contains(body, "urlhaus-api") {
		t.Fatalf("the saved address leaked into the answer: %s", body)
	}
}

func TestKnownBadHandlers_PutAddressSavesAValidAddressAndRefusesTheRest(t *testing.T) {
	f := &fakeKnownBad{}
	h := NewNetauditKnownBadHandlers(f)
	rec := do(h.PutAddress, http.MethodPut, "/admin/api/netaudit/known-bad/key", `{"address":"  `+kbAddress+` "}`)
	if rec.Code != http.StatusOK || f.address != kbAddress || strings.Contains(rec.Body.String(), "0123456789abcdef") {
		t.Fatalf("put = %d %s saved %q", rec.Code, rec.Body.String(), f.address)
	}
	for _, body := range []string{`{"address":"http://urlhaus-api.abuse.ch/x"}`, `{"address":"https://evil.example/x"}`, `{"address":""}`, `{}`} {
		f.address = ""
		rec := do(h.PutAddress, http.MethodPut, "/admin/api/netaudit/known-bad/key", body)
		if rec.Code != http.StatusBadRequest || f.address != "" {
			t.Errorf("%s = %d, saved %q", body, rec.Code, f.address)
		}
		if strings.Contains(rec.Body.String(), "evil.example") {
			t.Errorf("an error repeated the address: %s", rec.Body.String())
		}
	}
	f.err = errors.New("disk full")
	if rec := do(h.PutAddress, http.MethodPut, "/", `{"address":"`+kbAddress+`"}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("a failed save = %d", rec.Code)
	}
}

func TestKnownBadHandlers_DeleteAndDownload(t *testing.T) {
	f := &fakeKnownBad{address: kbAddress, ready: true, st: knownbad.Status{Count: 453}}
	h := NewNetauditKnownBadHandlers(f)
	if rec := do(h.Download, http.MethodPost, "/admin/api/netaudit/known-bad/download", ""); rec.Code != http.StatusOK || f.download != 1 || !strings.Contains(rec.Body.String(), `"count":453`) {
		t.Fatalf("download = %d %s", rec.Code, rec.Body.String())
	}
	f.err = errors.New("abuse.ch answered 403: check that the saved address is the one on your URLhaus page")
	if rec := do(h.Download, http.MethodPost, "/", ""); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "403") {
		t.Fatalf("failed download = %d %s", rec.Code, rec.Body.String())
	}
	f.err = nil
	if rec := do(h.DeleteAddress, http.MethodDelete, "/admin/api/netaudit/known-bad/key", ""); rec.Code != http.StatusOK || f.address != "" {
		t.Fatalf("delete = %d, address %q", rec.Code, f.address)
	}
}
