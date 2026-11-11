package ipplaces

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/ipowners"
)

const (
	// urlFormat is the only address the table is fetched from: the free City Lite table of db-ip.com (CC BY 4.0), named for its month.
	urlFormat = "https://download.db-ip.com/free/dbip-city-lite-%s.csv.gz"
	monthForm = "2006-01"
	service   = "db-ip.com"
	// Attribution is the credit the licence (CC BY 4.0) asks for, shown wherever the places are.
	Attribution = "IP geolocation by DB-IP (db-ip.com), CC BY 4.0"
	// maxDownload bounds what is read of the download, compressed (the real file is about 86 MB). maxExpanded bounds it once expanded (about 700 MB).
	maxDownload int64 = 160 << 20
	maxExpanded int64 = 2 << 30
	// downloadLimit bounds the whole download and the pass that builds the index.
	downloadLimit = 10 * time.Minute
	// monthsTried is how many months back a download looks when this month's file is not up yet.
	monthsTried = 2
)

// Opener fetches a URL through the guarded network client: NetworkTools.Open satisfies it.
type Opener = ipowners.Opener

// Deps are what the manager needs. Path is the one file the index is kept in.
type Deps struct {
	Path string
	Open Opener
	Now  func() time.Time
}

// Status says how many ranges the index tells apart and when it was downloaded; it is the owner table's status, so one handler serves both.
type Status = ipowners.Status

// Manager keeps the index file open and downloads a new one when the operator asks. Nothing else downloads it.
type Manager struct {
	deps Deps

	mu     sync.RWMutex
	file   *os.File
	index  *Index
	status Status
}

// NewManager opens the stored index. A file that cannot be read starts the manager empty and returns the error to log: the manager is usable
// either way.
func NewManager(deps Deps) (*Manager, error) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m := &Manager{deps: deps}
	if err := m.openStored(); err != nil {
		return m, fmt.Errorf("failed to read the places table: %w", err)
	}
	return m, nil
}

// openStored holds the index file on disk, if there is one.
func (m *Manager) openStored() error {
	f, err := os.Open(m.deps.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil {
		var x *Index
		if x, err = Open(f, info.Size()); err == nil {
			m.hold(f, x, info.ModTime())
			return nil
		}
	}
	_ = f.Close()
	return err
}

func (m *Manager) hold(f *os.File, x *Index, at time.Time) {
	m.mu.Lock()
	old := m.file
	m.file, m.index, m.status = f, x, Status{Ranges: x.Counts().Ranges(), LoadedAt: at}
	m.mu.Unlock()
	if old != nil {
		_ = old.Close() // a lookup still reading it gets an error, which reads as "no place"
	}
}

// Place says where an address is used: nowhere, until the table is downloaded.
func (m *Manager) Place(a netip.Addr) (Place, bool) {
	m.mu.RLock()
	x := m.index
	m.mu.RUnlock()
	if x == nil {
		return Place{}, false
	}
	p, ok, err := x.Lookup(a)
	return p, ok && err == nil
}

// Status says what is held.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Remove forgets the index and deletes the file.
func (m *Manager) Remove() error {
	m.mu.Lock()
	f := m.file
	m.file, m.index, m.status = nil, nil, Status{}
	m.mu.Unlock()
	if f != nil {
		_ = f.Close()
	}
	if err := os.Remove(m.deps.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove the places table: %w", err)
	}
	return nil
}

// Delete is Remove under the name a factory reset calls.
func (m *Manager) Delete() error { return m.Remove() }

// Download fetches this month's table (or last month's, when this month's is not up yet), turns it into the index while it streams and replaces the
// held index only if the new one is whole. Whatever goes wrong keeps the old index and leaves no partial file.
func (m *Manager) Download(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, downloadLimit)
	defer cancel()
	body, err := m.open(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	zr, err := gzip.NewReader(io.LimitReader(body, maxDownload+1))
	if err != nil {
		return fmt.Errorf("the places table is not a compressed file: %w", err)
	}
	defer zr.Close()
	tmp, err := os.CreateTemp(filepath.Dir(m.deps.Path), "destination-places-*.tmp")
	if err != nil {
		return fmt.Errorf("the places table could not be saved: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	expanded := &io.LimitedReader{R: zr, N: maxExpanded + 1}
	if _, err := Build(expanded, tmp); err != nil {
		if expanded.N <= 0 {
			return fmt.Errorf("the places table is larger than %d MiB once expanded", maxExpanded>>20)
		}
		return err
	}
	if expanded.N <= 0 {
		return fmt.Errorf("the places table is larger than %d MiB once expanded", maxExpanded>>20)
	}
	return m.install(tmp)
}

// install makes a finished temporary file the index: it is checked, renamed over the old one and opened under its new name.
func (m *Manager) install(tmp *os.File) error {
	info, err := tmp.Stat()
	if err != nil {
		return fmt.Errorf("the places table could not be saved: %w", err)
	}
	if _, err := Open(tmp, info.Size()); err != nil {
		return err
	}
	at := m.deps.Now()
	if err := os.Chtimes(tmp.Name(), at, at); err != nil {
		return fmt.Errorf("the places table could not be saved: %w", err)
	}
	if err := os.Rename(tmp.Name(), m.deps.Path); err != nil {
		return fmt.Errorf("the places table could not be saved: %w", err)
	}
	f, err := os.Open(m.deps.Path)
	if err != nil {
		return fmt.Errorf("the places table could not be saved: %w", err)
	}
	x, err := Open(f, info.Size())
	if err != nil {
		_ = f.Close()
		return err
	}
	m.hold(f, x, at)
	return nil
}

// open asks for this month's file, then the month before, and returns the first body that answers.
func (m *Manager) open(ctx context.Context) (io.ReadCloser, error) {
	var last error
	now := m.deps.Now()
	for back := range monthsTried {
		resp, err := m.deps.Open(ctx, fmt.Sprintf(urlFormat, time.Date(now.Year(), now.Month()-time.Month(back), 1, 0, 0, 0, 0, time.UTC).Format(monthForm)))
		if err == nil {
			return resp.Body, nil
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("the places table could not be downloaded from %s: %w", service, ctx.Err())
		}
		last = err
	}
	return nil, fmt.Errorf("the places table could not be downloaded from %s: %w", service, last)
}
