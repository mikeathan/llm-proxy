package app

import (
	"context"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

// The watch over the Network module's own parts (monitor_stalled): which parts it reads and how long each may go without
// working. Every limit is a judgement, not a measurement; the router log's quiet limit especially (some routers say nothing at
// night), which is why it is a day.
const (
	// minMonitorLimit is the shortest limit, so a part on a short interval is not called stalled for one slow cycle.
	minMonitorLimit = 30 * time.Minute
	// socketStallLimit is how long a listener (the decoys, the router log) may be closed while switched on.
	socketStallLimit = 15 * time.Minute
	// routerQuietLimit is how long the router's log may send nothing at all. Not measured on a real router yet.
	routerQuietLimit = 24 * time.Hour
	// auditStallLimit is how long after the last audited day before the nightly audit counts as stopped: two missed nights.
	auditStallLimit = 48 * time.Hour
	// webChecksStallLimit is how long the nightly web checks may go without a run that worked: three nights, so the nightly audit's own
	// stall (two nights) is told first when the whole audit stopped.
	webChecksStallLimit = 72 * time.Hour
	// auditHistoryDays is how far back an audit counts as "the audit is in use".
	auditHistoryDays = 30
)

// newMonitorWatch builds the watch over the parts that exist. It reads statuses already held in memory and the daily history;
// it probes nothing.
func newMonitorWatch(n *netauditServices, notify notifyFunc) *netaudit.MonitorWatch {
	var parts []netaudit.MonitorPart
	add := func(wired bool, id, words string, read func() netaudit.MonitorReading) {
		if wired {
			parts = append(parts, netaudit.MonitorPart{ID: id, Words: words, Read: read})
		}
	}
	add(n.Presence != nil && n.PresenceConfig != nil, netaudit.MonitorPresence, "the presence check", func() netaudit.MonitorReading {
		return presenceReading(n.PresenceConfig.Settings(), n.Presence.Status())
	})
	if n.RouterLog != nil && n.RouterLogConfig != nil {
		read := func() (netaudit.MonitorReading, netaudit.MonitorReading) {
			return routerLogReadings(n.RouterLogConfig.Enabled(), n.RouterLog.Status(), time.Now())
		}
		add(true, netaudit.MonitorRouterLog, "the listener for the router's log", func() netaudit.MonitorReading { s, _ := read(); return s })
		add(true, netaudit.MonitorRouterLogQuiet, "the router's log (nothing received for a day)", func() netaudit.MonitorReading { _, q := read(); return q })
	}
	add(n.Network != nil, netaudit.MonitorDNS, "the DNS comparison", func() netaudit.MonitorReading {
		st, _ := n.Network.Module(context.Background(), netaudit.MonitorDNS)
		return dnsReading(st)
	})
	add(n.PhoneHomeConfig != nil && n.RouterLog != nil, netaudit.MonitorPhoneHome, "the phone-home watch (no DNS names received for a day: check that the router logs DNS queries and sends them to this computer)", func() netaudit.MonitorReading {
		return phoneHomeReading(n.PhoneHomeConfig.Enabled(), n.RouterLog.Status())
	})
	add(n.WebChecksConfig != nil && n.RunHealth != nil, netaudit.MonitorWebChecks, "the nightly web checks (the last run failed, or none worked for three days)", func() netaudit.MonitorReading {
		return n.RunHealth.Reading(netaudit.MonitorWebChecks, n.WebChecksConfig.Enabled(), webChecksStallLimit)
	})
	add(n.LoginWatch != nil, netaudit.MonitorLoginWatch, "the watch over failed logins to this computer (the system's record cannot be read)", func() netaudit.MonitorReading {
		return n.LoginWatch.Reading()
	})
	add(n.Wifi != nil, netaudit.MonitorWifi, "the Wi-Fi look-alike watch (its last look failed)", func() netaudit.MonitorReading {
		return n.Wifi.Reading()
	})
	add(n.Decoys != nil && n.DecoyConfig != nil, netaudit.MonitorDecoys, "the decoy services", func() netaudit.MonitorReading {
		return decoyReading(n.DecoyConfig.Enabled(), n.Decoys.Status(), time.Now())
	})
	add(n.History != nil, netaudit.MonitorAudit, "the nightly audit", func() netaudit.MonitorReading {
		return auditReading(n.History.Series)
	})
	add(n.Alerts != nil, netaudit.MonitorAlerts, "sending alerts to your connectors", func() netaudit.MonitorReading {
		items, err := n.Alerts.After(0, alertDeliveryWindow)
		if err != nil {
			return netaudit.MonitorReading{}
		}
		return alertDeliveryReading(items, time.Now())
	})
	return netaudit.NewMonitorWatch(netaudit.MonitorWatchDeps{Parts: parts, Notify: notify})
}

// alertDeliveryWindow is how many of the newest alerts the delivery reading looks at; alertDeliveryLimit how long sends may
// keep failing before the part counts as stopped (a connector that is down for a moment recovers on the next send).
const (
	alertDeliveryWindow = 10
	alertDeliveryLimit  = time.Hour
)

// alertDeliveryReading is healthy while the newest alert went out (or had no connector, which alert_no_channel already says).
// While the newest sends failed, the part last worked when that run of failures began.
func alertDeliveryReading(newestFirst []models.NetworkAlert, now time.Time) netaudit.MonitorReading {
	if len(newestFirst) == 0 {
		return netaudit.MonitorReading{}
	}
	r := netaudit.MonitorReading{On: true, LastGood: now, Limit: alertDeliveryLimit}
	for _, a := range newestFirst {
		if a.Delivery != models.NetworkAlertFailed {
			break
		}
		r.LastGood = a.At
	}
	return r
}

// cycleLimit is how long a part on an interval may go without a cycle: three intervals, never less than minMonitorLimit.
func cycleLimit(every time.Duration) time.Duration { return max(3*every, minMonitorLimit) }

// presenceReading counts a look that worked, whether or not its alert could be sent: an undelivered alert is the connectors'
// trouble, which the alert log already shows, not a stopped check.
func presenceReading(s netaudit.PresenceSettings, st netaudit.PresenceStatus) netaudit.MonitorReading {
	return netaudit.MonitorReading{On: s.Enabled, LastGood: st.LastLook, Limit: cycleLimit(s.Every)}
}

// dnsReading counts a cycle that compared as working; a cycle the settings refused did nothing. A check with no names never
// runs a cycle, so it is not watched.
func dnsReading(st stacksdk.ModuleStatus) netaudit.MonitorReading {
	r := netaudit.MonitorReading{On: st.Enabled, Limit: cycleLimit(st.Every)}
	if st.Problem == "" {
		r.LastGood = st.LastRun
	}
	return r
}

// routerLogReadings are the socket (open and without a problem now) and the log itself (when the router last sent anything).
func routerLogReadings(on bool, st routerlog.Status, now time.Time) (socket, quiet netaudit.MonitorReading) {
	socket = netaudit.MonitorReading{On: on, Limit: socketStallLimit}
	if st.Listening && st.Problem == "" {
		socket.LastGood = now
	}
	return socket, netaudit.MonitorReading{On: on, LastGood: st.LastHeard, Limit: routerQuietLimit}
}

// phoneHomeStallLimit is how long the phone-home watch may go without a DNS line from the router before it counts as stopped. A home
// network asks for names all day; a day of silence means the router is not sending them (DNS query logging is off, or the router log
// is not reaching this computer).
const phoneHomeStallLimit = 24 * time.Hour

// phoneHomeReading is healthy while DNS lines keep arriving.
func phoneHomeReading(on bool, st routerlog.Status) netaudit.MonitorReading {
	return netaudit.MonitorReading{On: on, LastGood: st.QueriesLast, Limit: phoneHomeStallLimit}
}

// decoyReading is healthy while every decoy listens and nothing is wrong.
func decoyReading(on bool, st decoy.Status, now time.Time) netaudit.MonitorReading {
	r := netaudit.MonitorReading{On: on, Limit: socketStallLimit}
	if st.Problem == "" && len(st.Listening) > 0 {
		r.LastGood = now
	}
	return r
}

// auditReading reads the daily history: the audit counts as in use when a day of the last month was audited, and as working
// until the end of the latest such day. A history that cannot be read raises nothing: it is not evidence of a stopped audit.
func auditReading(series func(int) ([]models.NetworkDay, error)) netaudit.MonitorReading {
	r := netaudit.MonitorReading{Limit: auditStallLimit}
	days, err := series(auditHistoryDays)
	if err != nil {
		return r
	}
	for _, d := range slices.Backward(days) {
		if !d.Audited {
			continue
		}
		day, err := time.ParseInLocation(time.DateOnly, d.Date, time.Local)
		if err != nil {
			continue
		}
		r.On, r.LastGood = true, day.AddDate(0, 0, 1)
		return r
	}
	return r
}
