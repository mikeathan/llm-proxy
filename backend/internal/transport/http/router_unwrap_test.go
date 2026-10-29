package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// deadlineWriter is a response writer that supports per-request deadlines, as the server's own does.
type deadlineWriter struct {
	http.ResponseWriter
	deadline time.Time
}

func (d *deadlineWriter) SetReadDeadline(t time.Time) error { d.deadline = t; return nil }

// A handler behind the panic recovery can still extend its own read deadline (the vulnerability-data import needs more
// than the server's default for a file of tens of megabytes), which only works if the wrapper lets the controller through.
func TestRecoverHandler_LetsAHandlerReachTheServersDeadlineControls(t *testing.T) {
	under := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
	want := time.Now().Add(time.Minute)
	var err error
	recoverHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		err = http.NewResponseController(w).SetReadDeadline(want)
	})).ServeHTTP(under, httptest.NewRequest(http.MethodPost, "/", nil))
	if err != nil || !under.deadline.Equal(want) {
		t.Errorf("err %v, deadline %v want %v", err, under.deadline, want)
	}
}
