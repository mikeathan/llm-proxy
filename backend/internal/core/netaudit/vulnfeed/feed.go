// Package vulnfeed downloads the vulnerability data (NVD yearly files and CISA's exploited list) on the operator's
// say-so and keeps the index current. Nothing here runs by itself: a download starts only from Start, which an
// operator's click reaches, and it ends when it finishes, fails, is cancelled or the app shuts down. The fetch goes
// through a guarded opener the caller supplies (the network tools), so the guardrails and the blocked-domain list
// apply, and only the two fixed hosts of DefaultSources are ever asked for.
package vulnfeed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/netaudit/vulnindex"
	"llm-proxy/models"
)

// States a download can be in. Idle means nothing is held; Stopped means some files are held and the rest are not (a
// failure, a cancel or a restart cut the download short); Done means every wanted file is held.
const (
	StateIdle    = "idle"
	StateRunning = "running"
	StateStopped = "stopped"
	StateDone    = "done"
)

// yearsWanted is how many NVD yearly files are fetched: this year and the four before. Older flaws in software still
// running on a home network exist but are rare, and each year costs 20 to 40 MB.
const yearsWanted = 5

// MaxFileBytes caps one downloaded or imported file (as sent, compressed). The largest yearly file seen is about 42 MB.
const MaxFileBytes = 256 << 20

var (
	// ErrBusy means a download or an import is already running.
	ErrBusy = errors.New("a download or an import is already running")
	// ErrNotReady means the application has not finished starting, so no download can begin yet.
	ErrNotReady = errors.New("the application is still starting; try again in a moment")
	// ErrUpToDate means everything is held and recent, so there is nothing to fetch; Start with restart fetches it anyway.
	ErrUpToDate = errors.New("the vulnerability data is up to date")
	// ErrBadSource means an import named a source that is not one of the two the data comes from.
	ErrBadSource = errors.New("the source must be nvd or kev")
	// ErrBadYear means an NVD import named a year the data does not cover.
	ErrBadYear = errors.New("that year is not one of the years the vulnerability data covers")
	// ErrImportTooBig means the supplied file is larger than the cap. NVD's yearly file is much smaller as the .gz it is
	// published as, so that is the one to hand over.
	ErrImportTooBig = errors.New("that file is larger than 256 MB; use the compressed .gz file as NVD publishes it")
	// ErrBadImport means the supplied file could not be used: it is not the expected kind of data, or it is too large.
	// Nothing of the file's own text is in the message.
	ErrBadImport = errors.New("that file could not be read as vulnerability data, so it was not used")
	// errTooBig means a file was larger than the cap.
	errTooBig = errors.New("file larger than the cap")
	// errRedirected means the answer came from a different host or scheme than the one asked.
	errRedirected = errors.New("answered from another address")
)

// Opener makes the guarded GET and returns the body of a 200 answer; the caller closes it.
type Opener func(ctx context.Context, url string) (*http.Response, error)

// Store keeps the index between runs. *persistence.VulnIndexStore satisfies it.
type Store interface {
	Read() (models.VulnIndex, error)
	Write(models.VulnIndex) error
	// Delete removes the stored data; there being none is not an error.
	Delete() error
}

// Sources are where the files come from.
type Sources struct {
	KEV string
	NVD func(year int) string
}

// DefaultSources are the two fixed places the data is downloaded from. Nothing is configurable: a changed address would
// make the download a way to fetch anything.
func DefaultSources() Sources {
	return Sources{
		KEV: "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json",
		NVD: func(year int) string {
			return fmt.Sprintf("https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-%d.json.gz", year)
		},
	}
}

// file is one file the download wants.
type file struct {
	label string
	year  int // 0 for the exploited list
	url   string
}

// Status is what a screen shows. File, the byte counts and the file counts describe the download; NVD and KEV describe
// what the index holds.
type Status struct {
	State      string `json:"state"`
	File       string `json:"file,omitempty"`
	FilesDone  int    `json:"files_done"`
	FilesTotal int    `json:"files_total"`
	BytesDone  int64  `json:"bytes_done,omitempty"`
	BytesTotal int64  `json:"bytes_total,omitempty"`
	// Reason is why a download stopped, or what is wrong with the saved data, in words an operator can act on.
	Reason string            `json:"reason,omitempty"`
	NVD    models.VulnSource `json:"nvd"`
	KEV    models.VulnSource `json:"kev"`
}

