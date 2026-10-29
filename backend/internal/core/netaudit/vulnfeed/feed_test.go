package vulnfeed

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

var clock = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

type memStore struct {
	mu      sync.Mutex
	idx     models.VulnIndex
	writes  int
	deletes int
	readErr error
	wErr    error
	delErr  error
}

func (s *memStore) Read() (models.VulnIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx, s.readErr
}

func (s *memStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idx, s.deletes = models.VulnIndex{}, s.deletes+1
	return s.delErr
}

func (s *memStore) Write(i models.VulnIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wErr != nil {
		return s.wErr
	}
	s.idx, s.writes = i, s.writes+1
	return nil
}

func nvdBody(year int) []byte {
	cve := fmt.Sprintf("CVE-%d-0001", year)
	doc := `{"vulnerabilities":[{"cve":{"id":"` + cve + `","vulnStatus":"Analyzed","metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":9.8}}]},` +
		`"configurations":[{"nodes":[{"operator":"OR","cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:vendor:app:*:*:*:*:*:*:*:*","versionEndExcluding":"` + fmt.Sprint(year-2000) + `.0"}]}]}]}}]}`
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(doc))
	zw.Close()
	return buf.Bytes()
}

const kevBody = `{"vulnerabilities":[{"cveID":"CVE-2026-0001","dateAdded":"2026-01-02"}]}`

// feedServer serves the KEV file and one NVD file per year; hook can delay or fail a request by its path.
type feedServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits []string
	hook func(path string, w http.ResponseWriter) bool
}

func newFeedServer(t *testing.T) *feedServer {
	fs := &feedServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.hits = append(fs.hits, r.URL.Path)
		hook := fs.hook
		fs.mu.Unlock()
		if hook != nil && hook(r.URL.Path, w) {
			return
		}
		if r.URL.Path == "/kev.json" {
			w.Write([]byte(kevBody))
			return
		}
		var year int
		fmt.Sscanf(r.URL.Path, "/nvd/%d.json.gz", &year)
		w.Header().Set("Content-Length", fmt.Sprint(len(nvdBody(year))))
		w.Write(nvdBody(year))
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *feedServer) requests() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]string(nil), fs.hits...)
}

func (fs *feedServer) sources() Sources {
	return Sources{KEV: fs.URL + "/kev.json", NVD: func(y int) string { return fmt.Sprintf("%s/nvd/%d.json.gz", fs.URL, y) }}
}

func opener(c *http.Client) Opener {
	return func(ctx context.Context, u string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("server returned unexpected status: %s", resp.Status)
		}
		return resp, nil
	}
}

func newManager(t *testing.T, fs *feedServer, store *memStore) *Manager {
	m := New(store, opener(fs.Client()), fs.sources())
	m.Attach(context.Background())
	m.now = func() time.Time { return clock }
	t.Cleanup(m.Close)
	return m
}

func waitFor(t *testing.T, m *Manager, want string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Status(); s.State == want {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state never became %q; last %+v", want, m.Status())
	return Status{}
}

func TestDownload_FetchesTheExploitedListAndFiveYearsAndSavesAfterEachFile(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, m, StateDone)
	if s.FilesDone != 6 || s.FilesTotal != 6 || s.Reason != "" {
		t.Errorf("status = %+v", s)
	}
	if got := s.NVD.Years; len(got) != 5 || got[0] != 2022 || got[4] != 2026 {
		t.Errorf("years = %v", got)
	}
	if store.writes != 6 {
		t.Errorf("writes = %d, want one per file", store.writes)
	}
	idx := m.Index()
	if len(idx.Entries["a:vendor:app"]) != 5 || idx.Exploited["CVE-2026-0001"] == "" {
		t.Errorf("index = %+v", idx)
	}
}

