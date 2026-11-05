package netaudit

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/nuclei"
	"llm-proxy/models"
)

// The web checks (Constitution II.3, nuclei): reviewed HTTP templates run against the web services a scan found, in the operator's deep
// check of one device and, while the operator has switched the module on, in the nightly audit. A hit is a finding in this
// repository's own words (the title of the reviewed template), never the template's or the device's text.
const CheckWebExposure = "web_exposure"

const (
	// maxWebPortsPerDevice and maxWebTargets bound what one run is aimed at.
	maxWebPortsPerDevice = 8
	maxWebTargets        = nuclei.MaxTargets
	// webCheckLimit is how long one run of the web checks may take.
	webCheckLimit = 10 * time.Minute
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

// WebChecker runs the reviewed web checks: *nuclei.Checker.
type WebChecker interface {
	Ready() (bool, string)
	Check(ctx context.Context, targets []nuclei.Target, limit time.Duration) (nuclei.Result, error)
}

// WebTargets are the web services among the open-port findings of a scan: TCP ports whose service is HTTP or HTTPS, on private IPv4
// addresses, at most a few per device and in all. A device is one address, or every device when it is empty.
func WebTargets(findings []Finding, device string) []nuclei.Target {
	var out []nuclei.Target
	perDevice := map[string]int{}
	for _, f := range findings {
		tls, web := webService(f.Service)
		addr, err := netip.ParseAddr(f.Device)
		if f.Check != CheckOpenPort || f.Proto != "tcp" || !web || err != nil || !addr.Is4() || !addr.IsPrivate() || (device != "" && f.Device != device) {
			continue
		}
		if perDevice[f.Device] >= maxWebPortsPerDevice || len(out) >= maxWebTargets {
			continue
		}
		perDevice[f.Device]++
		out = append(out, nuclei.Target{Addr: addr, Port: f.Port, TLS: tls})
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
// the hits; the Version is a digest of the template ids, so accepting a finding holds until a new kind of thing matches.
func WebFindings(hits []nuclei.Hit) []Finding {
	type key struct {
		addr netip.Addr
		port int
	}
	var order []key
	group := map[key][]nuclei.Hit{}
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
			titles, ids = append(titles, h.Title), append(ids, h.TemplateID)
			if severityRank(h.Severity) > severityRank(worst) {
				worst = h.Severity
			}
		}
		slices.Sort(ids)
		out = append(out, Finding{
			Device: k.addr.String(), Port: k.port, Proto: "tcp", Check: CheckWebExposure, Severity: worst,
			Evidence: "The reviewed web checks matched: " + strings.Join(titles, "; "), Version: namesDigest(ids),
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

// addWebChecks runs the reviewed templates against the web services this scan found and appends what matched. It says in the note
// when it could not run (nuclei missing, templates not downloaded, a failure), never reading that as "nothing found".
func (r *RunReporter) addWebChecks(ctx context.Context, result *Result) string {
	if r.web == nil || r.webOn == nil || !r.webOn() {
		return ""
	}
	if ok, why := r.web.Ready(); !ok {
		return fmt.Sprintf("\n\n> Note: the web checks did not run (%s).", why)
	}
	targets := WebTargets(result.Findings, "")
	if len(targets) == 0 {
		return ""
	}
	res, err := r.web.Check(ctx, targets, webCheckLimit)
	if err != nil {
		return fmt.Sprintf("\n\n> Note: the web checks could not run (%v).", err)
	}
	result.Findings = append(result.Findings, WebFindings(res.Hits)...)
	if res.Partial {
		return "\n\n> Note: the web checks were stopped by their time limit, so some web pages were not checked."
	}
	return ""
}

// WithWebChecks makes a deep check also run the web checks on the device's web ports, when they are ready. Without it a deep check is
// as it was.
func (d *DeepChecks) WithWebChecks(w WebChecker) *DeepChecks {
	d.web = w
	return d
}

// webFindings are the web checks of one device from the latest scan's web ports. A checker that is not ready or fails adds nothing:
// the deep check's own result stands, and the web checks have their own readiness line.
func (d *DeepChecks) webFindings(ctx context.Context, baseline []Finding, device string) []Finding {
	if d.web == nil {
		return nil
	}
	if ok, _ := d.web.Ready(); !ok {
		return nil
	}
	targets := WebTargets(baseline, device)
	if len(targets) == 0 {
		return nil
	}
	res, err := d.web.Check(ctx, targets, webCheckLimit)
	if err != nil {
		return nil
	}
	return WebFindings(res.Hits)
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