// StaleAfter is how old held data may get before an update is worth fetching; until then Start refuses to download it
// again.
const StaleAfter = 60 * 24 * time.Hour

// staleLocked reports whether the held data is older than StaleAfter. The caller holds m.mu.
func (m *Manager) staleLocked() bool {
	s := Status{NVD: m.index.NVD, KEV: m.index.KEV}
	return s.Age(m.now()) > StaleAfter
}

// Age is how long ago the older of the two sources was downloaded; zero when neither has been, so a screen can tell
// "not downloaded" from "old".
func (s Status) Age(now time.Time) time.Duration {
	var oldest time.Time
	for _, at := range []time.Time{s.NVD.ImportedAt, s.KEV.ImportedAt} {
		if !at.IsZero() && (oldest.IsZero() || at.Before(oldest)) {
			oldest = at
		}
	}
	if oldest.IsZero() {
		return 0
	}
	return now.Sub(oldest)
}

// Manager owns the index and the download job.
type Manager struct {
	store Store
	open  Opener
	src   Sources
	now   func() time.Time
	// maxFileBytes is a field so a test can lower it.
	maxFileBytes int64

	ctx  context.Context
	stop context.CancelFunc
	wg   sync.WaitGroup

	mu sync.Mutex
	// jobSize and jobDone count the files this job was asked to fetch and has finished, so a refresh of files that are
	// all held still counts from zero.
	jobSize, jobDone int
	loaded           bool
	index            models.VulnIndex
	loadNote         string
	running          bool
	cancel           context.CancelFunc
	current          string
	reason           string
	bytes            atomic.Int64
	total            atomic.Int64
}

// New returns a manager that cannot start a download until Attach gives it the application's lifecycle.
func New(store Store, open Opener, src Sources) *Manager {
	return &Manager{store: store, open: open, src: src, now: time.Now, maxFileBytes: MaxFileBytes}
}

// Attach ties every download the manager starts to parent: it ends when parent does. It is called once, when the
// application's root context exists.
func (m *Manager) Attach(parent context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx, m.stop = context.WithCancel(parent)
}

// Close stops a running download and waits for it to end.
func (m *Manager) Close() {
	m.mu.Lock()
	stop := m.stop
	m.mu.Unlock()
	if stop != nil {
		stop()
	}
	m.wg.Wait()
}

// Clear deletes all the vulnerability data and returns the manager to its first state, "not downloaded". A running
// download is stopped first and nothing of it is kept. The audits that follow have no data to compare with until it is
// downloaded again.
func (m *Manager) Clear() error {
	m.mu.Lock()
	if m.running {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return ErrBusy
	}
	if err := m.store.Delete(); err != nil {
		return fmt.Errorf("failed to delete the vulnerability data: %w", err)
	}
	m.index, m.loaded, m.reason, m.loadNote = models.VulnIndex{}, true, "", ""
	return nil
}

// Index is the current vulnerability data; empty until a download has finished a file. The value is never changed after
// it is returned, so a caller may keep using it while a download goes on.
func (m *Manager) Index() models.VulnIndex {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	return m.index
}

func (m *Manager) loadLocked() {
	if m.loaded {
		return
	}
	m.loaded = true
	idx, err := m.store.Read()
	if err != nil {
		m.loadNote = "The saved vulnerability data could not be read and was set aside. Download it again."
		return
	}
	m.index = idx
}

func (m *Manager) wanted() []file {
	files := []file{{label: "CISA exploited list", url: m.src.KEV}}
	for i := range yearsWanted {
		y := m.now().Year() - i
		files = append(files, file{label: fmt.Sprintf("NVD %d", y), year: y, url: m.src.NVD(y)})
	}
	return files
}

func held(f file, idx models.VulnIndex) bool {
	if f.year == 0 {
		return !idx.KEV.ImportedAt.IsZero()
	}
	return slices.Contains(idx.NVD.Years, f.year)
}

func countHeld(files []file, idx models.VulnIndex) int {
	n := 0
	for _, f := range files {
		if held(f, idx) {
			n++
		}
	}
	return n
}