func TestDownload_ProgressShowsTheFileAndItsSizeWhileRunning(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	release := make(chan struct{})
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/nvd/2025.json.gz" {
			w.Header().Set("Content-Length", "1000")
			w.(http.Flusher).Flush()
			<-release
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	defer close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := m.Status()
		// The file's name is set before the request is made and its size only once the answer's headers arrive, so wait
		// for both.
		if s.State == StateRunning && s.File == "NVD 2025" && s.BytesTotal == 1000 {
			if s.FilesDone != 2 || s.FilesTotal != 6 {
				t.Errorf("status = %+v", s)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never reached the 2025 file: %+v", s)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDownload_AFailureKeepsFinishedFilesAndResumeFetchesOnlyTheRest(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	fail := true
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if fail && path == "/nvd/2024.json.gz" {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, m, StateStopped)
	if s.FilesDone != 3 || s.FilesTotal != 6 || !strings.Contains(s.Reason, "NVD 2024") || strings.Contains(s.Reason, "boom") {
		t.Errorf("status = %+v", s)
	}
	if len(store.idx.NVD.Years) != 2 || store.idx.KEV.CVEs != 1 {
		t.Errorf("finished files must be kept: %+v", store.idx)
	}

	fail = false
	before := len(fs.requests())
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateDone)
	if got := len(fs.requests()) - before; got != 3 {
		t.Errorf("resume fetched %d files, want only the 3 missing", got)
	}
}

// later moves the manager's clock past the age at which held data counts as old.
func later(m *Manager) { m.now = func() time.Time { return clock.Add(StaleAfter + time.Hour) } }

func TestDownload_WhenEverythingIsHeldAndOldStartRefreshesAll(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	m.Start(false)
	waitFor(t, m, StateDone)
	later(m)
	before := len(fs.requests())
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateDone)
	if got := len(fs.requests()) - before; got != 6 {
		t.Errorf("refresh fetched %d files, want 6", got)
	}
}

func TestDownload_WhenEverythingIsHeldAndFreshStartDoesNothing(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	m.Start(false)
	waitFor(t, m, StateDone)
	before := len(fs.requests())
	if err := m.Start(false); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("err = %v, want ErrUpToDate", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(fs.requests()) - before; got != 0 {
		t.Errorf("fresh data must not be fetched again, %d requests were made", got)
	}
	if s := m.Status(); s.State != StateDone {
		t.Errorf("status = %+v", s)
	}
	if err := m.Start(true); err != nil {
		t.Fatalf("start again must still work on fresh data: %v", err)
	}
	waitFor(t, m, StateDone)
	if got := len(fs.requests()) - before; got != 6 {
		t.Errorf("start again fetched %d files, want 6", got)
	}
}

func TestDownload_ARefreshCountsFromZeroNotFromWhatIsHeld(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	m.Start(false)
	waitFor(t, m, StateDone)
	later(m)
	release := make(chan struct{})
	fs.mu.Lock()
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/nvd/2025.json.gz" {
			w.Header().Set("Content-Length", "1000")
			w.(http.Flusher).Flush()
			<-release
			return true
		}
		return false
	}
	fs.mu.Unlock()
	defer close(release)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := m.Status()
		if s.State == StateRunning && s.File == "NVD 2025" {
			if s.FilesDone != 2 || s.FilesTotal != 6 {
				t.Errorf("status = %+v, want 2 of 6 done while the third is fetched", s)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never reached the 2025 file: %+v", s)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDownload_StartAgainFetchesEverythingEvenWhenFilesAreMissing(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	fail := true
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if fail && path == "/nvd/2024.json.gz" {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateStopped)
	fail = false
	before := len(fs.requests())
	if err := m.Start(true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateDone)
	if got := len(fs.requests()) - before; got != 6 {
		t.Errorf("start again fetched %d files, want all 6", got)
	}
}

func TestDownload_CancelStopsAtOnceAndSaysSo(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	started := make(chan struct{})
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/nvd/2026.json.gz" {
			close(started)
			<-w.(http.CloseNotifier).CloseNotify()
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	m.Start(false)
	<-started
	m.Cancel()
	s := waitFor(t, m, StateStopped)
	if s.Reason != "Cancelled." || s.FilesDone != 1 {
		t.Errorf("status = %+v", s)
	}
}

func TestDownload_StartWhileRunningIsRefused(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	release := make(chan struct{})
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/kev.json" {
			<-release
		}
		return false
	}
	m := newManager(t, fs, store)
	m.Start(false)
	defer close(release)
	if err := m.Start(false); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
}

func TestDownload_AFileThatIsNotDataLeavesTheIndexAndSaysSo(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/kev.json" {
			w.Write([]byte("<html>captive portal</html>"))
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	m.Start(false)
	s := waitFor(t, m, StateStopped)
	if !strings.Contains(s.Reason, "could not be read") || strings.Contains(s.Reason, "captive") || store.writes != 0 {
		t.Errorf("status %+v writes %d", s, store.writes)
	}
}

