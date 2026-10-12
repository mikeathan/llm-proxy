package reminders

import "fmt"

// InAppChannel is the channel of a reminder that is shown in the web UI only: it
// is never sent to a connector. It is a valid explicit choice, and it is what a
// reminder falls back to when nothing else applies.
const InAppChannel = "in-app"

// ChannelPolicy is the one place that decides where a reminder goes. What is
// stored on a reminder is the user's choice: a connector type, InAppChannel, or ""
// for "follow the default". Resolve turns that into the channel actually used, at
// send and display time, so changing the default setting applies to reminders
// already waiting.
type ChannelPolicy struct {
	// Default is the default channel setting: a connector type, or "" for in-app only.
	Default func() string
	// Configured reports whether a connector type is configured and enabled.
	Configured func(connector string) bool
}

// Resolve is the channel a reminder with this stored choice is delivered to. A
// default that is no longer configured, or none at all, means in-app only: a
// connector is only ever used because it was named or chosen as the default,
// never because it happens to be available.
func (p ChannelPolicy) Resolve(stored string) string {
	if stored != "" {
		return stored
	}
	if def := p.Default(); def != "" && p.Configured(def) {
		return def
	}
	return InAppChannel
}

// ForNew is the choice to store on a new reminder. A channel the user named wins;
// otherwise a request that arrived through a chat app is answered on that app, as
// long as it is still configured; otherwise "" (follow the default).
func (p ChannelPolicy) ForNew(named, origin string) string {
	if named != "" {
		return named
	}
	if origin != "" && p.Configured(origin) {
		return origin
	}
	return ""
}

// ValidateNamed refuses a channel the user named that cannot be used. "" and
// InAppChannel are always acceptable.
func (p ChannelPolicy) ValidateNamed(named string) error {
	if named == "" || named == InAppChannel || p.Configured(named) {
		return nil
	}
	return fmt.Errorf("no %q connector is configured: enable it in Settings or choose another channel", named)
}
