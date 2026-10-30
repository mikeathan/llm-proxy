package netaudit

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"llm-proxy/models"
)

// Active IPv6 discovery (Constitution I.7). The host's IPv6 neighbour table lists only devices it has already talked to, so
// the inventory can ask each link that holds an in-scope IPv4 device to announce itself once (see neighbors.Discover6) and
// read the table again. It is off until the operator switches it on, adds no finding type, and never scans an IPv6 address:
// the declared scan scope stays IPv4.

// ErrDiscoveryNotPermitted means the host's settings do not let this program send the echo; ErrDiscoveryUnavailable means
// this computer cannot (not Linux). The adapter in the app turns the platform's errors into these.
var (
	ErrDiscoveryNotPermitted = errors.New("this computer does not allow this program to send the IPv6 echo")
	ErrDiscoveryUnavailable  = errors.New("this computer cannot look for IPv6 devices (only Linux can)")
)

// The problem codes the screen turns into words and a fix. A code, not text, so nothing a device sent can be in it.
const (
	Discovery6NotPermitted = "not_permitted"
	Discovery6Unavailable  = "unavailable"
	Discovery6Failed       = "failed"
)

// Discoverer6 makes the devices on the named links announce themselves: *neighbors.Discover6 behind the app's adapter.
type Discoverer6 interface {
	Discover(ctx context.Context, interfaces []string) error
}

// WithDiscovery6 gives the inventory its active IPv6 discovery, run on each look while enabled reports true.
func (v *Inventory) WithDiscovery6(d Discoverer6, enabled func() bool) *Inventory {
	v.disc, v.discOn = d, enabled
	return v
}

// Discovery6Problem is why the last discovery did not work (one of the Discovery6 codes), or "" when it worked or is off.
func (v *Inventory) Discovery6Problem() string {
	v.discMu.Lock()
	defer v.discMu.Unlock()
	return v.discProblem
}

func (v *Inventory) setDiscovery6Problem(code string) {
	v.discMu.Lock()
	v.discProblem = code
	v.discMu.Unlock()
}

// discover runs the echo on the links that hold in-scope IPv4 devices and reads the table again. A refused echo is a stated
// problem and the look goes on with what the first read found: the IPv4 devices are no less seen for it.
func (v *Inventory) discover(ctx context.Context, table NeighborTable) NeighborTable {
	if v.disc == nil || !v.discOn() {
		v.setDiscovery6Problem("")
		return table
	}
	links := linksOf(scopedEntries(table.Entries, v.inScope))
	if len(links) == 0 {
		return table
	}
	if err := v.disc.Discover(ctx, links); err != nil {
		v.noteDiscoveryError(ctx, err)
		return table
	}
	after, err := v.src.Neighbors(ctx)
	if err != nil {
		v.noteDiscoveryError(ctx, fmt.Errorf("failed to read the table after the echo: %w", err))
		return table
	}
	v.setDiscovery6Problem("")
	after.Discovered6 = true
	return after
}

func (v *Inventory) noteDiscoveryError(ctx context.Context, err error) {
	switch {
	case ctx.Err() != nil: // shutting down, not a problem to show
	case errors.Is(err, ErrDiscoveryNotPermitted):
		v.setDiscovery6Problem(Discovery6NotPermitted)
	case errors.Is(err, ErrDiscoveryUnavailable):
		v.setDiscovery6Problem(Discovery6Unavailable)
	default:
		v.setDiscovery6Problem(Discovery6Failed)
	}
}

// linksOf lists the network interfaces the entries were learned on, once each, in name order.
func linksOf(entries []Neighbor) []string {
	var out []string
	for _, e := range entries {
		if e.Interface != "" && !slices.Contains(out, e.Interface) {
			out = append(out, e.Interface)
		}
	}
	slices.Sort(out)
	return out
}

// Discovery6Config reads and changes whether active IPv6 discovery is on.
type Discovery6Config struct{ store PolicyStore }

func NewDiscovery6Config(store PolicyStore) *Discovery6Config { return &Discovery6Config{store: store} }

// Enabled reports whether the discovery is switched on.
func (c *Discovery6Config) Enabled() bool { return c.store.Get().IPv6Discovery }

// Set switches the discovery on or off.
func (c *Discovery6Config) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.IPv6Discovery = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the IPv6 discovery setting: %w", err)
	}
	return nil
}