// Status reports the download and the data. The state is worked out from what the index holds and what the last job
// said, so it is right after a restart too: a download cut off by a restart reads as stopped.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	files := m.wanted()
	s := Status{FilesTotal: len(files), FilesDone: countHeld(files, m.index), NVD: m.index.NVD, KEV: m.index.KEV}
	switch {
	case m.running:
		s.State, s.File, s.BytesDone, s.BytesTotal = StateRunning, m.current, m.bytes.Load(), m.total.Load()
		s.FilesDone = min(max(s.FilesTotal-m.jobSize+m.jobDone, 0), s.FilesTotal)
	case m.reason != "":
		s.State, s.Reason = StateStopped, m.reason
	case s.FilesDone == s.FilesTotal:
		s.State = StateDone
	case s.FilesDone > 0:
		s.State, s.Reason = StateStopped, "The download did not finish."
	default:
		s.State, s.Reason = StateIdle, m.loadNote
	}
	return s
}

// Start begins a download in the background. It fetches the files the index does not hold yet, which is how a stopped
// download resumes; when it holds everything, it fetches everything again to refresh it. With restart it fetches every
// file whatever is held: what is already held stays in place until its file has been replaced.
func (m *Manager) Start(restart bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return ErrBusy
	}
	if m.ctx == nil {
		return ErrNotReady
	}
	m.loadLocked()
	todo := slices.DeleteFunc(m.wanted(), func(f file) bool { return held(f, m.index) })
	if len(todo) == 0 && !restart && !m.staleLocked() {
		return ErrUpToDate
	}
	if restart || len(todo) == 0 {
		todo = m.wanted()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.running, m.cancel, m.reason = true, cancel, ""
	m.jobSize, m.jobDone = len(todo), 0
	m.bytes.Store(0)
	m.total.Store(0)
	m.wg.Add(1)
	go m.run(ctx, todo)
	return nil
}

// Cancel stops a running download; the files already finished stay. An import in progress is not cancelled (it holds a
// no-op cancel): it is one file the operator already supplied, and ends when the file has arrived and been read.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		m.cancel()
	}
}

// The two sources an import may name.
const (
	SourceNVD = "nvd"
	SourceKEV = "kev"
)

// Import reads one file the operator supplies (NVD's yearly file, gzip or plain, or CISA's exploited list) into the
// index, as a download of the same file would. It exists for a host that may not reach the Internet: the operator fetches
// the file elsewhere and hands it over. It runs in the caller's goroutine, one at a time with downloads and other
// imports. The year of an NVD file is the one the operator says it is, and only the years the data covers are accepted,
// because the file's own content does not tell it. A file that cannot be used changes nothing.
func (m *Manager) Import(source string, year int, r io.Reader) error {
	f, err := m.importTarget(source, year)
	if err != nil {
		return err
	}
	if err := m.beginImport(f.label); err != nil {
		return err
	}
	defer m.endImport()

	m.mu.Lock()
	base := m.index
	m.mu.Unlock()
	m.bytes.Store(0)
	m.total.Store(0)
	body := &countingReader{r: r, n: &m.bytes, limit: m.maxFileBytes}
	next, err := m.importFile(base, f, body)
	if err != nil {
		// Whatever went wrong while reading it, the file was not used and nothing of its own text is repeated. Too large
		// is said as such: it is the one thing the operator can fix by choosing the .gz.
		var tooBig *http.MaxBytesError
		if errors.As(body.err, &tooBig) || errors.Is(body.err, errTooBig) {
			return ErrImportTooBig
		}
		return ErrBadImport
	}
	if err := m.store.Write(next); err != nil {
		return fmt.Errorf("the data could not be saved: %w", err)
	}
	m.mu.Lock()
	m.index, m.reason = next, ""
	m.mu.Unlock()
	return nil
}

func (m *Manager) importTarget(source string, year int) (file, error) {
	switch source {
	case SourceKEV:
		return file{label: "CISA exploited list"}, nil
	case SourceNVD:
		now := m.now().Year()
		if year > now || year <= now-yearsWanted {
			return file{}, ErrBadYear
		}
		return file{label: fmt.Sprintf("NVD %d", year), year: year}, nil
	}
	return file{}, ErrBadSource
}

// beginImport takes the one job slot, or says why it cannot. Cancel and Clear see a job that has no download to stop.
func (m *Manager) beginImport(label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return ErrBusy
	}
	if m.ctx == nil {
		return ErrNotReady
	}
	m.loadLocked()
	m.running, m.cancel, m.current = true, func() {}, label
	m.jobSize, m.jobDone = 1, 0
	return nil
}

