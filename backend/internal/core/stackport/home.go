package stackport

import (
	"context"
	"encoding/json"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/radiodigest"
	"llm-proxy/models"
)

// Home is the SDK's Home port over what the core already knows (ADR-0077): the device inventory's counts and the Radio digest's finding ids. It hands the
// stack a count and a sentence from this file's table or from radiodigest's table, and nothing a device, a frame or a resolver sent: no address, hardware
// address, name, network name or tag is read into a line. Nothing here writes, sends or starts a look.
type Home struct {
	// Devices reads the stored inventory without refreshing it.
	Devices func() (models.DeviceInventory, error)
	// Presence says whether the network sees a device now; unknown while the presence check is off or has not looked lately.
	Presence func(models.NetworkDevice) models.DevicePresence
	// RadioOn says whether the Radio stack is switched on; Radio reads its digest answer.
	RadioOn func(ctx context.Context) bool
	Radio   func(ctx context.Context) (json.RawMessage, error)
	Now     func() time.Time
}

// maxHomeCount clamps a count: a bound on what the stack is told, not on what exists.
const maxHomeCount = 100_000

// The ids of the network facts and the fixed words for each. The radio ids and words are radiodigest's.
const (
	HomeNetworkOnline   = "network_online"
	HomeNetworkNotMine  = "network_not_marked_mine"
	HomeNetworkChanges  = "network_address_changes"
	HomeNetworkUnknown  = "network_presence_unknown"
	HomeNetworkOff      = "network_unavailable"
	HomeRadioOff        = "radio_off"
	HomeRadioQuiet      = "radio_quiet"
	HomeRadioUnreadable = "radio_unavailable"
)

var homeWords = map[string]string{
	HomeNetworkOnline:   "Devices the network sees right now.",
	HomeNetworkNotMine:  "Devices on your network that you have not marked as yours.",
	HomeNetworkChanges:  "Addresses that changed hands lately.",
	HomeNetworkUnknown:  "The presence check is off or has not looked lately, so who is online is not known.",
	HomeNetworkOff:      "The network list could not be read.",
	HomeRadioOff:        "Radio is off, so nothing is said about the air.",
	HomeRadioQuiet:      "Radio found nothing to report in its last listen.",
	HomeRadioUnreadable: "Radio could not be read.",
}

// HomeWords is the core's sentence for a home fact id, from this file's table or Radio's; false for an id the core has no words for.
func HomeWords(id string) (string, bool) {
	if w, ok := homeWords[id]; ok {
		return w, true
	}
	w, _, ok := radiodigest.Words(id)
	return w, ok
}

func homeCount(n int) int { return max(0, min(n, maxHomeCount)) }

func homeLine(id string, count int) stacksdk.HomeLine {
	return stacksdk.HomeLine{ID: id, Count: homeCount(count), Words: homeWords[id]}
}

// Lines is the answer: the network's facts, then Radio's, at most stacksdk.MaxHomeLines. A part that cannot be read gives one fixed line, not an error, so
// the other part still shows.
func (h Home) Lines(ctx context.Context) ([]stacksdk.HomeLine, error) {
	out := h.network()
	out = append(out, h.radio(ctx)...)
	if len(out) > stacksdk.MaxHomeLines {
		out = out[:stacksdk.MaxHomeLines]
	}
	return out, nil
}

func (h Home) network() []stacksdk.HomeLine {
	if h.Devices == nil {
		return []stacksdk.HomeLine{homeLine(HomeNetworkOff, 0)}
	}
	inv, err := h.Devices()
	if err != nil {
		return []stacksdk.HomeLine{homeLine(HomeNetworkOff, 0)}
	}
	var online, known, notMine int
	for _, d := range inv.Devices {
		if !d.Approved {
			notMine++
		}
		switch h.presence(d) {
		case models.PresenceOnline:
			online++
			known++
		case models.PresenceAway:
			known++
		}
	}
	var out []stacksdk.HomeLine
	if len(inv.Devices) > 0 && known == 0 {
		out = append(out, homeLine(HomeNetworkUnknown, 0))
	} else {
		out = append(out, homeLine(HomeNetworkOnline, online))
	}
	if notMine > 0 {
		out = append(out, homeLine(HomeNetworkNotMine, notMine))
	}
	if n := len(inv.Changes); n > 0 {
		out = append(out, homeLine(HomeNetworkChanges, n))
	}
	return out
}

func (h Home) presence(d models.NetworkDevice) models.DevicePresence {
	if h.Presence == nil {
		return models.PresenceUnknown
	}
	return h.Presence(d)
}

func (h Home) radio(ctx context.Context) []stacksdk.HomeLine {
	if h.RadioOn == nil || h.Radio == nil || !h.RadioOn(ctx) {
		return []stacksdk.HomeLine{homeLine(HomeRadioOff, 0)}
	}
	raw, err := h.Radio(ctx)
	if err != nil {
		return []stacksdk.HomeLine{homeLine(HomeRadioUnreadable, 0)}
	}
	found, err := radiodigest.Findings(raw)
	if err != nil {
		return []stacksdk.HomeLine{homeLine(HomeRadioUnreadable, 0)}
	}
	if len(found) == 0 {
		return []stacksdk.HomeLine{homeLine(HomeRadioQuiet, 0)}
	}
	out := make([]stacksdk.HomeLine, len(found))
	for i, f := range found {
		out[i] = stacksdk.HomeLine{ID: f.ID, Count: homeCount(f.Count), Words: f.Words}
	}
	return out
}