func TestDownload_AFileOverTheSizeCapIsRefused(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	m.maxFileBytes = 10
	m.Start(false)
	s := waitFor(t, m, StateStopped)
	if !strings.Contains(s.Reason, "larger than expected") || store.writes != 0 {
		t.Errorf("status %+v writes %d", s, store.writes)
	}
}

func TestDownload_ASaveFailureStopsAndSaysSo(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{wErr: errors.New("disk full")}
	m := newManager(t, fs, store)
	m.Start(false)
	if s := waitFor(t, m, StateStopped); !strings.Contains(s.Reason, "could not be saved") {
		t.Errorf("status = %+v", s)
	}
}

func TestDownload_TheGuardedClientsRefusalIsShownInPlainWords(t *testing.T) {
	store := &memStore{}
	m := New(store, func(context.Context, string) (*http.Response, error) {
		return nil, errors.New("network access is fully blocked by guardrails")
	}, DefaultSources())
	m.Attach(context.Background())
	defer m.Close()
	m.Start(false)
	s := waitFor(t, m, StateStopped)
	if !strings.Contains(s.Reason, "blocked by guardrails") {
		t.Errorf("status = %+v", s)
	}
}

func TestStatus_IsDerivedFromWhatTheIndexHolds(t *testing.T) {
	fs := newFeedServer(t)
	empty := newManager(t, fs, &memStore{})
	if s := empty.Status(); s.State != StateIdle || s.FilesDone != 0 || s.FilesTotal != 6 {
		t.Errorf("empty = %+v", s)
	}
	partial := models.VulnIndex{
		KEV: models.VulnSource{ImportedAt: clock, CVEs: 1},
		NVD: models.VulnSource{ImportedAt: clock, Years: []int{2026, 2025}},
	}
	if s := newManager(t, fs, &memStore{idx: partial}).Status(); s.State != StateStopped || s.FilesDone != 3 || s.Reason == "" {
		t.Errorf("partial (a restart mid-download) = %+v", s)
	}
	full := partial
	full.NVD.Years = []int{2022, 2023, 2024, 2025, 2026}
	if s := newManager(t, fs, &memStore{idx: full}).Status(); s.State != StateDone || s.FilesDone != 6 {
		t.Errorf("full = %+v", s)
	}
}

func TestStatus_AnUnreadableSavedIndexIsReportedNotHidden(t *testing.T) {
	fs := newFeedServer(t)
	m := newManager(t, fs, &memStore{readErr: errors.New("corrupt")})
	s := m.Status()
	if s.State != StateIdle || !strings.Contains(s.Reason, "set aside") {
		t.Errorf("status = %+v", s)
	}
}

func TestDefaultSources_AreFixedHTTPSURLsOnTwoHosts(t *testing.T) {
	src := DefaultSources()
	for _, raw := range []string{src.KEV, src.NVD(2026)} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || (u.Host != "www.cisa.gov" && u.Host != "nvd.nist.gov") || u.RawQuery != "" {
			t.Errorf("%q is not one of the two fixed https hosts", raw)
		}
	}
	if got := src.NVD(2026); got != "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2026.json.gz" {
		t.Errorf("NVD url = %s", got)
	}
}

func TestClose_StopsARunningJobAndWaitsForIt(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	started := make(chan struct{})
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/kev.json" {
			close(started)
			<-w.(http.CloseNotifier).CloseNotify()
			return true
		}
		return false
	}
	m := New(store, opener(fs.Client()), fs.sources())
	m.Attach(context.Background())
	m.Start(false)
	<-started
	m.Close()
	if s := m.Status(); s.State == StateRunning {
		t.Errorf("still running after Close: %+v", s)
	}
}

func TestDownload_ACutConnectionIsReportedAsInterruptedNotAsABadFile(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/nvd/2026.json.gz" {
			whole := nvdBody(2026)
			w.Header().Set("Content-Length", fmt.Sprint(len(whole)))
			w.Write(whole[:len(whole)-8])
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	m.Start(false)
	s := waitFor(t, m, StateStopped)
	if !strings.Contains(s.Reason, "interrupted") || s.FilesDone != 1 {
		t.Errorf("status = %+v", s)
	}
}

