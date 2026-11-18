package netaudit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/models"
)

// The router's list of joined Wi-Fi devices as a second input to presence (Constitution I.15, ADR-0062). The router hears every frame a
// phone sends, so a phone that dozes and does not answer the sweep is still listed as joined. Each presence look asks the router manager for the
// list (MeshClients: at most one read every 30 s however many ask, shared with a Radio watch, on a connection closed after 2 minutes idle) and
// marks the inventory's own devices it lists. It only ever makes a device online, never away; it adds no device and keeps nothing but a time.

// RouterClientsConfig reads and changes whether presence uses the router's list. It is off until the operator turns it on, and turning it on
// needs the passphrase (the module's step-up), because it makes each presence look sign in to the router.
type RouterClientsConfig struct{ store PolicyStore }

func NewRouterClientsConfig(store PolicyStore) *RouterClientsConfig {
	return &RouterClientsConfig{store: store}
}

// Enabled reports whether presence reads the router's list.
func (c *RouterClientsConfig) Enabled() bool { return c.store.Get().RouterClientsEnabled }

// Set switches the router's list on or off.
func (c *RouterClientsConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.RouterClientsEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the joined-devices setting: %w", err)
	}
	return nil
}

// RouterJoined is one presence look's use of the router's list. Read is the router manager's MeshClients; Mark is Inventory.MarkJoined.
type RouterJoined struct {
	Enabled func() bool
	Read    func(context.Context) (*models.MeshClients, error)
	Mark    func(macs []string, at time.Time) error
	// OnError is told of a failed read or mark, once per distinct message until a look works again.
	OnError func(error)
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	last *joinedState
}

// routerJoinedGap is the least time between two reads presence asks for. A look woken early (the router log, at most one a minute) does not
// read again sooner, so presence signs in to the router at most once per gap: at most 288 times a day (Constitution I.15).
const routerJoinedGap = 5 * time.Minute

// joinedState remembers the last read asked for and the trouble reported last, shared by the copies of a RouterJoined.
type joinedState struct {
	mu    sync.Mutex
	asked time.Time
	msg   string
}

// due says whether a read may be asked for now, and counts it when it may. A read that fails still counts, so a failing router is not asked
// more often.
func (r RouterJoined) due() bool {
	if r.last == nil {
		return true
	}
	now := r.Now()
	r.last.mu.Lock()
	defer r.last.mu.Unlock()
	if !r.last.asked.IsZero() && now.Sub(r.last.asked) < routerJoinedGap {
		return false
	}
	r.last.asked = now
	return true
}

// Look reads the list and marks the devices it names, guests left out. Switched off it reads nothing. A router that is not set up, or that was
// just read with no earlier list to answer with, is nothing to do; a failed read marks nothing, because a failure is never "nobody is joined".
func (r RouterJoined) Look(ctx context.Context) {
	if r.Enabled == nil || r.Read == nil || r.Mark == nil || !r.Enabled() || !r.due() {
		return
	}
	list, err := r.Read(ctx)
	if errors.Is(err, routerconn.ErrNotSet) || errors.Is(err, routerconn.ErrMeshWaiting) {
		return
	}
	if err == nil && list != nil {
		err = r.Mark(joinedMACs(list), list.At)
	} else if err == nil {
		return
	}
	r.trouble(err)
}

// joinedMACs are the hardware addresses the list names on the owner's own networks: a guest is a visitor's device (ADR-0059).
func joinedMACs(list *models.MeshClients) []string {
	out := make([]string, 0, len(list.Clients))
	for _, c := range list.Clients {
		if !c.Guest && c.MAC != "" {
			out = append(out, c.MAC)
		}
	}
	return out
}

// trouble reports err once until it changes; nil clears it, so the same failure after a good look is reported again.
func (r RouterJoined) trouble(err error) {
	if r.last == nil {
		return
	}
	r.last.mu.Lock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	same := msg == r.last.msg
	r.last.msg = msg
	r.last.mu.Unlock()
	if err != nil && !same && r.OnError != nil {
		r.OnError(fmt.Errorf("the router's list of joined devices: %w", err))
	}
}

// NewRouterJoined builds the look with its own memory of the last read and the last trouble.
func NewRouterJoined(j RouterJoined) RouterJoined {
	if j.Now == nil {
		j.Now = time.Now
	}
	j.last = &joinedState{}
	return j
}

// MarkJoined records that the router listed these devices as joined at at. Only devices already in the inventory are marked, and only their
// RouterSeen changes: the list adds no device and never moves LastSeen, which decides who holds an address. Nothing is written when no device
// changed.
func (v *Inventory) MarkJoined(macs []string, at time.Time) error {
	want := make(map[string]bool, len(macs))
	for _, m := range macs {
		if n, err := normalizeMAC(m); err == nil {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	inv, err := v.store.Read()
	if err != nil {
		return fmt.Errorf("failed to read the device inventory: %w", err)
	}
	changed := false
	for i := range inv.Devices {
		if want[inv.Devices[i].MAC] && at.After(inv.Devices[i].RouterSeen) {
			inv.Devices[i].RouterSeen = at
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return v.store.Write(inv)
}
