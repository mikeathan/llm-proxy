package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"llm-proxy/internal/core/netaudit/framewire"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/privhelper/catalog"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// capture_window (plan network-capture-window-plan): the service's side of the helper's headers-only listen. The route is the generic helper
// route (step-up, one-use grant); this file adds what is specific to it: the interface it listens on when the operator names none, where the
// last summary is kept, and the read route. The summary is a snapshot of one link, never a finding, and it stays out of the audit report and
// every prompt.

// captureSink keeps the last summary. *persistence.CaptureWindowStore satisfies it.
type captureSink interface {
	Read() (models.CaptureSummary, error)
	Write(models.CaptureSummary) error
}

// newCaptureStore keeps the summary in the data folder and registers it for a factory reset.
func newCaptureStore(ctx *AppContext) captureSink {
	return withReset(ctx, "The last capture_window summary", persistence.NewCaptureWindowStore(ctx.RootDir()))
}

// errNoDefaultInterface is what the operator is told when the default route is not known; the wording is fixed.
var errNoDefaultInterface = errors.New("this computer has no default network interface to listen on")

// defaultInterface is the interface the default route leaves by, read from the route table.
func defaultInterface() (string, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return "", errNoDefaultInterface
	}
	defer f.Close()
	name, err := neighbors.ParseDefaultInterface(f)
	if err != nil {
		return "", errNoDefaultInterface
	}
	return name, nil
}

// helperDefaults completes an empty capture_window request with the default interface and the longest window. A body the operator wrote
// is passed on as it is: the catalogue checks it and the helper checks that the interface exists.
func helperDefaults(iface func() (string, error)) api.HelperDefaults {
	return func(action string, body []byte) ([]byte, error) {
		if action != "capture_window" || len(bytes.TrimSpace(body)) != 0 {
			return body, nil
		}
		name, err := iface()
		if err != nil {
			return nil, err
		}
		return json.Marshal(catalog.CaptureWindowParams{Iface: name})
	}
}

// keepCapture stamps a decoded summary with the time, the interface it listened on and whether that link is Wi-Fi, marks the announced
// addresses outside the ranges that link has itself (prefixes), and keeps it. A failed
// save is logged and dropped: the operator already has the answer on screen.
func keepCapture(sink captureSink, prefixes func(iface string) []netip.Prefix, params []byte, s models.CaptureSummary) {
	if sink == nil {
		return
	}
	var req catalog.CaptureWindowParams
	if json.Unmarshal(params, &req) != nil {
		return
	}
	wireless := false
	if st, err := os.Stat(filepath.Join("/sys/class/net", req.Iface, "wireless")); err == nil && st.IsDir() {
		wireless = true
	}
	s = framewire.MarkOffNetwork(s, prefixes(req.Iface))
	if err := sink.Write(framewire.Stamp(s, time.Now(), req.Iface, wireless)); err != nil {
		logging.Warn("a capture summary was not kept", "error", err)
	}
}

// linkPrefixes are the address ranges the named interface has itself, none when it cannot be read.
func linkPrefixes(iface string) []netip.Prefix {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return nil
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

// captureView is the answer of GET /admin/api/netaudit/capture-window: the last summary, or null when there has been none.
type captureView struct {
	Summary *models.CaptureSummary `json:"summary"`
}

// captureWindowHandler answers the last kept summary.
func captureWindowHandler(sink captureSink) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		view := captureView{}
		if sink != nil {
			s, err := sink.Read()
			if err != nil {
				api.WriteJSONError(w, http.StatusInternalServerError, "the last capture summary could not be read")
				return
			}
			if !s.At.IsZero() {
				view.Summary = &s
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	}
}
