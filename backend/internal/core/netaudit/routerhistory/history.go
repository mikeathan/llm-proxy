// Package routerhistory remembers, for each device on the network, which companies and countries its connections went to, as read from the
// router's connection table (Constitution I.15), and says when one is new: first seen after the device's first day of history and within the
// last day. It keeps no address, port or connection; every list is bounded; and the names are public-table text that is shown as text only.
package routerhistory

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"llm-proxy/models"
)

// The bounds. A home network is far below them; a flood can only push the least recently seen out.
const (
	// LearnFor is how long a device's destinations are only learned: everything it connects to in its first day is "normal".
	LearnFor = 24 * time.Hour
	// NewFor is how long a destination first seen after the learning day still counts as new.
	NewFor = 24 * time.Hour
	// MaxDevices, MaxCompanies and MaxCountries bound what is kept for the network and for one device.
	MaxDevices   = 128
	MaxCompanies = 64
	MaxCountries = 32
	// maxNames is how many names a signal lists.
	maxNames = 8
)

// Store keeps the history. A missing record is an empty one.
type Store interface {
	Read() (models.RouterHistory, error)
	Write(models.RouterHistory) error
}

// Kind says what is new about a device.
type Kind string

const (
	NewCompany Kind = "new_company"
	NewCountry Kind = "new_country"
)

// Signal is one thing that is new about one device: the companies or the countries first seen in the last day.
type Signal struct {
	Kind Kind
	// IP is the device's address at its last read; Key is how the history knows it (hardware address, else the address).
	IP, Key string
	Names   []string
}

// History is the memory. Observe changes it and writes it.
type History struct {
	store Store
	mu    sync.Mutex
	state models.RouterHistory
}

// New reads the stored history. A record that cannot be read is an error, never an empty record that reads as "nothing new".
func New(store Store) (*History, error) {
	state, err := store.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read the router connection history: %w", err)
	}
	if state.Devices == nil {
		state.Devices = map[string]models.RouterHistoryDevice{}
	}
	return &History{store: store, state: state}, nil
}

// NewEmpty is a history that starts with nothing, for a record that could not be read.
func NewEmpty(store Store) *History {
	return &History{store: store, state: models.RouterHistory{Devices: map[string]models.RouterHistoryDevice{}}}
}

// Delete forgets everything, in memory and on disk.
func (h *History) Delete() error {
	h.mu.Lock()
	h.state = models.RouterHistory{Devices: map[string]models.RouterHistoryDevice{}}
	h.mu.Unlock()
	if d, ok := h.store.(interface{ Delete() error }); ok {
		return d.Delete()
	}
	return nil
}

// isNew says whether a destination was first seen after the device's learning day and is still within the window.
func isNew(d models.RouterHistoryDevice, s models.RouterHistorySeen, now time.Time) bool {
	return s.First.After(d.First.Add(LearnFor)) && now.Sub(s.First) < NewFor
}

func companyKey(o *models.TrafficOwner) string { return "AS" + strconv.FormatUint(uint64(o.ASN), 10) }

func countryOf(o *models.TrafficOwner) string {
	if o.PlaceCountry != "" {
		return o.PlaceCountry
	}
	return o.Country
}

// Observe adds one read of the router's table to the history and marks, in the read, each place whose company or country is new for its
// device. key names a device by its address on this network: the hardware address when the inventory knows it, else the address itself. A
// failed write is returned and the memory keeps the read.
func (h *History) Observe(read *models.RouterConnections, key func(ip string) string, now time.Time) error {
	if read == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range read.Devices {
		dev := &read.Devices[i]
		k := key(dev.Address)
		d, known := h.state.Devices[k]
		if !known {
			d = models.RouterHistoryDevice{First: now}
		}
		d.IP, d.Last = dev.Address, now
		for j := range dev.Places {
			p := &dev.Places[j]
			if p.Owner == nil {
				continue
			}
			var newCompany, newCountry bool
			d.Companies, newCompany = see(d.Companies, companyKey(p.Owner), p.Owner.Name, MaxCompanies, d, now)
			if c := countryOf(p.Owner); c != "" {
				d.Countries, newCountry = see(d.Countries, c, c, MaxCountries, d, now)
			}
			p.New = newCompany || newCountry
		}
		h.state.Devices[k] = d
	}
	h.evictDevices()
	if err := h.store.Write(h.state); err != nil {
		return fmt.Errorf("failed to save the router connection history: %w", err)
	}
	return nil
}

// see records a destination and says whether it is new for the device (first seen after the learning day, within the window). The least
// recently seen goes first when the list is full.
func see(m map[string]models.RouterHistorySeen, key, name string, limit int, d models.RouterHistoryDevice, now time.Time) (map[string]models.RouterHistorySeen, bool) {
	if m == nil {
		m = map[string]models.RouterHistorySeen{}
	}
	s, ok := m[key]
	if !ok {
		s = models.RouterHistorySeen{Name: name, First: now}
	}
	s.Last = now
	m[key] = s
	if len(m) > limit {
		oldest, when := "", time.Time{}
		for k, v := range m {
			if k != key && (oldest == "" || v.Last.Before(when)) {
				oldest, when = k, v.Last
			}
		}
		delete(m, oldest)
	}
	return m, isNew(d, s, now)
}

// evictDevices drops the least recently seen devices past the bound.
func (h *History) evictDevices() {
	for len(h.state.Devices) > MaxDevices {
		oldest, when := "", time.Time{}
		for k, v := range h.state.Devices {
			if oldest == "" || v.Last.Before(when) {
				oldest, when = k, v.Last
			}
		}
		delete(h.state.Devices, oldest)
	}
}

// Signals are the devices with a company or a country that is new, in a fixed order. Each device has at most one signal of each kind.
func (h *History) Signals(now time.Time) []Signal {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Signal
	for _, k := range slices.Sorted(mapKeys(h.state.Devices)) {
		d := h.state.Devices[k]
		if s, ok := signal(NewCompany, k, d, d.Companies, now); ok {
			out = append(out, s)
		}
		if s, ok := signal(NewCountry, k, d, d.Countries, now); ok {
			out = append(out, s)
		}
	}
	return out
}

func signal(kind Kind, key string, d models.RouterHistoryDevice, seen map[string]models.RouterHistorySeen, now time.Time) (Signal, bool) {
	var names []string
	for _, s := range seen {
		if isNew(d, s, now) {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 || d.IP == "" {
		return Signal{}, false
	}
	slices.Sort(names)
	if len(names) > maxNames {
		names = names[:maxNames]
	}
	return Signal{Kind: kind, IP: d.IP, Key: key, Names: names}, true
}

func mapKeys[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
