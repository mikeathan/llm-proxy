package scantool

import (
	"encoding/json"
	"slices"

	"llm-proxy/internal/core/netaudit"
)

// The model is not given the raw scan result. A /24 result run through the agent's tool-result cap lost its middle (the model saw 5
// of 15 devices and the end of the findings), so what it reads is a compact view sized to that cap: only the devices that
// answered, only their open ports (the scan also lists filtered ones), one entry per port, a guessed name marked as a guess, and no
// open-port findings (every open port is already in its device's list, and the header counts them).

// viewPort is one open port as the model reads it.
type viewPort struct {
	Port    int    `json:"port"`
	Proto   string `json:"proto,omitempty"`
	Service string `json:"service,omitempty"`
	Product string `json:"product,omitempty"`
	Version string `json:"version,omitempty"`
}

// viewHost is one device that answered. Unscanned marks one the engine did not finish: its ports are unknown, not none.
type viewHost struct {
	Addr      string     `json:"addr"`
	Ports     []viewPort `json:"ports,omitempty"`
	Unscanned bool       `json:"unscanned,omitempty"`
}

// scanView is what the model is given of a scan.
type scanView struct {
	Engine   string             `json:"engine"`
	Hosts    []viewHost         `json:"hosts"`
	Findings []netaudit.Finding `json:"findings"`
	Warnings []string           `json:"warnings,omitempty"`
}

// viewOf reduces a result. A service name that is only the usual name for that port number, and a product name nmap gave from the
// shape of a reply alone ("Cloudflare public DNS" for a router's DNS port), are left out, wherever they appear: marked "(guess)" they
// were still repeated as fact, and a port with no name gives the model nothing to repeat. The report's tables still show them.
func viewOf(res netaudit.Result) scanView {
	v := scanView{Engine: res.Engine, Hosts: []viewHost{}, Findings: []netaudit.Finding{}, Warnings: res.Warnings}
	for _, h := range res.Hosts {
		if !h.Up && !h.Unscanned && len(h.Ports) == 0 {
			continue
		}
		vh := viewHost{Addr: h.Addr.String(), Unscanned: h.Unscanned}
		for _, p := range h.Ports {
			if p.State == "open" {
				vh.Ports = append(vh.Ports, viewPortOf(p))
			}
		}
		v.Hosts = append(v.Hosts, vh)
	}
	for _, f := range res.Findings {
		if f.Check != netaudit.CheckOpenPort {
			f.Service, f.Version = netaudit.ConfirmedText(f.Service), netaudit.ConfirmedText(f.Version)
			v.Findings = append(v.Findings, f)
		}
	}
	return v
}

func viewPortOf(p netaudit.Port) viewPort {
	out := viewPort{Port: p.Number, Service: p.Service, Product: p.Product, Version: p.Version}
	if p.Proto != "tcp" {
		out.Proto = p.Proto
	}
	if p.Guessed {
		out.Service = ""
	}
	if p.ProductGuessed {
		out.Product = ""
	}
	return out
}

// cut says what fitting left out.
type cut struct{ Hosts, Findings int }

// encodedSize is how long text is once JSON-encoded, which is how the agent measures a tool result against its cap.
func encodedSize(s string) int {
	b, err := json.Marshal(s)
	if err != nil {
		return len(s)
	}
	return len(b)
}

// fit leaves out whole devices (those with no open port first, then from the end) and then whole findings until the compact JSON
// fits budget, and says how many. It never cuts inside an entry, so the model never reads half a record.
func (v scanView) fit(budget int) (scanView, cut) {
	var c cut
	fits := func() bool {
		b, err := json.Marshal(v)
		return err != nil || encodedSize(string(b)) <= budget
	}
	slices.SortStableFunc(v.Hosts, func(a, b viewHost) int { return len(b.Ports) - len(a.Ports) })
	for !fits() && len(v.Hosts) > 0 {
		v.Hosts = v.Hosts[:len(v.Hosts)-1]
		c.Hosts++
	}
	for !fits() && len(v.Findings) > 0 {
		v.Findings = v.Findings[:len(v.Findings)-1]
		c.Findings++
	}
	return v, c
}
