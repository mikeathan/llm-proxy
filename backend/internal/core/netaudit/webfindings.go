package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/webprobe"
	"llm-proxy/models"
)

// The web checks (core/netaudit/webprobe): a fixed list of plain GET requests aimed at the web services a scan found, in the
// operator's deep check of one device and, while the operator has switched the module on, in the nightly audit. A hit is a
// finding in this repository's own words, never the device's text.
const CheckWebExposure = "web_exposure"

const (
	// maxWebPortsPerDevice bounds what one device is asked.
	maxWebPortsPerDevice = 8
	// webCheckLimit is how long the nightly run of the web checks may take; deepWebCheckLimit is the shorter limit inside the operator's
	// deep check of one device, which answers an HTTP request that a browser or a proxy would otherwise give up on.
	webCheckLimit     = 10 * time.Minute
	deepWebCheckLimit = 3 * time.Minute
)

func init() {
	checkTitles[CheckWebExposure] = "A web page on this device tells more than it needs to"
	remediations[CheckWebExposure] = "Open the page the finding names. If the device does not need to show it on the network, turn it off or limit it to this computer in the device's settings; a login page is expected, but one that anyone on the network can reach is worth protecting with a strong password."
	guides[CheckWebExposure] = FixGuide{
		Steps: []string{
			"Open the address of the device and port the finding names in a browser, on this network.",
			"If the page is an administration or status page you do not use, turn it off or restrict it in the device's settings.",
			"If you use it, make sure it asks for a strong password and is not forwarded to the internet.",
		},
		Risk: "Turning off a device's web page can stop you managing the device until you switch it back on.",
		Undo: "Switch the page back on in the device's settings, or accept this finding to say it is meant to be there.",
	}
}

// WebChecker asks the web services their fixed questions: *webprobe.Prober.
type WebChecker interface {
	Probe(ctx context.Context, targets []webprobe.Target, limit time.Duration) (webprobe.Result, error)
	ProbeTLS(ctx context.Context, targets []webprobe.Target, limit time.Duration) (webprobe.TLSResult, error)
}

// WebTargets are the web services among the open-port findings of a scan: TCP ports whose service is HTTP or HTTPS, on private IPv4
// addresses, at most a few per device and in all. A device is one address, or every device when it is empty.
func WebTargets(findings []Finding, device string) []webprobe.Target {
	var out []webprobe.Target
	perDevice := map[string]int{}
	for _, f := range findings {
		tls, web := webService(f.Service)
		addr, err := netip.ParseAddr(f.Device)
		if f.Check != CheckOpenPort || f.Proto != "tcp" || !web || err != nil || !addr.Is4() || !addr.IsPrivate() || (device != "" && f.Device != device) {
			continue
		}
		if perDevice[f.Device] >= maxWebPortsPerDevice || len(out) >= webprobe.MaxTargets {
			continue
		}
		perDevice[f.Device]++
		out = append(out, webprobe.Target{Addr: addr, Port: f.Port, TLS: tls})
	}
	return out
}

// webService says whether a service name is a web server and whether it speaks TLS ("http", "https", "http-proxy", "ssl/http").
func webService(service string) (tls, web bool) {
	name, _, _ := strings.Cut(service, " ")
	switch {
	case strings.HasPrefix(name, "https"), strings.HasPrefix(name, "ssl/http"):
		return true, true
	case strings.HasPrefix(name, "http"):
		return false, true
	}
	return false, false
}

// WebFindings turns hits into findings, one per web service, listing the titles of what matched. The severity is the most serious of
// the hits; the Version is a digest of the check ids, so accepting a finding holds until a new kind of thing matches.
func WebFindings(hits []webprobe.Hit) []Finding {
	type key struct {
		addr netip.Addr
		port int
	}
	var order []key
	group := map[key][]webprobe.Hit{}
	for _, h := range hits {
		k := key{h.Addr, h.Port}
		if _, seen := group[k]; !seen {
			order = append(order, k)
		}
		group[k] = append(group[k], h)
	}
	var out []Finding
	for _, k := range order {
		hs := group[k]
		titles, ids, worst := make([]string, 0, len(hs)), make([]string, 0, len(hs)), SeverityInfo
		for _, h := range hs {
			titles, ids = append(titles, h.Title), append(ids, h.ID)
			if severityRank(h.Severity) > severityRank(worst) {
				worst = h.Severity
			}
		}
		slices.Sort(ids)
		out = append(out, Finding{
			Device: k.addr.String(), Port: k.port, Proto: "tcp", Check: CheckWebExposure, Severity: worst,
			Evidence: "The web checks found: " + strings.Join(titles, "; "), Version: namesDigest(ids),
		})
	}
	return out
}

