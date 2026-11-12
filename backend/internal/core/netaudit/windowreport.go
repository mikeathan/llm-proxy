package netaudit

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// WindowSource builds the window digest of an audit run. DigestSource satisfies it.
type WindowSource interface {
	Build(ctx context.Context, workspaceID, automation string, current *CurrentScan) (WindowDigest, bool)
}

// writeWindow adds how the network moved over the audit's window, from the same digest the model reads, so every number and port
// in it is written by code and the model describes no movement itself. The records end at the scan before this one; what this
// scan shows already corrected them (see CurrentScan). Every value from the network is escaped.
func writeWindow(b *strings.Builder, d *WindowDigest) {
	if d == nil || (d.Scans.Count == 0 && d.AsOf.IsZero()) {
		return
	}
	fmt.Fprintf(b, "\n### Last %s before this scan\n\n%s\n", d.Window, windowSummary(d))
	for _, c := range windowChanges(d) {
		fmt.Fprintf(b, "\n- %s: %s.", c.label, cell(c.text))
	}
	if d.OpenAllWindow > 0 {
		fmt.Fprintf(b, "\n- %d finding(s) were already open when the window began and still are.", d.OpenAllWindow)
	}
	if line := activityText(d.Activity); line != "" {
		fmt.Fprintf(b, "\n- Activity: %s.", line)
	}
	for _, m := range d.Modules {
		fmt.Fprintf(b, "\n- %s", cell(m.Line()))
	}
	if len(d.Unread) > 0 {
		fmt.Fprintf(b, "\n- Not read, so not counted: %s.", cell(strings.Join(d.Unread, ", ")))
	}
	b.WriteString("\n")
}

// windowSummary is the first line of the section: how many scans the log holds for the window and what they showed.
func windowSummary(d *WindowDigest) string {
	if d.Scans.Count == 0 {
		return fmt.Sprintf("The scan log has no scan in this window; the latest saved scan is from %s.", d.AsOf.UTC().Format(timeLayout))
	}
	return fmt.Sprintf("%d scan(s) recorded: devices up %s, open ports %s. The records end at the scan of %s.",
		d.Scans.Count, rangeText(d.Scans.DevicesUp), rangeText(d.Scans.OpenPorts), d.AsOf.UTC().Format(timeLayout))
}

type windowChange struct{ label, text string }

// windowChanges are the lists of what went away, came back or appeared inside the window, each cut as the digest cut it.
func windowChanges(d *WindowDigest) []windowChange {
	lists := []struct {
		label string
		rows  []DigestChange
		more  int
	}{
		{"Went away", d.GoneInWindow, d.GoneMore},
		{"Came back", d.ReturnedInWindow, d.ReturnedMore},
		{"Appeared", d.NewInWindow, d.NewMore},
	}
	var out []windowChange
	for _, l := range lists {
		if len(l.rows) == 0 {
			continue
		}
		items := make([]string, 0, len(l.rows)+1)
		for _, r := range l.rows {
			items = append(items, changeWhere(r))
		}
		if l.more > 0 {
			items = append(items, fmt.Sprintf("and %d more", l.more))
		}
		out = append(out, windowChange{l.label, strings.Join(items, "; ")})
	}
	return out
}

func rangeText(r DigestRange) string {
	if r.Min == r.Max {
		return fmt.Sprint(r.Min)
	}
	return fmt.Sprintf("%d to %d", r.Min, r.Max)
}

// changeWhere is "192.0.2.10 port 22 (open port)" or "192.0.2.10 (the firewall is switched off)": where, then what.
func changeWhere(c DigestChange) string {
	what := c.Check
	if what == CheckOpenPort || what == titleOrID(CheckOpenPort) {
		what = "open port"
	}
	if c.Port > 0 {
		return fmt.Sprintf("%s port %d (%s)", c.Device, c.Port, what)
	}
	return fmt.Sprintf("%s (%s)", c.Device, what)
}

func activityText(a DigestActivity) string {
	var parts []string
	add := func(n int, text string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, text))
		}
	}
	add(a.NewDevices, "new device(s)")
	add(a.AddressChanges, "address change(s)")
	add(a.PortsOpened, "port(s) opened")
	add(a.PortsClosed, "port(s) closed")
	add(a.RouterBlocked, "connection attempt(s) the router refused")
	kinds := make([]string, 0, len(a.Alerts))
	for k := range a.Alerts {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s alert(s)", a.Alerts[k], cell(k)))
	}
	return strings.Join(parts, ", ")
}
