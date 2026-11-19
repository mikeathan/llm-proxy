package reminders

import (
	"fmt"
	"slices"
	"strings"
)

// InAppChannel is the channel of a reminder that is shown in the web UI only: it
// is never sent to a connector. It is a valid explicit choice, and it is what a
// reminder falls back to when no connector applies. Every reminder is in the
// web UI list whatever else it is sent to.
const InAppChannel = "in-app"

// ChannelPolicy is the one place that decides where a reminder goes. What is
// stored on a reminder is the user's choice: a connector type, InAppChannel, or ""
// for "follow the default". Resolve turns that into the connectors actually used,
// at send and display time, so changing the settings applies to reminders already
// waiting.
type ChannelPolicy struct {
	// Connectors returns the connector types that are configured and enabled.
	Connectors func() []string
	// Skipped returns the connector types the owner ticked off for reminders.
	Skipped func() []string
}

// Resolve is the connectors a reminder with this stored choice is delivered to; none
// means the web UI only. An unset choice means every enabled connector the owner has
// not skipped, so a connector that is switched on gets reminders until it is ticked
// off, and one switched on later is included without further setup.
func (p ChannelPolicy) Resolve(stored string) []string {
	switch stored {
	case InAppChannel:
		return nil
	case "":
		skipped := p.Skipped()
		var out []string
		for _, c := range p.Connectors() {
			if !slices.Contains(skipped, c) {
				out = append(out, c)
			}
		}
		return out
	}
	return []string{stored}
}

// ChannelLabel names where a reminder goes, for people: "in-app" for none, else the
// connector types ("telegram, slack").
func ChannelLabel(connectors []string) string {
	if len(connectors) == 0 {
		return InAppChannel
	}
	return strings.Join(connectors, ", ")
}

// ForNew is the choice to store on a new reminder. A channel the user named wins;
// otherwise a request that arrived through a chat app is answered on that app, as
// long as it is still configured; otherwise "" (follow the default).
func (p ChannelPolicy) ForNew(named, origin string) string {
	if named != "" {
		return named
	}
	if origin != "" && p.configured(origin) {
		return origin
	}
	return ""
}

// ValidateNamed refuses a channel the user named that cannot be used. "" and
// InAppChannel are always acceptable.
func (p ChannelPolicy) ValidateNamed(named string) error {
	if named == "" || named == InAppChannel || p.configured(named) {
		return nil
	}
	return fmt.Errorf("no %q connector is configured: enable it in Settings or choose another channel", named)
}

func (p ChannelPolicy) configured(connector string) bool {
	return slices.Contains(p.Connectors(), connector)
}

// AlertsWebUI reports whether the web UI should be told about the reminder's last outcome. A failure or a miss always
// is: the web UI is the only place those are reported. A delivery is only when the web UI was one of the places it went,
// which is the default and the in-app choice, not a reminder sent to one named connector.
func (r Reminder) AlertsWebUI() bool {
	switch r.Event {
	case "":
		return false
	case EventFailed, EventMissed:
		return true
	}
	return r.Connector == "" || r.Connector == InAppChannel
}
