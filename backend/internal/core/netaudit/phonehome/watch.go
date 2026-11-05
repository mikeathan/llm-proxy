// Package phonehome remembers which DNS names each device on the network asks for and says when something about it is new: a name
// a device did not ask for during its first week, a new name asked for at a steady beat, and a device that is online and asks for
// nothing (it uses a resolver of its own). It reads only what the router's log reduced to a Query, keys every device by hardware
// address, and keeps names as plain text that is shown and never sent to a model (Constitution I.8, once the DNS lines are approved).
package phonehome

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

// The bounds. Each is a judgement, not a measurement: the router's real query rate has not been captured yet, so the lists are
// kept small and a flood of forged names can only push the oldest out.
const (
	// LearnFor is how long a device's names are only learned: every name it asks for in its first week is "normal".
	LearnFor = 7 * 24 * time.Hour
	// NewFor is how long a name asked for after the learning week still counts as new.
	NewFor = 7 * 24 * time.Hour
	// maxDevices and maxNames bound what is kept; past them the least recently seen goes first.
	maxDevices = 256
	maxNames   = 512
	// maxTimes is how many recent lookup times a new name keeps, and minGap how far apart two of them must be (a burst is one).
	maxTimes = 32
	minGap   = 5 * time.Second
	// dupWindow is how close two lookups of one name are to be one lookup: the router logs an A and an AAAA line for each.
	dupWindow = 2 * time.Second
	// maxRecent bounds the table that remembers the last lookup of each device and name for the duplicate test.
	maxRecent = 1024
)

// Store keeps the names. A missing record is an empty one.
type Store interface {
	Read() (models.PhoneHome, error)
	Write(models.PhoneHome) error
}

type recentKey struct{ mac, name string }

// Watch is the memory of the names. Observe only changes memory; Flush writes it.
type Watch struct {
	store Store

	mu     sync.Mutex
	state  models.PhoneHome
	recent map[recentKey]time.Time
	dirty  bool
}

// New reads the stored names. A record that cannot be read is an error, never an empty record that reads as "nothing new".
func New(store Store) (*Watch, error) {
	state, err := store.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read the phone-home names: %w", err)
	}
	return &Watch{store: store, state: state, recent: map[recentKey]time.Time{}}, nil
}

// NewEmpty is a watch that starts with nothing, for when the stored names could not be read: the unreadable file has been set aside,
// and a watch that remembered nothing is better than one that cannot start. The caller reports the read error.
func NewEmpty(store Store) *Watch {
	return &Watch{store: store, recent: map[recentKey]time.Time{}}
}

// Observe records that the device with this hardware address asked for a name at a time. A lookup with no usable hardware address is
// not kept, and the second line of an A and AAAA pair is not counted again.
func (w *Watch) Observe(mac string, q routerlog.Query, at time.Time) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil || len(hw) != 6 {
		return
	}
	mac = strings.ToLower(hw.String())
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.duplicate(recentKey{mac, q.Name}, at) {
		return
	}
	if w.state.Since.IsZero() {
		w.state.Since = at
	}
	dev := w.device(mac, at)
	if at.After(dev.Last) {
		dev.Last = at
	}
	noteName(dev, q.Name, at)
	w.dirty = true
}

// duplicate says whether this lookup is the twin of one just counted, and remembers it.
func (w *Watch) duplicate(k recentKey, at time.Time) bool {
	if last, ok := w.recent[k]; ok && at.Sub(last) < dupWindow && !at.Before(last) {
		return true
	}
	if len(w.recent) >= maxRecent {
		clear(w.recent)
	}
	w.recent[k] = at
	return false
}

// device returns the record of a device, adding it and dropping the quietest when the list is full.
func (w *Watch) device(mac string, at time.Time) *models.PhoneHomeDevice {
	for i := range w.state.Devices {
		if w.state.Devices[i].MAC == mac {
			return &w.state.Devices[i]
		}
	}
	if len(w.state.Devices) >= maxDevices {
		oldest := 0
		for i := range w.state.Devices {
			if w.state.Devices[i].Last.Before(w.state.Devices[oldest].Last) {
				oldest = i
			}
		}
		w.state.Devices = slices.Delete(w.state.Devices, oldest, oldest+1)
	}
	w.state.Devices = append(w.state.Devices, models.PhoneHomeDevice{MAC: mac, First: at})
	return &w.state.Devices[len(w.state.Devices)-1]
}

// noteName counts one lookup of a name, keeps its recent times while it is new, and drops the name asked for longest ago when full.
func noteName(dev *models.PhoneHomeDevice, name string, at time.Time) {
	i := slices.IndexFunc(dev.Names, func(n models.PhoneHomeName) bool { return n.Name == name })
	if i < 0 {
		if len(dev.Names) >= maxNames {
			dev.Names = slices.Delete(dev.Names, quietest(dev.Names), quietest(dev.Names)+1)
		}
		dev.Names = append(dev.Names, models.PhoneHomeName{Name: name, First: at})
		i = len(dev.Names) - 1
	}
	n := &dev.Names[i]
	n.Count++
	if at.After(n.Last) {
		n.Last = at
	}
	if isNew(*dev, *n) && (len(n.Times) == 0 || at.Sub(n.Times[len(n.Times)-1]) >= minGap) {
		n.Times = append(n.Times, at)
		if len(n.Times) > maxTimes {
			n.Times = n.Times[len(n.Times)-maxTimes:]
		}
	}
}

func quietest(names []models.PhoneHomeName) int {
	oldest := 0
	for i := range names {
		if names[i].Last.Before(names[oldest].Last) {
			oldest = i
		}
	}
	return oldest
}

// isNew says whether a name was first asked for after the device's learning week.
func isNew(dev models.PhoneHomeDevice, n models.PhoneHomeName) bool {
	return n.First.After(dev.First.Add(LearnFor))
}

// flushEvery is how often Run writes the names.
const flushEvery = 5 * time.Minute

// Run writes the names every few minutes, and once more when ctx ends, until then. onError hears a write that failed.
func (w *Watch) Run(ctx context.Context, onError func(error)) {
	tick := time.NewTicker(flushEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := w.Flush(); err != nil && onError != nil {
				onError(err)
			}
			return
		case <-tick.C:
			if err := w.Flush(); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

// Flush writes the names if anything changed since the last write. A failed write keeps the change for the next try.
func (w *Watch) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.dirty {
		return nil
	}
	if err := w.store.Write(clone(w.state)); err != nil {
		return fmt.Errorf("failed to save the phone-home names: %w", err)
	}
	w.dirty = false
	return nil
}

// Snapshot is a copy of the names, safe to read while the watch goes on.
func (w *Watch) Snapshot() models.PhoneHome {
	w.mu.Lock()
	defer w.mu.Unlock()
	return clone(w.state)
}

func clone(s models.PhoneHome) models.PhoneHome {
	out := models.PhoneHome{Since: s.Since, Devices: make([]models.PhoneHomeDevice, len(s.Devices))}
	for i, d := range s.Devices {
		d.Names = slices.Clone(d.Names)
		for j := range d.Names {
			d.Names[j].Times = slices.Clone(d.Names[j].Times)
		}
		out.Devices[i] = d
	}
	return out
}
