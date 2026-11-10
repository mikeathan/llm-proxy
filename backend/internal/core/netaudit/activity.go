package netaudit

import (
	"slices"
	"strconv"
	"strings"
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
	// ActivityRouterBlocks is a day on which the router's log said it refused connection attempts from the internet: a count and
	// the ports most tried. It is a hint from an unauthenticated log, never a finding.
	ActivityRouterBlocks = "router_blocks"
	// ActivityRelated says a new device appeared within an hour before a decoy service was touched. It states the timing and
	// nothing more: the alert does not name its source in a typed field, so the two may be unrelated.
	ActivityRelated = "related"
	// ActivityFixed is a finding a recheck found gone, and ActivityReturned one that came back after it had gone. Both are kept
	// in the finding's history; the row carries the device, port and check title and nothing a device wrote.
	ActivityFixed    = "finding_fixed"
	ActivityReturned = "finding_returned"
)

// Groups are the filters of the activity screen.
const (
	ActivityGroupDevices  = "devices"
	ActivityGroupPorts    = "ports"
	ActivityGroupFindings = "findings"
	ActivityGroupAlerts   = "alerts"
	ActivityGroupRouter   = "router"
)

// DefaultActivityRows is how many rows a request without a limit gets, and MaxActivityRows the most it can get.
const (
	DefaultActivityRows = 20
	MaxActivityRows     = 200
	// relatedWindow is how long before a decoy touch a new device still counts as appearing shortly before it.
	relatedWindow = time.Hour
	// RouterCardDays is how many days the router card draws.
	RouterCardDays = 14
	dayLayout      = historyDateLayout
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
	// Ports are the ports most tried on a router_blocks day, busiest first.
	Ports []models.PortCount `json:"ports,omitempty"`
	// Device, Port and Check name the finding of a finding_fixed or finding_returned row; Check is the check's plain title.
	Device string `json:"device,omitempty"`
	Port   int    `json:"port,omitempty"`
	Check  string `json:"check,omitempty"`
}

// BlockDay is one day's count of attempts the router refused.
type BlockDay struct {
	Date  string `json:"date"`
	Total int    `json:"total"`
}

// RouterBlocks is what the router card draws: today's count and busiest ports, and the last days' counts. It exists only once
// the router's log has been counted on some day, so a house whose router does not send its log sees no card.
type RouterBlocks struct {
	Today      int                `json:"today"`
	TodayPorts []models.PortCount `json:"today_ports,omitempty"`
	Days       []BlockDay         `json:"days"`
}

// Activity is the Network page's timeline: the records the program already keeps, as one list.
type Activity struct {
	Rows   []ActivityRow `json:"rows"`
	Router *RouterBlocks `json:"router,omitempty"`
}

// ActivityInput is what the timeline is built from; the caller reads each part from where it already lives.
type ActivityInput struct {
	Now       time.Time
	Inventory models.DeviceInventory
	Alerts    []models.NetworkAlert
	Series    []models.NetworkDay
	Baseline  models.ScanBaseline
	// Life is the history of each finding; a recheck's "fixed" and a return after it become rows. Optional.
	Life []models.FindingLife
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
	rows = append(rows, blockRows(in.Series, in.Now)...)
	if r, ok := auditRow(in.Baseline); ok {
		rows = append(rows, r)
	}
	rows = append(rows, lifeRows(in.Life, in.Now)...)
	slices.SortStableFunc(rows, func(a, b ActivityRow) int { return b.At.Compare(a.At) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return Activity{Rows: rows, Router: routerCard(in.Series, in.Now)}
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

// blockRows is one row per day on which the router refused attempts. A day's time is the end of that day, and today's is now.
func blockRows(series []models.NetworkDay, now time.Time) []ActivityRow {
	var out []ActivityRow
	for _, d := range series {
		if d.Blocked <= 0 {
			continue
		}
		out = append(out, ActivityRow{Kind: ActivityRouterBlocks, Group: ActivityGroupRouter, At: endOfDay(d.Date, now), Day: true, Count: d.Blocked, Ports: d.BlockedPorts})
	}
	return out
}

// routerCard is the router card, or nil when no day in the series has a count.
func routerCard(series []models.NetworkDay, now time.Time) *RouterBlocks {
	var days []BlockDay
	any := false
	card := &RouterBlocks{}
	today := now.Format(dayLayout)
	for _, d := range series {
		if d.Blocked > 0 {
			any = true
		}
		days = append(days, BlockDay{Date: d.Date, Total: d.Blocked})
		if d.Date == today {
			card.Today, card.TodayPorts = d.Blocked, d.BlockedPorts
		}
	}
	if !any {
		return nil
	}
	if len(days) > RouterCardDays {
		days = days[len(days)-RouterCardDays:]
	}
	card.Days = days
	return card
}

// endOfDay is the last second of a YYYY-MM-DD day, or now if that is still ahead; a day that cannot be read is now.
func endOfDay(date string, now time.Time) time.Time {
	day, err := time.ParseInLocation(dayLayout, date, now.Location())
	if err != nil {
		return now
	}
	if end := day.AddDate(0, 0, 1).Add(-time.Second); end.Before(now) {
		return end
	}
	return now
}

// portRows is one row per day on which a port opened or closed. A day's time is the end of that day, and today's is now, so
// no row is in the future.
func portRows(series []models.NetworkDay, now time.Time) []ActivityRow {
	var out []ActivityRow
	for _, d := range series {
		if d.PortsOpened == 0 && d.PortsClosed == 0 {
			continue
		}
		out = append(out, ActivityRow{Kind: ActivityPortsChanged, Group: ActivityGroupPorts, At: endOfDay(d.Date, now), Day: true, Opened: d.PortsOpened, Closed: d.PortsClosed})
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

// lifeRows are the findings the operator's recheck found gone, and those that came back, within the recent window.
func lifeRows(life []models.FindingLife, now time.Time) []ActivityRow {
	var out []ActivityRow
	for _, l := range life {
		device, port, check, ok := splitFindingKey(l.Key)
		if !ok {
			continue
		}
		row := ActivityRow{Group: ActivityGroupFindings, Device: device, Port: port, Check: titleOrID(check)}
		switch {
		case !l.Open() && l.ResolvedBy == models.LifeByRecheck && now.Sub(l.ResolvedAt) <= recentWindow:
			row.Kind, row.At = ActivityFixed, l.ResolvedAt
		case l.Open() && l.Returned > 0 && now.Sub(l.OpenSince) <= recentWindow:
			row.Kind, row.At, row.Count = ActivityReturned, l.OpenSince, l.Returned
		default:
			continue
		}
		out = append(out, row)
	}
	return out
}

// splitFindingKey reads a Finding.Key ("device|proto|port|check") back into its parts.
func splitFindingKey(key string) (device string, port int, check string, ok bool) {
	parts := strings.SplitN(key, "|", 4)
	if len(parts) != 4 {
		return "", 0, "", false
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, "", false
	}
	return parts[0], n, parts[3], true
}

// titleOrID is a check's plain title, or its id when it has none (an open port).
func titleOrID(check string) string {
	if t := CheckTitle(check); t != "" {
		return t
	}
	return check
}