func (m *Manager) endImport() {
	m.mu.Lock()
	m.running, m.current = false, ""
	m.mu.Unlock()
}

func (m *Manager) run(ctx context.Context, todo []file) {
	defer m.wg.Done()
	var failure string
	for _, f := range todo {
		if err := m.fetch(ctx, f); err != nil {
			failure = reasonFor(ctx, f, err)
			break
		}
	}
	m.mu.Lock()
	m.running, m.reason, m.current = false, failure, ""
	m.cancel()
	m.mu.Unlock()
}

// fetch downloads one file, imports it and saves the index.
func (m *Manager) fetch(ctx context.Context, f file) error {
	m.mu.Lock()
	m.current = f.label
	base := m.index
	m.mu.Unlock()
	m.bytes.Store(0)
	m.total.Store(0)

	resp, err := m.open(ctx, f.url)
	if err != nil {
		return &openError{err}
	}
	defer resp.Body.Close()
	if !stayedOnTheAddress(f.url, resp) {
		return errRedirected
	}
	if resp.ContentLength > m.maxFileBytes {
		return errTooBig
	}
	m.total.Store(max(resp.ContentLength, 0))
	body := &countingReader{r: resp.Body, n: &m.bytes, limit: m.maxFileBytes}

	next, err := m.importFile(base, f, body)
	switch {
	case body.err != nil && !errors.Is(body.err, io.EOF):
		return body.err
	case err != nil:
		return err
	}
	if err := m.store.Write(next); err != nil {
		return &saveError{err}
	}
	m.mu.Lock()
	m.index = next
	m.jobDone++
	m.mu.Unlock()
	return nil
}

func (m *Manager) importFile(base models.VulnIndex, f file, r io.Reader) (models.VulnIndex, error) {
	if f.year == 0 {
		return vulnindex.ImportKEV(base, r, m.now())
	}
	next, err := vulnindex.ImportNVD(base, r, m.now())
	if err != nil {
		return base, err
	}
	years := append(slices.Clone(base.NVD.Years), f.year)
	slices.Sort(years)
	next.NVD.Years = slices.Compact(years)
	return next, nil
}

// stayedOnTheAddress is true when the response came from the host and scheme that were asked for. The two fixed hosts do
// not redirect today; one that started to would be following a link nobody reviewed.
func stayedOnTheAddress(asked string, resp *http.Response) bool {
	want, err := url.Parse(asked)
	if err != nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	got := resp.Request.URL
	return got.Scheme == want.Scheme && got.Host == want.Host
}

type openError struct{ err error }

func (e *openError) Error() string { return e.err.Error() }
func (e *openError) Unwrap() error { return e.err }

type saveError struct{ err error }

func (e *saveError) Error() string { return e.err.Error() }
func (e *saveError) Unwrap() error { return e.err }

// reasonFor turns a failure into one plain sentence. Nothing the remote server sent is repeated in it.
func reasonFor(ctx context.Context, f file, err error) string {
	var open *openError
	var save *saveError
	switch {
	case ctx.Err() != nil:
		return "Cancelled."
	case errors.Is(err, errRedirected):
		return fmt.Sprintf("The server for %s answered from another address, so it was not used.", f.label)
	case errors.Is(err, errTooBig):
		return fmt.Sprintf("The file for %s is larger than expected, so it was not used.", f.label)
	case errors.As(err, &save):
		return "The data could not be saved."
	case errors.As(err, &open):
		return fmt.Sprintf("Could not download %s: %s.", f.label, open.err)
	case errors.Is(err, vulnindex.ErrBadFile), errors.Is(err, vulnindex.ErrTooLarge):
		return fmt.Sprintf("The file for %s could not be read, so it was not used.", f.label)
	}
	return fmt.Sprintf("The download of %s was interrupted.", f.label)
}

// countingReader counts the bytes read for the progress bar and refuses to read past the cap. It keeps the first error
// the body gave, so a cut connection can be told from a bad file.
type countingReader struct {
	r     io.Reader
	n     *atomic.Int64
	limit int64
	err   error
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n.Load() >= c.limit {
		c.err = errTooBig
		return 0, c.err
	}
	n, err := c.r.Read(p[:min(int64(len(p)), c.limit-c.n.Load()+1)])
	if c.n.Add(int64(n)) > c.limit {
		c.err = errTooBig
		return n, c.err
	}
	if err != nil && c.err == nil {
		c.err = err
	}
	return n, err
}
