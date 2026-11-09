package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// The web checks (the Network stack's webprobe, ADR-0038): a fixed list of plain GET requests aimed at the web services a scan found, in the
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

// MaxWebTargets is the most web services one run is aimed at. It equals the SDK's bound, which a test in app pins.
const MaxWebTargets = 64

// WebTarget is one web service on one device: an address already approved by the scan scope, a port, and whether it speaks TLS.
type WebTarget struct {
	Addr netip.Addr
	Port int
	TLS  bool
}

// WebHit is a check that matched on one service. Only its id came from the Network stack; the words and the severity are this file's table.
type WebHit struct {
	Addr netip.Addr
	Port int
	ID   string
}

// WebResult is what a run found and whether its time limit ended it before every service was asked.
type WebResult struct {
	Hits    []WebHit
	Partial bool
}

// TLSFact is what one secure web service showed: the dates of its certificate, whether it signed itself, and the protocol versions older than
// 1.2 it accepted, as the standard's numbers.
type TLSFact struct {
	Addr        netip.Addr
	Port        int
	NotBefore   time.Time
	NotAfter    time.Time
	SelfSigned  bool
	OldVersions []uint16
}

// TLSResult is what a run of the handshakes found and whether its time limit ended it before every service was asked.
type TLSResult struct {
	Facts   []TLSFact
	Partial bool
}

// WebChecker asks the web services their fixed questions: the Network stack's web sensor, through the stack host.
type WebChecker interface {
	Probe(ctx context.Context, targets []WebTarget, limit time.Duration) (WebResult, error)
	ProbeTLS(ctx context.Context, targets []WebTarget, limit time.Duration) (TLSResult, error)
}

// WebCheck is the words and the severity of one check id. The Network stack owns the paths and the tests of an answer; this table owns what
// a finding says, so nothing a device sent can reach a report. A test in app holds it equal to the stack's list.
type WebCheck struct{ ID, Title, Severity string }

// webChecks is the whole list, in the order the stack asks them.
var webChecks = []WebCheck{
	{ID: "asus_router_login", Title: "The login page of an Asus router", Severity: SeverityInfo},
	{ID: "brother_printer_page", Title: "The status page of a Brother printer", Severity: SeverityInfo},
	{ID: "fuji_xerox_printer_page", Title: "The status page of a Fuji Xerox printer", Severity: SeverityInfo},
	{ID: "samsung_printer_page", Title: "The web page of a Samsung printer", Severity: SeverityInfo},
	{ID: "grafana_login", Title: "The login page of Grafana", Severity: SeverityInfo},
	{ID: "home_assistant_page", Title: "The page of Home Assistant", Severity: SeverityInfo},
	{ID: "kubernetes_dashboard", Title: "A Kubernetes dashboard", Severity: SeverityInfo},
	{ID: "openwrt_luci_login", Title: "The login page of an OpenWrt router", Severity: SeverityInfo},
	{ID: "openvpn_router_management", Title: "The OpenVPN page of a router", Severity: SeverityInfo},
	{ID: "qnap_login", Title: "The login page of a QNAP storage box", Severity: SeverityInfo},
	{ID: "truenas_login", Title: "The login page of TrueNAS", Severity: SeverityInfo},
	{ID: "ubiquiti_edgerouter_page", Title: "The page of a Ubiquiti EdgeRouter", Severity: SeverityInfo},
	{ID: "unifi_os_login", Title: "The login page of UniFi OS", Severity: SeverityInfo},
	{ID: "unifi_security_gateway_page", Title: "The page of a UniFi Security Gateway", Severity: SeverityInfo},
	{ID: "zyxel_router_page", Title: "The configuration page of a Zyxel router", Severity: SeverityInfo},
	{ID: "jellyfin_page", Title: "The page of a Jellyfin media server", Severity: SeverityInfo},
	{ID: "synology_api_list", Title: "Synology DSM lists its programming interface to anyone", Severity: SeverityLow},
	{ID: "apache_status_page", Title: "An Apache status page that anyone can open", Severity: SeverityLow},
	{ID: "nginx_status_page", Title: "An nginx status page that anyone can open", Severity: SeverityLow},
	{ID: "metrics_exporter_page", Title: "A metrics page that anyone can open", Severity: SeverityLow},
	{ID: "openapi_description", Title: "A description of the device's programming interface that anyone can read", Severity: SeverityLow},
	{ID: "ds_store_file", Title: "A Mac folder-listing file that the web server hands out", Severity: SeverityLow},
	{ID: "apache_default_page", Title: "A web server that still shows Apache's test page", Severity: SeverityLow},
	{ID: "nginx_default_page", Title: "A web server that still shows nginx's welcome page", Severity: SeverityLow},
}

// WebChecks returns the table of check words, in the order the stack asks them.
func WebChecks() []WebCheck { return slices.Clone(webChecks) }

// webCheckByID finds a check by its id.
var webCheckByID = func() map[string]WebCheck {
	m := make(map[string]WebCheck, len(webChecks))
	for _, c := range webChecks {
		m[c.ID] = c
	}
	return m
}()

// WebTargets are the web services among the open-port findings of a scan: TCP ports whose service is HTTP or HTTPS, on private IPv4
// addresses, at most a few per device and in all. A device is one address, or every device when it is empty.
func WebTargets(findings []Finding, device string) []WebTarget {
	var out []WebTarget
	perDevice := map[string]int{}
	for _, f := range findings {
		tls, web := webService(f.Service)
		addr, err := netip.ParseAddr(f.Device)
		if f.Check != CheckOpenPort || f.Proto != "tcp" || !web || err != nil || !addr.Is4() || !addr.IsPrivate() || (device != "" && f.Device != device) {
			continue
		}
		if perDevice[f.Device] >= maxWebPortsPerDevice || len(out) >= MaxWebTargets {
			continue
		}
		perDevice[f.Device]++
		out = append(out, WebTarget{Addr: addr, Port: f.Port, TLS: tls})
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
func WebFindings(hits []WebHit) []Finding {
	type key struct {
		addr netip.Addr
		port int
	}
	var order []key
	group := map[key][]WebHit{}
	for _, h := range hits {
		if _, known := webCheckByID[h.ID]; !known {
			continue
		}
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
			c := webCheckByID[h.ID]
			titles, ids = append(titles, c.Title), append(ids, h.ID)
			if severityRank(c.Severity) > severityRank(worst) {
				worst = c.Severity
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
func (r *RunReporter) addCertificateChecks(ctx context.Context, result *Result, targets []WebTarget) string {
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
func secureTargets(targets []WebTarget) []WebTarget {
	return slices.DeleteFunc(slices.Clone(targets), func(t WebTarget) bool { return !t.TLS })
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