// WithWebChecks makes the reporter run the web checks over the web services of the nightly scan while enabled returns true. Without it
// nothing runs.
func (r *RunReporter) WithWebChecks(enabled func() bool, w WebChecker) *RunReporter {
	r.webOn, r.web = enabled, w
	return r
}

// WithRunHealth makes the reporter record whether each optional step of a run worked, so a step that fails shows as stopped
// instead of only as a line in one night's report.
func (r *RunReporter) WithRunHealth(h *RunHealth) *RunReporter {
	r.health = h
	return r
}

// addWebChecks asks the web services this scan found their fixed questions and appends what matched. It says in the note when it
// could not run, never reading that as "nothing found", and records the outcome for the module's health.
func (r *RunReporter) addWebChecks(ctx context.Context, result *Result) string {
	if r.web == nil || r.webOn == nil || !r.webOn() {
		return ""
	}
	targets := WebTargets(result.Findings, "")
	if len(targets) == 0 {
		r.health.Record(MonitorWebChecks, nil)
		return ""
	}
	res, err := r.web.Probe(ctx, targets, webCheckLimit)
	if err != nil {
		if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
			r.health.Record(MonitorWebChecks, err)
		}
		return fmt.Sprintf("\n\n> Note: the web checks could not run (%v).", err)
	}
	r.health.Record(MonitorWebChecks, nil)
	result.Findings = append(result.Findings, WebFindings(res.Hits)...)
	note := ""
	if res.Partial {
		note = "\n\n> Note: the web checks were stopped by their time limit, so some web pages were not checked."
	}
	return note + r.addCertificateChecks(ctx, result, targets)
}

// addCertificateChecks handshakes with the secure services among the targets and adds what their certificates show. A run that cannot
// do it is said in the note and recorded as a failure of the module, like the web checks themselves.
func (r *RunReporter) addCertificateChecks(ctx context.Context, result *Result, targets []webprobe.Target) string {
	secure := secureTargets(targets)
	if len(secure) == 0 {
		return ""
	}
	res, err := r.web.ProbeTLS(ctx, secure, webCheckLimit)
	if err != nil {
		if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
			r.health.Record(MonitorWebChecks, err)
		}
		return fmt.Sprintf("\n\n> Note: the certificate checks could not run (%v).", err)
	}
	result.Findings = mergeTLS(result.Findings, res.Facts, TLSFindings(res.Facts, r.now()))
	if res.Partial {
		return "\n\n> Note: the certificate checks were stopped by their time limit, so some secure pages were not checked."
	}
	return ""
}

// secureTargets are the targets that speak TLS.
func secureTargets(targets []webprobe.Target) []webprobe.Target {
	return slices.DeleteFunc(slices.Clone(targets), func(t webprobe.Target) bool { return !t.TLS })
}

// WithWebChecks makes a deep check also run the web checks on the device's web ports. Without it a deep check is as it was.
func (d *DeepChecks) WithWebChecks(w WebChecker) *DeepChecks {
	d.web = w
	return d
}

// webFindings are the web checks of one device from the latest scan's web ports. A checker that fails adds nothing: the deep check's
// own result stands.
func (d *DeepChecks) webFindings(ctx context.Context, baseline []Finding, device string) []Finding {
	if d.web == nil {
		return nil
	}
	targets := WebTargets(baseline, device)
	if len(targets) == 0 {
		return nil
	}
	res, err := d.web.Probe(ctx, targets, deepWebCheckLimit)
	if err != nil {
		return nil
	}
	out := WebFindings(res.Hits)
	if secure := secureTargets(targets); len(secure) > 0 {
		if tlsRes, err := d.web.ProbeTLS(ctx, secure, deepWebCheckLimit); err == nil {
			out = append(out, TLSFindings(tlsRes.Facts, d.now())...)
		}
	}
	return out
}

// WebChecksConfig reads and changes whether the nightly audit runs the web checks. It is off until the operator turns it on.
type WebChecksConfig struct{ store PolicyStore }

func NewWebChecksConfig(store PolicyStore) *WebChecksConfig { return &WebChecksConfig{store: store} }

// Enabled reports whether the nightly web checks are on.
func (c *WebChecksConfig) Enabled() bool { return c.store.Get().WebChecksEnabled }

// Set switches the nightly web checks on or off.
func (c *WebChecksConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.WebChecksEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the web checks setting: %w", err)
	}
	return nil
}
