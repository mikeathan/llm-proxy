// Package knownbad holds the list of host names that abuse.ch's URLhaus says serve malware, so the phone-home watch can tell the operator
// when a device asks for one. The list is public data; the address it is downloaded from holds the operator's key, so it lives in the
// secrets store and is never kept, logged or shown here. The list is matched on this computer: no name a device asked for leaves it.
package knownbad

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

const (
	// MaxNames bounds the list; the real one holds a few hundred. A larger file is refused, not cut.
	MaxNames = 100_000
	// maxBytes bounds what is read of a download.
	maxBytes = 8 << 20
	maxLine  = 1024
	// downloadLimit bounds one download.
	downloadLimit = time.Minute
	// allowedDomain is the only place the list may be fetched from.
	allowedDomain = "abuse.ch"
)

// blockAddresses are the addresses a host file points a blocked name at.
var blockAddresses = []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("::1"), netip.MustParseAddr("::")}

// Parse reads a host file: comment lines start with "#", an entry is "127.0.0.1 name" (or 0.0.0.0, ::1, ::) or a bare name. Every name
// is vetted like a name from a DNS query line; anything that does not pass, and any line that points a name at another address, is
// dropped. A file with no name is an error: an empty list must not read as "nothing is known to be bad".
func Parse(r io.Reader) ([]string, error) {
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	sc := bufio.NewScanner(limited)
	sc.Buffer(make([]byte, 0, 4096), maxLine)
	seen := map[string]bool{}
	var names []string
	for sc.Scan() {
		name, ok := entry(sc.Text())
		if !ok || seen[name] {
			continue
		}
		if len(names) >= MaxNames {
			return nil, fmt.Errorf("the list holds more than %d names", MaxNames)
		}
		seen[name] = true
		names = append(names, name)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("the list could not be read: %w", err)
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("the list is larger than %d MiB", maxBytes>>20)
	}
	if len(names) == 0 {
		return nil, errors.New("the list holds no names")
	}
	return names, nil
}

func entry(line string) (string, bool) {
	line, _, _ = strings.Cut(line, "#")
	f := strings.Fields(line)
	switch len(f) {
	case 1:
		return routerlog.VetName(f[0])
	case 2:
		if addr, err := netip.ParseAddr(f[0]); err != nil || !slices.Contains(blockAddresses, addr) {
			return "", false
		}
		return routerlog.VetName(f[1])
	}
	return "", false
}

// Set is a list ready for matching.
type Set struct{ names map[string]struct{} }

func NewSet(names []string) *Set {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return &Set{names: m}
}

// Match says whether a name, or a name it is under, is on the list, and which listed name matched. "a.b.example.cc" matches a listed
// "example.cc"; a parent of a listed name, and a name that only looks like one, do not match. A last label alone ("cc") never matches.
func (s *Set) Match(name string) (string, bool) {
	for name != "" {
		if _, ok := s.names[name]; ok {
			return name, true
		}
		_, rest, found := strings.Cut(name, ".")
		if !found || !strings.Contains(rest, ".") {
			return "", false
		}
		name = rest
	}
	return "", false
}

// ValidAddress checks an address for the list: https, no user name or password, on abuse.ch. The address holds the operator's key, so it
// is never put in an error.
func ValidAddress(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return errors.New("the address must be an https address on abuse.ch")
	}
	host := strings.ToLower(u.Hostname())
	if host != allowedDomain && !strings.HasSuffix(host, "."+allowedDomain) {
		return errors.New("the address must be an https address on abuse.ch")
	}
	return nil
}

// Store keeps the downloaded list.
type Store interface {
	Read() (models.KnownBad, error)
	Write(models.KnownBad) error
}

// Opener fetches a URL through the guarded network client: NetworkTools.Open satisfies it.
type Opener func(ctx context.Context, url string) (*http.Response, error)

// Deps are what the manager needs. Address returns the operator's saved address, read on each download.
type Deps struct {
	Store   Store
	Open    Opener
	Address func() string
	Now     func() time.Time
}

// Status says how many names are held and when they were downloaded.
type Status struct {
	Count    int       `json:"count"`
	LoadedAt time.Time `json:"loaded_at,omitzero"`
}

// Manager holds the list and downloads it when the operator asks. Nothing else downloads it.
type Manager struct {
	deps Deps

	mu     sync.RWMutex
	set    *Set
	status Status
}

// NewManager reads the stored list. A list that cannot be read starts the manager empty and returns the error to log: the manager is
// usable either way.
func NewManager(deps Deps) (*Manager, error) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m := &Manager{deps: deps, set: NewSet(nil)}
	stored, err := deps.Store.Read()
	if err != nil {
		return m, fmt.Errorf("failed to read the known-bad names: %w", err)
	}
	m.set, m.status = NewSet(stored.Names), Status{Count: len(stored.Names), LoadedAt: stored.LoadedAt}
	return m, nil
}

// Match says whether a name is on the list.
func (m *Manager) Match(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.set.Match(name)
}

// Status says what is held.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Download fetches the list with the operator's saved address and replaces the held list only if the new one is whole. Whatever goes
// wrong keeps the old list, and no error carries the address.
func (m *Manager) Download(ctx context.Context) error {
	addr := strings.TrimSpace(m.deps.Address())
	if addr == "" {
		return errors.New("no list address is saved yet")
	}
	if err := ValidAddress(addr); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, downloadLimit)
	defer cancel()
	resp, err := m.deps.Open(ctx, addr)
	if err != nil {
		return redact(fmt.Errorf("the list could not be downloaded: %w", err), addr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("abuse.ch answered %d: check that the saved address is the one on your URLhaus page", resp.StatusCode)
	}
	names, err := Parse(resp.Body)
	if err != nil {
		return err
	}
	list := models.KnownBad{LoadedAt: m.deps.Now(), Names: names}
	if err := m.deps.Store.Write(list); err != nil {
		return fmt.Errorf("the list could not be saved: %w", err)
	}
	m.mu.Lock()
	m.set, m.status = NewSet(names), Status{Count: len(names), LoadedAt: list.LoadedAt}
	m.mu.Unlock()
	return nil
}

// redact removes the address from an error's text.
func redact(err error, addr string) error {
	return errors.New(strings.ReplaceAll(err.Error(), addr, "the saved address"))
}