func TestDownload_ARedirectToAnotherHostIsRefused(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(kevBody)) }))
	defer elsewhere.Close()
	fs, store := newFeedServer(t), &memStore{}
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/kev.json" {
			w.Header().Set("Location", elsewhere.URL+"/kev.json")
			w.WriteHeader(http.StatusFound)
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	m.Start(false)
	s := waitFor(t, m, StateStopped)
	if !strings.Contains(s.Reason, "another address") || store.writes != 0 {
		t.Errorf("status %+v writes %d", s, store.writes)
	}
}

func TestStatus_AgeIsThatOfTheOlderSourceAndZeroWhenNothingIsHeld(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	var s Status
	if s.Age(now) != 0 {
		t.Errorf("empty status has age %v", s.Age(now))
	}
	s.KEV.ImportedAt = now.Add(-2 * time.Hour)
	if s.Age(now) != 2*time.Hour {
		t.Errorf("one source: %v", s.Age(now))
	}
	s.NVD.ImportedAt = now.Add(-72 * time.Hour)
	if s.Age(now) != 72*time.Hour {
		t.Errorf("two sources: %v, want the older", s.Age(now))
	}
}

func TestClear_DeletesWhatIsHeldAndTheStatusGoesBackToNotDownloaded(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	m := newManager(t, fs, store)
	m.Start(false)
	waitFor(t, m, StateDone)

	if err := m.Clear(); err != nil {
		t.Fatal(err)
	}
	s := m.Status()
	if s.State != StateIdle || s.FilesDone != 0 || s.NVD.CVEs != 0 || s.KEV.CVEs != 0 || store.deletes != 1 {
		t.Errorf("status = %+v, deletes %d", s, store.deletes)
	}
	if len(m.Index().Entries) != 0 || len(m.Index().Exploited) != 0 {
		t.Errorf("the index in memory must be empty too, or audits would keep using deleted data: %+v", m.Index())
	}
	before := len(fs.requests())
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateDone)
	if got := len(fs.requests()) - before; got != 6 {
		t.Errorf("after a clear everything is fetched again, got %d files", got)
	}
}

func TestClear_StopsARunningDownloadFirstAndKeepsNothingOfIt(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	release := make(chan struct{})
	fs.hook = func(path string, w http.ResponseWriter) bool {
		if path == "/nvd/2025.json.gz" {
			w.Header().Set("Content-Length", "1000")
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-time.After(5 * time.Second):
			}
			return true
		}
		return false
	}
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, StateRunning)
	defer close(release)
	if err := m.Clear(); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.State != StateIdle || s.FilesDone != 0 || s.Reason != "" {
		t.Errorf("status after clearing a running download = %+v", s)
	}
	if len(m.Index().Entries) != 0 {
		t.Errorf("nothing of the stopped download may remain: %+v", m.Index())
	}
}

func TestClear_WithNothingHeldIsFineAndAFailedDeleteIsSaid(t *testing.T) {
	store := &memStore{}
	m := newManager(t, newFeedServer(t), store)
	if err := m.Clear(); err != nil {
		t.Fatalf("clearing nothing: %v", err)
	}
	store.delErr = errors.New("disk is read-only")
	if err := m.Clear(); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("a failed delete must be reported: %v", err)
	}
}

func TestStart_BeforeTheApplicationIsAttachedSaysItIsNotReady(t *testing.T) {
	m := New(&memStore{}, func(context.Context, string) (*http.Response, error) { return nil, errors.New("must not be called") }, DefaultSources())
	if err := m.Start(false); !errors.Is(err, ErrNotReady) {
		t.Errorf("err = %v, want ErrNotReady", err)
	}
	m.Close() // closing a manager that never started is fine
}

func TestImport_AFileTheOperatorSuppliesIsReadAndSavedLikeADownloadedOne(t *testing.T) {
	store := &memStore{}
	m := newManager(t, newFeedServer(t), store)
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); err != nil {
		t.Fatalf("kev: %v", err)
	}
	if err := m.Import(SourceNVD, 2026, bytes.NewReader(nvdBody(2026))); err != nil {
		t.Fatalf("nvd: %v", err)
	}
	s := m.Status()
	if store.writes != 2 || s.KEV.ImportedAt.IsZero() || s.NVD.ImportedAt.IsZero() || !slices.Equal(s.NVD.Years, []int{2026}) || s.FilesDone != 2 {
		t.Errorf("writes %d status %+v", store.writes, s)
	}
	if s.State == StateRunning {
		t.Errorf("an import that has ended still reads as running: %+v", s)
	}
}

