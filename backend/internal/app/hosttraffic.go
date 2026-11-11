package app

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit"
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

// sysNet is where Linux lists each network card, with its hardware address in a file named address. Reading it needs no netlink socket, which
// the deployment refuses (pitfall 65).
const sysNet = "/sys/class/net"

// hardwareAddrLen is the length of an Ethernet hardware address.
const hardwareAddrLen = 6

// cardAddresses lists the hardware addresses of this computer's network cards from sysNet; none where there is no such folder (macOS) or it
// cannot be read.
func cardAddresses(root string) [][6]byte {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out [][6]byte
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(root, e.Name(), "address"))
		if err != nil {
			continue
		}
		hw, err := net.ParseMAC(strings.TrimSpace(string(raw)))
		if err != nil || len(hw) != hardwareAddrLen {
			continue
		}
		out = append(out, [6]byte(hw))
	}
	return out
}

// trafficSelf says, at the moment a look is read, which packets are this computer's: its addresses through the scan scope's listing (which falls
// back to the default route's address where netlink is refused), its cards' hardware addresses, and the port this program listens on.
func trafficSelf(scope *netaudit.Scope, bind func() string) func() hosttraffic.Self {
	return func() hosttraffic.Self {
		var self hosttraffic.Self
		if scope != nil {
			if addrs, err := scope.LocalAddrs(); err == nil {
				for _, a := range addrs {
					self.Addrs = append(self.Addrs, a.Addr)
				}
			}
		}
		self.MACs = cardAddresses(sysNet)
		if bind != nil {
			self.Port = bindPort(bind())
		}
		return self
	}
}

// bindPort is the port of a listen address ("host:port"), or 0 when it has none.
func bindPort(addr string) uint16 {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(n)
}

// readHostTraffic reads one run's output in this unprivileged process (Constitution I.13).
func readHostTraffic(data []byte, self hosttraffic.Self, owners hosttraffic.Owners) (models.HostTraffic, error) {
	return hosttraffic.Summarize(data, self, owners)
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
