package netaudit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"llm-proxy/internal/core/netaudit/routerhistory"
	"llm-proxy/models"
)

// The router connection findings: what the router's connection table says about where a device connects (Constitution I.15). They are
// hints to review, never proof: the table is a snapshot, a company is the registered holder of an address, and a CDN's servers sit under
// many names. A device is judged only after its first day of history, so the first nights only learn.
const (
	CheckRouterNewCompany = "router_new_company"
	CheckRouterNewCountry = "router_new_country"
)

// MonitorRouterConn is the nightly read of the router's table: stopped while switched on and the latest read failed, or none worked for three days.
const MonitorRouterConn = "router_connections"

// RouterConnStallLimit is how long the nightly read may go without working before the monitor calls it stopped.
const RouterConnStallLimit = 3 * 24 * time.Hour

// routerConnReadLimit bounds the nightly read of the router, which is one SSH session.
const routerConnReadLimit = 45 * time.Second

func init() {
	checkTitles[CheckRouterNewCompany] = "A device connected to a company it had not connected to before"
	checkTitles[CheckRouterNewCountry] = "A device connected to a country it had not connected to before"
	remediations[CheckRouterNewCompany] = "Look at the company. A vendor's update or cloud service is the usual answer, and a service you just started using explains it. If it is yours, accept this finding. The router's table is a snapshot and a company is only the registered holder of an address, so treat it as a hint."
	remediations[CheckRouterNewCountry] = "Look at the country. Streaming, updates and websites are often served from another country, and a trip or a new service explains it. If you expect it, accept this finding. The country is where the address is used, from a table kept on this computer, and is approximate, so treat it as a hint."
	guides[CheckRouterNewCompany] = routerConnGuide("Find out which program or device at that address talks to the company.", "Search for the company name, or look at the device's apps and services for one that uses it. A cloud, update or analytics service is the usual answer.")
	guides[CheckRouterNewCountry] = routerConnGuide("Find out what the device was doing in the last day.", "Look at what is new on the device: an app, a website or a service. Content networks serve from many countries.")
}

func routerConnGuide(find, how string) FixGuide {
	return FixGuide{
		Steps: []string{find, how, "If it is a service you use, accept this finding. If it is not, remove the program or the device, and change the passwords of any account that was on it."},
		Risk:  "Removing a program or a device can stop something you use. Accepting a finding hides it for ninety days, then it is shown again.",
		Undo:  "Reinstall the program, or take the acceptance back on the Findings tab.",
	}
}

// RouterConnFindings turns the watch's signals into findings keyed by the device's address, one per device and kind. The evidence is fixed
// words and the holders' own names from a public table (text only); the operator's name for the device is never put in it. The Version is a
// digest of the set, so accepting a finding holds until the set grows.
func RouterConnFindings(signals []routerhistory.Signal) []Finding {
	var out []Finding
	for _, s := range signals {
		names := slices.Clone(s.Names)
		slices.Sort(names)
		f := Finding{Device: s.IP, Severity: SeverityLow, Version: namesDigest(names)}
		switch s.Kind {
		case routerhistory.NewCompany:
			f.Check, f.Evidence = CheckRouterNewCompany, "Connected, in the last day, to a company it had not connected to before: "+listNames(names)
		case routerhistory.NewCountry:
			f.Check, f.Evidence = CheckRouterNewCountry, "Connected, in the last day, to a country it had not connected to before: "+listNames(names)
		default:
			continue
		}
		out = append(out, f)
	}
	return out
}

// RouterConnConfig reads and changes whether the router watch is on. It is off until the operator turns it on, and turning it on needs the
// passphrase (the module's step-up), because it makes the audit sign in to the router.
type RouterConnConfig struct{ store PolicyStore }

func NewRouterConnConfig(store PolicyStore) *RouterConnConfig { return &RouterConnConfig{store: store} }

// Enabled reports whether the watch is on.
func (c *RouterConnConfig) Enabled() bool { return c.store.Get().RouterConnWatchEnabled }

// Set switches the watch on or off.
func (c *RouterConnConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.RouterConnWatchEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the router watch setting: %w", err)
	}
	return nil
}

// RouterConnSource is what the nightly report reads of the router watch. Refresh reads the router once, in the program's own code: no tool of
// an agent, and nothing an agent writes, names the router, the user or the key. Signals are empty while the watch is off.
type RouterConnSource interface {
	Enabled() bool
	Refresh(ctx context.Context) error
	Signals(now time.Time) []routerhistory.Signal
}

// ErrRouterConnSkipped is what a source returns from Refresh when there was nothing to do: the setup is not finished, or the router was just read.
var ErrRouterConnSkipped = errors.New("the router was not read")

// WithRouterConn makes the reporter read the router once and add what the watch found. Without it nothing is added.
func (r *RunReporter) WithRouterConn(src RouterConnSource) *RunReporter {
	r.routerConn = src
	return r
}

// addRouterConn reads the router's table once (when the watch is on), keeps it in the history, and appends the findings for the devices this
// scan looked at. A read that fails is recorded for the module's health and said in the note, never read as "nothing new".
func (r *RunReporter) addRouterConn(ctx context.Context, result *Result) string {
	if r.routerConn == nil || !r.routerConn.Enabled() {
		return ""
	}
	note := ""
	readCtx, cancel := context.WithTimeout(ctx, routerConnReadLimit)
	err := r.routerConn.Refresh(readCtx)
	cancel()
	switch {
	case err == nil:
		r.health.Record(MonitorRouterConn, nil)
	case errors.Is(err, ErrRouterConnSkipped):
		// Nothing to read tonight: neither a success nor a failure.
	default:
		if ctx.Err() == nil {
			r.health.Record(MonitorRouterConn, err)
		}
		note = fmt.Sprintf("\n\n> Note: the router's connection table could not be read (%v).", err)
	}
	result.Findings = append(result.Findings, restrictTo(RouterConnFindings(r.routerConn.Signals(r.now())), scannedDevices(result.Hosts))...)
	return note
}
