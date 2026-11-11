package app

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"time"

	"llm-proxy/internal/core/netaudit/hosttraffic"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/privhelper/catalog"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// capture_host (plan network-host-traffic-and-findings-ui, Constitution I.13): the service's side of the look at this computer's own traffic.
// The route is the generic helper route (step-up, one-use grant); this file adds what is specific to it: this computer's own addresses (which
// say which way a packet went), where the last summary is kept, and the read route. The summary is a snapshot of one link for up to a minute,
// never a finding, and it stays out of the audit report and every prompt.

// hostTrafficSink keeps the last summary. *persistence.HostTrafficStore satisfies it.
type hostTrafficSink interface {
	Read() (models.HostTraffic, error)
	Write(models.HostTraffic) error
}

// newHostTrafficStore keeps the summary in the data folder and registers it for a factory reset.
func newHostTrafficStore(ctx *AppContext) hostTrafficSink {
	return withReset(ctx, "The last host traffic summary", persistence.NewHostTrafficStore(ctx.RootDir()))
}

// ownAddresses lists the addresses this computer holds on every interface. It is a variable so a test can name them.
var ownAddresses = func() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr())
		}
	}
	return out
}

// readHostTraffic reads one run's output in this unprivileged process (Constitution I.13).
func readHostTraffic(data []byte, owners hosttraffic.Owners) (models.HostTraffic, error) {
	return hosttraffic.Summarize(data, ownAddresses(), owners)
}

// keepHostTraffic stamps a decoded summary with the time and the interface it listened on, and keeps it. A failed save is logged and dropped:
// the operator already has the answer on screen.
func keepHostTraffic(sink hostTrafficSink, params []byte, s models.HostTraffic) {
	if sink == nil {
		return
	}
	var req catalog.CaptureHostParams
	if json.Unmarshal(params, &req) != nil {
		return
	}
	if err := sink.Write(hosttraffic.Stamp(s, time.Now(), req.Iface)); err != nil {
		logging.Warn("a host traffic summary was not kept", "error", err)
	}
}

// hostTrafficView is the answer of GET /admin/api/netaudit/host-traffic: the last summary, or null when there has been none.
type hostTrafficView struct {
	Summary *models.HostTraffic `json:"summary"`
}

// hostTrafficHandler answers the last kept summary.
func hostTrafficHandler(sink hostTrafficSink) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		view := hostTrafficView{}
		if sink != nil {
			s, err := sink.Read()
			if err != nil {
				api.WriteJSONError(w, http.StatusInternalServerError, "the last host traffic summary could not be read")
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
