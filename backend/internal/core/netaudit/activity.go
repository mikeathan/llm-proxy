package netaudit

import (
	"slices"
	"time"

	"llm-proxy/models"
)

// Kinds of activity row. The screen composes the words from the kind and its typed fields, so nothing a device printed is in
// a row. An alert row also carries the alert's own text, which the program wrote and which the bell already shows.
const (
	ActivityNewDevice      = "new_device"
	ActivityAddressChanged = "address_changed"
	ActivityAlert          = "alert"
	ActivityPortsChanged   = "ports_changed"
	ActivityAudit          = "audit"
	// ActivityRelated says a new device appeared within an hour before a decoy service was touched. It states the timing and
	// nothing more: the alert does not name its source in a typed field, so the two may be unrelated.
	ActivityRelated = "related"
)

// Groups are the filters of the activity screen.
const (
	ActivityGroupDevices  = "devices"
	ActivityGroupPorts    = "ports"
	ActivityGroupFindings = "findings"
	ActivityGroupAlerts   = "alerts"
)

// DefaultActivityRows is how many rows a request without a limit gets, and MaxActivityRows the most it can get.
const (
	DefaultActivityRows = 20
	MaxActivityRows     = 200
	// relatedWindow is how long before a decoy touch a new device still counts as appearing shortly before it.
	relatedWindow = time.Hour
	dayLayout     = historyDateLayout
)

// ActivityRow is one thing that happened. Day is true for a row that stands for a whole day (its time is the end of that
// day, or now), and the screen then shows the date and not the time.
type ActivityRow struct {
	Kind      string    `json:"kind"`
	Group     string    `json:"group"`
	At        time.Time `json:"at"`
	Day       bool      `json:"day,omitempty"`
	MAC       string    `json:"mac,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Name      string    `json:"name,omitempty"`
	AlertKind string    `json:"alert_kind,omitempty"`
	Text      string    `json:"text,omitempty"`
	Count     int       `json:"count,omitempty"`
	Opened    int       `json:"opened,omitempty"`
	Closed    int       `json:"closed,omitempty"`
}

// Activity is the Network page's timeline: the records the program already keeps, as one list.
type Activity struct {
	Rows []ActivityRow `json:"rows"`
}

// ActivityInput is what the timeline is built from; the caller reads each part from where it already lives.
type ActivityInput struct {
	Now       time.Time
	Inventory models.DeviceInventory
	Alerts    []models.NetworkAlert
	Series    []models.NetworkDay
	Baseline  models.ScanBaseline
}

// ActivityOf builds the timeline, newest first and cut to limit rows (the default when limit is not positive, and never more
// than MaxActivityRows). Building it probes nothing and keeps nothing.
func ActivityOf(in ActivityInput, limit int) Activity {
	limit = boundedLimit(limit)
	rows := []ActivityRow{}
	for _, c := range allRecentChanges(in.Inventory, in.Now) {
		rows = append(rows, deviceRow(c))
	}
	for _, a := range in.Alerts {
		rows = append(rows, ActivityRow{Kind: ActivityAlert, Group: ActivityGroupAlerts, At: a.At, AlertKind: string(a.Kind), Text: a.Message, Count: max(a.Count, 1)})
	}
	rows = append(rows, relatedRows(in.Inventory, in.Alerts, in.Now)...)
	rows = append(rows, portRows(in.Series, in.Now)...)
	if r, ok := auditRow(in.Baseline); ok {
		rows = append(rows, r)
	}
	slices.SortStableFunc(rows, func(a, b ActivityRow) int { return b.At.Compare(a.At) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return Activity{Rows: rows}
}

func boundedLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultActivityRows
	case limit > MaxActivityRows:
		return MaxActivityRows
	}
	return limit
}

func deviceRow(c RecentChange) ActivityRow {
	kind := ActivityNewDevice
	if c.Kind == RecentAddressChanged {
		kind = ActivityAddressChanged
	}
	return ActivityRow{Kind: kind, Group: ActivityGroupDevices, At: c.At, MAC: c.MAC, IP: c.IP, Name: c.Name}
}

// relatedRows adds one row per decoy touch that a new device preceded by less than an hour. A device that arrived after the
// touch, or long before it, is not related.
func relatedRows(inv models.DeviceInventory, alerts []models.NetworkAlert, now time.Time) []ActivityRow {
	if !inv.Baselined {
		return nil
	}
	var out []ActivityRow
	for _, a := range alerts {
		if a.Kind != models.NetworkAlertDecoy {
			continue
		}
		for _, d := range inv.Devices {
			if gap := a.At.Sub(d.FirstSeen); gap >= 0 && gap <= relatedWindow && now.Sub(d.FirstSeen) <= recentWindow {
				out = append(out, ActivityRow{Kind: ActivityRelated, Group: ActivityGroupAlerts, At: a.At, MAC: d.MAC, IP: d.IP, Name: d.Name})
				break
			}
		}
	}
	return out
}

// portRows is one row per day on which a port opened or closed. A day's time is the end of that day, and today's is now, so
// no row is in the future.
func portRows(series []models.NetworkDay, now time.Time) []ActivityRow {
	var out []ActivityRow
	for _, d := range series {
		if d.PortsOpened == 0 && d.PortsClosed == 0 {
			continue
		}
		day, err := time.ParseInLocation(dayLayout, d.Date, now.Location())
		if err != nil {
			continue
		}
		at := day.AddDate(0, 0, 1).Add(-time.Second)
		if at.After(now) {
			at = now
		}
		out = append(out, ActivityRow{Kind: ActivityPortsChanged, Group: ActivityGroupPorts, At: at, Day: true, Opened: d.PortsOpened, Closed: d.PortsClosed})
	}
	return out
}

// auditRow is the last audit, and only when it found something new or saw something go.
func auditRow(b models.ScanBaseline) (ActivityRow, bool) {
	if b.ScannedAt.IsZero() || (len(b.NewKeys) == 0 && len(b.Resolved) == 0) {
		return ActivityRow{}, false
	}
	return ActivityRow{Kind: ActivityAudit, Group: ActivityGroupFindings, At: b.ScannedAt, Opened: len(b.NewKeys), Closed: len(b.Resolved)}, true
}
