package ipowners

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"llm-proxy/models"
)

const (
	// UrlV4 and UrlV6 are the only addresses the table is fetched from: the public-domain tables of iptoasn.com.
	UrlV4 = "https://iptoasn.com/data/ip2asn-v4.tsv.gz"
	UrlV6 = "https://iptoasn.com/data/ip2asn-v6.tsv.gz"
	// maxDownload bounds what is read of one download, compressed (the real files are 9 and 2 MB).
	maxDownload = 32 << 20
	// downloadLimit bounds the whole download.
	downloadLimit = 2 * time.Minute
	service       = "iptoasn.com"
	lengthPrefix  = 4
)

// maxExpanded bounds one file once decompressed (the real ones are about 50 and 17 MB). It is a variable only so a test can lower it.
var maxExpanded int64 = 160 << 20

// Store keeps the downloaded table as the two compressed files, one after the other, with the time they were downloaded.
type Store interface {
	Read() (data []byte, at time.Time, err error)
	Write(data []byte, at time.Time) error
	Delete() error
}

// Opener fetches a URL through the guarded network client: NetworkTools.Open satisfies it.
type Opener func(ctx context.Context, url string) (*http.Response, error)

// Deps are what the manager needs.
type Deps struct {
	Store Store
	Open  Opener
	Now   func() time.Time
}

// Status says how many ranges are held and when they were downloaded.
type Status struct {
	Ranges   int       `json:"ranges"`
	LoadedAt time.Time `json:"loaded_at,omitzero"`
}

// Manager holds the table and downloads it when the operator asks. Nothing else downloads it.
type Manager struct {
	deps Deps

	mu     sync.RWMutex
	table  *Table
	status Status
}

// NewManager reads the stored table. A file that cannot be read starts the manager empty and returns the error to log: the manager is
// usable either way.
func NewManager(deps Deps) (*Manager, error) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m := &Manager{deps: deps}
	data, at, err := deps.Store.Read()
	if err != nil {
		return m, fmt.Errorf("failed to read the destination owners: %w", err)
	}
	if len(data) == 0 {
		return m, nil
	}
	bodies, err := unpack(data)
	if err == nil {
		var table *Table
		if table, err = parseGzip(bodies...); err == nil {
			m.hold(table, at)
			return m, nil
		}
	}
	return m, fmt.Errorf("failed to read the destination owners: %w", err)
}

func (m *Manager) hold(t *Table, at time.Time) {
	v4, v6 := t.Counts()
	m.mu.Lock()
	m.table, m.status = t, Status{Ranges: v4 + v6, LoadedAt: at}
	m.mu.Unlock()
}

// Owner says who owns an address: no one, until the table is downloaded.
func (m *Manager) Owner(a netip.Addr) (models.TrafficOwner, bool) {
	m.mu.RLock()
	t := m.table
	m.mu.RUnlock()
	return t.Lookup(a)
}

// Status says what is held.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Remove forgets the table and deletes the stored file.
func (m *Manager) Remove() error {
	if err := m.deps.Store.Delete(); err != nil {
		return fmt.Errorf("failed to remove the destination owners: %w", err)
	}
	m.mu.Lock()
	m.table, m.status = nil, Status{}
	m.mu.Unlock()
	return nil
}

// Download fetches both files and replaces the held table only if the new one is whole. Whatever goes wrong keeps the old table.
func (m *Manager) Download(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, downloadLimit)
	defer cancel()
	var bodies [][]byte
	for _, url := range []string{UrlV4, UrlV6} {
		body, err := m.fetch(ctx, url)
		if err != nil {
			return err
		}
		bodies = append(bodies, body)
	}
	table, err := parseGzip(bodies...)
	if err != nil {
		return err
	}
	at := m.deps.Now()
	if err := m.deps.Store.Write(pack(bodies), at); err != nil {
		return fmt.Errorf("the table could not be saved: %w", err)
	}
	m.hold(table, at)
	return nil
}

// fetch reads one file, still compressed.
func (m *Manager) fetch(ctx context.Context, url string) ([]byte, error) {
	resp, err := m.deps.Open(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("the table could not be downloaded from %s: %w", service, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", service, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("the table could not be downloaded from %s: %w", service, err)
	}
	if len(body) > maxDownload {
		return nil, fmt.Errorf("the table from %s is larger than %d MiB", service, maxDownload>>20)
	}
	return body, nil
}

// pack joins the compressed files into the one stored file: each is preceded by its length in four bytes.
func pack(bodies [][]byte) []byte {
	var out []byte
	for _, b := range bodies {
		out = binary.BigEndian.AppendUint32(out, uint32(len(b)))
		out = append(out, b...)
	}
	return out
}

// unpack splits a stored file back into the compressed files it holds.
func unpack(data []byte) ([][]byte, error) {
	var bodies [][]byte
	for len(data) > 0 {
		if len(data) < lengthPrefix {
			return nil, errors.New("the stored file is cut short")
		}
		n := int(binary.BigEndian.Uint32(data))
		if len(data) < lengthPrefix+n {
			return nil, errors.New("the stored file is cut short")
		}
		bodies = append(bodies, data[lengthPrefix:lengthPrefix+n])
		data = data[lengthPrefix+n:]
	}
	return bodies, nil
}

// parseGzip reads compressed files as one table. A file that expands past maxExpanded is refused.
func parseGzip(bodies ...[]byte) (*Table, error) {
	var readers []io.Reader
	var limits []*io.LimitedReader
	for _, body := range bodies {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("the table is not a compressed file: %w", err)
		}
		defer zr.Close()
		lr := &io.LimitedReader{R: zr, N: maxExpanded + 1}
		limits = append(limits, lr)
		readers = append(readers, lr)
	}
	table, perr := ParseAll(readers...)
	for _, lr := range limits {
		if lr.N <= 0 {
			return nil, fmt.Errorf("the table is larger than %d MiB once expanded", maxExpanded>>20)
		}
	}
	if perr != nil {
		return nil, perr
	}
	return table, nil
}
