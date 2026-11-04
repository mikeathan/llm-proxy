package ubuntuoval

import (
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"llm-proxy/models"
)

// Host is the one place the notices come from. Nothing is configurable: a changed address would make the download a way to
// fetch anything.
const Host = "security-metadata.canonical.com"

// Limits of one download: the file as sent (the noble file is 1.3 MB) and the notices once decompressed (37.7 MB).
const (
	MaxCompressedBytes = 32 << 20
	MaxXMLBytes        = 192 << 20
)

var (
	// ErrBusy means an update or a delete is already running.
	ErrBusy = errors.New("an update is already running")
	// ErrNotUbuntu means this computer is not a release the notices exist for, or its release name is not a plain word.
	ErrNotUbuntu = errors.New("this computer is not running an Ubuntu release that has security notices")
	// ErrTooBig means the file is larger than the cap.
	ErrTooBig = errors.New("the file is larger than the cap")
	// ErrRedirected means the answer came from another host or scheme than the one asked.
	ErrRedirected = errors.New("answered from another address")
	// ErrBadAnswer means the server did not answer with the file.
	ErrBadAnswer = errors.New("the server did not send the file")
)

var releaseName = regexp.MustCompile(`^[a-z]{3,20}$`)

// Opener makes the guarded GET (the network tools); the caller closes the body.
type Opener func(ctx context.Context, url string) (*http.Response, error)

// Store keeps the index between runs: *persistence.UbuntuIndexStore.
type Store interface {
	Read() (models.UbuntuIndex, error)
	Write(models.UbuntuIndex) error
	Delete() error
}

// Status is what a screen shows.
type Status struct {
	// Supported is true when this computer's release has notices to fetch, and HostRelease is its codename.
	Supported   bool   `json:"supported"`
	HostRelease string `json:"host_release,omitempty"`
	Running     bool   `json:"running"`
	// Release, Notices and Packages describe the data held; GeneratedAt is when Canonical built it and ImportedAt when this
	// program read it.
	Release     string    `json:"release,omitempty"`
	Notices     int       `json:"notices"`
	Packages    int       `json:"packages"`
	GeneratedAt time.Time `json:"generated_at,omitzero"`
	ImportedAt  time.Time `json:"imported_at,omitzero"`
	// Reason is why the last update failed, or what is wrong with the saved data, in words an operator can act on.
	Reason string `json:"reason,omitempty"`
}

// Manager owns the data and the one-click update. Nothing here runs by itself: an update starts only from Update, which an
// operator's click reaches. It is synchronous, because the file is small.
type Manager struct {
	store   Store
	open    Opener
	release func() string
	now     func() time.Time
	// decode wraps the downloaded bytes in the decompressor; bzip2, limited to MaxXMLBytes, unless a test replaces it.
	decode func(io.Reader) io.Reader

	mu     sync.Mutex
	index  models.UbuntuIndex
	loaded bool
	busy   bool
	reason string
}

// New builds the manager. release names this computer's Ubuntu release ("noble"), or "" when it is not Ubuntu.
func New(store Store, open Opener, release func() string) *Manager {
	return &Manager{
		store: store, open: open, release: release, now: time.Now,
		decode: func(r io.Reader) io.Reader { return io.LimitReader(bzip2.NewReader(r), MaxXMLBytes+1) },
	}
}

// Index is the notices held, empty until an update has finished. The value is a copy: a caller cannot change what is held.
func (m *Manager) Index() models.UbuntuIndex {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	return cloneIndex(m.index)
}

func (m *Manager) loadLocked() {
	if m.loaded {
		return
	}
	m.loaded = true
	idx, err := m.store.Read()
	if err != nil {
		m.reason = "The saved Ubuntu security data could not be read, so it was set aside. Update it again."
		return
	}
	m.index = idx
}

// Status reports whether this computer can use the data, what is held and the last trouble.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	host := m.release()
	return Status{
		Supported: releaseName.MatchString(host), HostRelease: host, Running: m.busy,
		Release: m.index.Release, Notices: len(m.index.Notices), Packages: len(m.index.Packages),
		GeneratedAt: m.index.GeneratedAt, ImportedAt: m.index.ImportedAt, Reason: m.reason,
	}
}

// Update fetches this release's notices and replaces the data held. A failure of any kind leaves what was held as it was.
func (m *Manager) Update(ctx context.Context) error {
	release := m.release()
	if !releaseName.MatchString(release) {
		return ErrNotUbuntu
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return ErrBusy
	}
	m.busy = true
	m.mu.Unlock()
	next, err := m.fetch(ctx, release)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.busy = false
	if err != nil {
		m.reason = reasonOf(err)
		return err
	}
	m.index, m.loaded, m.reason = next, true, ""
	return nil
}

func (m *Manager) fetch(ctx context.Context, release string) (models.UbuntuIndex, error) {
	address := fmt.Sprintf("https://%s/oval/com.ubuntu.%s.usn.oval.xml.bz2", Host, release)
	resp, err := m.open(ctx, address)
	if err != nil {
		return models.UbuntuIndex{}, fmt.Errorf("%w: %w", ErrBadAnswer, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.Request == nil || resp.Request.URL == nil || resp.Request.URL.Scheme != "https" || resp.Request.URL.Host != Host:
		return models.UbuntuIndex{}, ErrRedirected
	case resp.StatusCode != http.StatusOK:
		return models.UbuntuIndex{}, ErrBadAnswer
	case resp.ContentLength > MaxCompressedBytes:
		return models.UbuntuIndex{}, ErrTooBig
	}
	idx, err := Parse(m.decode(io.LimitReader(resp.Body, MaxCompressedBytes+1)), release)
	if err != nil {
		return models.UbuntuIndex{}, err
	}
	idx.ImportedAt = m.now().UTC()
	if err := m.store.Write(idx); err != nil {
		return models.UbuntuIndex{}, fmt.Errorf("failed to save the Ubuntu security data: %w", err)
	}
	return idx, nil
}

// reasonOf says what went wrong in words a screen can show; the text of the data and of the server is never in it.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrNotUsable):
		return "The file was not Ubuntu security notices, so nothing was changed."
	case errors.Is(err, ErrRedirected):
		return "The answer came from another address, so it was not used."
	case errors.Is(err, ErrTooBig):
		return "The file was larger than expected, so it was not used."
	case errors.Is(err, ErrBadAnswer):
		return "The notices could not be downloaded. Check the connection and the Guardrails internet switch, then try again."
	}
	return "The Ubuntu security data could not be updated, and what was held is unchanged."
}

// Clear deletes the data held. Audits have nothing to compare packages with until it is updated again.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return ErrBusy
	}
	if err := m.store.Delete(); err != nil {
		return fmt.Errorf("failed to delete the Ubuntu security data: %w", err)
	}
	m.index, m.loaded, m.reason = models.UbuntuIndex{}, true, ""
	return nil
}

func cloneIndex(idx models.UbuntuIndex) models.UbuntuIndex {
	out := idx
	out.Notices = make(map[string]models.UbuntuNotice, len(idx.Notices))
	for k, v := range idx.Notices {
		v.CVEs = slices.Clone(v.CVEs)
		out.Notices[k] = v
	}
	out.Packages = make(map[string][]models.UbuntuFix, len(idx.Packages))
	for k, v := range idx.Packages {
		out.Packages[k] = slices.Clone(v)
	}
	return out
}
