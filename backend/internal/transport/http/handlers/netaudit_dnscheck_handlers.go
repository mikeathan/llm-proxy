package handlers

import (
	"net/http"

	"llm-proxy/internal/core/netaudit"
)

// DNSCheckControl is the DNS check's setting: *netaudit.DNSCheckConfig.
type DNSCheckControl interface {
	View() netaudit.DNSCheckView
	SetEnabled(enabled bool) error
	SetInterval(minutes int) error
	SetNames(names []string) error
	SetResolverURL(raw string) error
}

// DNSCheckReport is what the check has done so far: *netaudit.DNSCollector.
type DNSCheckReport interface {
	Status() netaudit.DNSStatus
	Summary() (netaudit.DNSSummary, error)
}

// NetauditDNSCheckHandlers serve the DNS check's settings and its log boiled down: the summary is what the operator brings
// back after a week of comparisons.
type NetauditDNSCheckHandlers struct {
	control DNSCheckControl
	report  DNSCheckReport
}

func NewNetauditDNSCheckHandlers(control DNSCheckControl, report DNSCheckReport) *NetauditDNSCheckHandlers {
	return &NetauditDNSCheckHandlers{control: control, report: report}
}

type dnsCheckResponse struct {
	netaudit.DNSCheckView
	Status  netaudit.DNSStatus  `json:"status"`
	Summary netaudit.DNSSummary `json:"summary"`
}

type dnsCheckRequest struct {
	Enabled         *bool     `json:"enabled"`
	IntervalMinutes *int      `json:"interval_minutes"`
	Names           *[]string `json:"names"`
	ResolverURL     *string   `json:"resolver_url"`
}

// Get returns the settings, the latest cycle and the summary of the log.
func (h *NetauditDNSCheckHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	summary, err := h.report.Summary()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, dnsCheckResponse{DNSCheckView: h.control.View(), Status: h.report.Status(), Summary: summary})
}

// Put changes what is sent: the names and resolver first, then the interval, and the switch last, so a refused setting turns
// nothing on. Switching on lets the outside resolver learn the names asked and the network's public address, so it is a
// step-up route.
func (h *NetauditDNSCheckHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req dnsCheckRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	steps := []func() error{}
	if req.Names != nil {
		steps = append(steps, func() error { return h.control.SetNames(*req.Names) })
	}
	if req.ResolverURL != nil {
		steps = append(steps, func() error { return h.control.SetResolverURL(*req.ResolverURL) })
	}
	if req.IntervalMinutes != nil {
		steps = append(steps, func() error { return h.control.SetInterval(*req.IntervalMinutes) })
	}
	for _, step := range steps {
		if err := step(); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Enabled != nil {
		if err := h.control.SetEnabled(*req.Enabled); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.Get(w, r)
}