func TestImport_AFileThatIsNotDataChangesNothingAndNamesNoneOfItsText(t *testing.T) {
	store := &memStore{}
	m := newManager(t, newFeedServer(t), store)
	err := m.Import(SourceKEV, 0, strings.NewReader("<html>captive portal</html>"))
	if !errors.Is(err, ErrBadImport) || strings.Contains(err.Error(), "captive") || store.writes != 0 {
		t.Errorf("err %v writes %d", err, store.writes)
	}
	if m.Status().State == StateRunning {
		t.Error("a refused import left the manager running")
	}
}

func TestImport_RefusesAnUnknownSourceAndAYearNobodyWants(t *testing.T) {
	m := newManager(t, newFeedServer(t), &memStore{})
	if err := m.Import("vulners", 0, strings.NewReader(kevBody)); !errors.Is(err, ErrBadSource) {
		t.Errorf("source: %v", err)
	}
	for _, year := range []int{0, 1999, clock.Year() + 1, clock.Year() - yearsWanted} {
		if err := m.Import(SourceNVD, year, bytes.NewReader(nvdBody(2026))); !errors.Is(err, ErrBadYear) {
			t.Errorf("year %d: %v", year, err)
		}
	}
}

func TestImport_AFileOverTheSizeCapIsRefused(t *testing.T) {
	store := &memStore{}
	m := newManager(t, newFeedServer(t), store)
	m.maxFileBytes = 10
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); !errors.Is(err, ErrBadImport) || store.writes != 0 {
		t.Errorf("err %v writes %d", err, store.writes)
	}
}

func TestImport_SaveFailureIsReportedAndKeepsWhatWasHeld(t *testing.T) {
	store := &memStore{wErr: errors.New("disk full")}
	m := newManager(t, newFeedServer(t), store)
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); err == nil || errors.Is(err, ErrBadImport) {
		t.Errorf("err %v", err)
	}
	if !m.Status().KEV.ImportedAt.IsZero() {
		t.Error("an unsaved import is shown as held")
	}
}

func TestImport_OneAtATimeWithDownloadsAndWithItself(t *testing.T) {
	fs, store := newFeedServer(t), &memStore{}
	release := make(chan struct{})
	fs.hook = func(string, http.ResponseWriter) bool { <-release; return true }
	m := newManager(t, fs, store)
	if err := m.Start(false); err != nil {
		t.Fatal(err)
	}
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); !errors.Is(err, ErrBusy) {
		t.Errorf("import during a download: %v", err)
	}
	close(release)
	m.Cancel()
	waitFor(t, m, StateStopped)

	gate := &blockingReader{started: make(chan struct{}), release: make(chan struct{}), data: kevBody}
	done := make(chan error, 1)
	go func() { done <- m.Import(SourceKEV, 0, gate) }()
	<-gate.started
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); !errors.Is(err, ErrBusy) {
		t.Errorf("second import: %v", err)
	}
	if err := m.Start(false); !errors.Is(err, ErrBusy) {
		t.Errorf("download during an import: %v", err)
	}
	m.Cancel() // must not panic: an import has no download to cancel
	close(gate.release)
	if err := <-done; err != nil {
		t.Errorf("first import: %v", err)
	}
}

type blockingReader struct {
	started, release chan struct{}
	data             string
	once             sync.Once
	r                *strings.Reader
}

func (b *blockingReader) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started); <-b.release; b.r = strings.NewReader(b.data) })
	return b.r.Read(p)
}

func TestImport_BeforeTheApplicationIsAttachedSaysItIsNotReady(t *testing.T) {
	m := New(&memStore{}, nil, DefaultSources())
	if err := m.Import(SourceKEV, 0, strings.NewReader(kevBody)); !errors.Is(err, ErrNotReady) {
		t.Errorf("err %v", err)
	}
}

func TestImport_AFileCutShortIsAnUnusableFileNotAServerFault(t *testing.T) {
	store := &memStore{}
	m := newManager(t, newFeedServer(t), store)
	whole := nvdBody(2026)
	if err := m.Import(SourceNVD, 2026, bytes.NewReader(whole[:len(whole)/2])); !errors.Is(err, ErrBadImport) || store.writes != 0 {
		t.Errorf("err %v writes %d", err, store.writes)
	}
}
