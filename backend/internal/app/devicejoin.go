package app

import (
	"context"
	"net/netip"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/devicejoin"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// The join of the network inventory with what Radio heard over the air (ADR-0061) is wired here and nowhere else: the Network page reads its
// facts, the presence check's alert its note, and Radio's findings are mapped to devices with it. Neither stack sees the other; each side may be
// off, and then the join says so and adds nothing.

// newDeviceJoin builds the join over the inventory and the Radio host; a Radio stack that could not be hosted leaves the air off.
func newDeviceJoin(inventory func() (models.DeviceInventory, error), radio *stackHost, vault stackport.Vault, asked *stackport.AskedPurposes) *devicejoin.Join {
	deps := devicejoin.Deps{Inventory: inventory, Purposes: asked.List, Tag: vault.Tag,
		OnError: func(err error) { logging.Warn("network radio join", "error", err.Error()) }}
	if radio != nil {
		deps.Air = radio.Air
	}
	j := devicejoin.New(deps)
	if radio != nil {
		radio.tags = joinTags(j)
	}
	return j
}

// joinTags maps the stack's tags to the inventory devices' addresses on this network, for its findings by tag.
func joinTags(j *devicejoin.Join) func(ctx context.Context) (map[string]netip.Addr, error) {
	return func(ctx context.Context) (map[string]netip.Addr, error) {
		idx, err := j.Index(ctx)
		if err != nil {
			return nil, err
		}
		out := map[string]netip.Addr{}
		for tag, d := range idx {
			if a, err := netip.ParseAddr(d.IP); err == nil {
				out[tag] = a
			}
		}
		return out, nil
	}
}

// devicePresence is what the presence check says of a device now: unknown while the check is off or has not looked lately.
func devicePresence(config *netaudit.PresenceConfig, check *netaudit.Presence) func(models.NetworkDevice) models.DevicePresence {
	return func(d models.NetworkDevice) models.DevicePresence {
		if config == nil || check == nil {
			return models.PresenceUnknown
		}
		return netaudit.PresenceOf(d, config.Settings().Enabled, check.Status(), time.Now())
	}
}

// airNote is the new-device alert's fixed note about where the air heard the devices; empty until the join is wired or when Radio is not
// listening.
func (s *AppServices) airNote(ctx context.Context, macs []string) string {
	if s.netaudit == nil || s.netaudit.Join == nil {
		return ""
	}
	return devicejoin.NewDeviceNote(s.netaudit.Join.View(ctx), macs, time.Now())
}

// Air pulls the stack's air report (ADR-0061), held to what the manifest announced: a stack that does not report the air, or is not hosted, is
// unavailable, which the join reads as "off".
func (h *stackHost) Air(ctx context.Context) (stacksdk.AirReport, error) {
	if h == nil || !h.stack.Manifest().ReportsAir {
		return stacksdk.AirReport{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "no stack reports the air")
	}
	r, ok := h.stack.(stacksdk.AirReporter)
	if !ok {
		return stacksdk.AirReport{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the stack does not report the air")
	}
	return r.Air(ctx)
}
