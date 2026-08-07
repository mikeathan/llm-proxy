package storage

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"llm-proxy/models"
)

type testDoc struct {
	Name  string            `json:"name" yaml:"name"`
	Count int               `json:"count" yaml:"count"`
	Tags  map[string]string `json:"tags" yaml:"tags"`
	Child *testChild        `json:"child" yaml:"child"`
}

type testChild struct {
	Value string `json:"value" yaml:"value"`
}

func TestStore_JSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	s := NewStore[testDoc](path)

	if err := s.Update(func(d *testDoc) error {
		d.Name = "alpha"
		d.Count = 7
		d.Tags = map[string]string{"k": "v"}
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not persisted: %v", err)
	}

	s2 := NewStore[testDoc](path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.Get()
	if got.Name != "alpha" || got.Count != 7 || got.Tags["k"] != "v" {
		t.Errorf("reloaded doc = %+v", got)
	}
}

func TestStore_YAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	s := NewStore[testDoc](path)

	if err := s.Update(func(d *testDoc) error {
		d.Name = "beta"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	s2 := NewStore[testDoc](path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s2.Get(); got.Name != "beta" {
		t.Errorf("reloaded name = %q, want beta", got.Name)
	}
}

func TestStore_MissingFileZeroValue(t *testing.T) {
	dir := t.TempDir()
	s := NewStore[testDoc](filepath.Join(dir, "missing.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	got := s.Get()
	if got.Name != "" || got.Tags != nil {
		t.Errorf("expected zero value, got %+v", got)
	}
}

// TestStore_GetReturnsDeepCopy is the C1 regression: Get() must not hand out
// mutable internals (maps / pointers) that a concurrent Update can race with.
func TestStore_GetReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	s := NewStore[testDoc](filepath.Join(dir, "d.json"))
	if err := s.Update(func(d *testDoc) error {
		d.Tags = map[string]string{"k": "v"}
		d.Child = &testChild{Value: "x"}
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := s.Get()
	got.Tags["k"] = "mutated"
	got.Child.Value = "mutated"

	again := s.Get()
	if again.Tags["k"] != "v" {
		t.Errorf("map mutated through Get() copy: %v", again.Tags)
	}
	if again.Child.Value != "x" {
		t.Errorf("pointer mutated through Get() copy: %v", again.Child)
	}
}

func TestStore_OnChangeFanOut(t *testing.T) {
	dir := t.TempDir()
	s := NewStore[testDoc](filepath.Join(dir, "f.json"))

	var calls int32
	s.OnChange(func(d testDoc) { atomic.AddInt32(&calls, 1) })

	if err := s.Update(func(d *testDoc) error { d.Name = "x"; return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 callback after Update, got %d", got)
	}
}

// TestStore_LoadSkipsNoopReload is the C2 regression: reloading a file whose
// content is unchanged (e.g. the watcher reloading our own atomic write) must
// not re-fire OnChange.
func TestStore_LoadSkipsNoopReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "n.json")
	s := NewStore[testDoc](path)

	var calls int32
	s.OnChange(func(d testDoc) { atomic.AddInt32(&calls, 1) })

	if err := s.Update(func(d *testDoc) error { d.Name = "x"; return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("no-op Load re-fired OnChange: calls = %d, want 1", got)
	}

	// External modification must fire exactly once.
	if err := os.WriteFile(path, []byte(`{"name":"y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("external change did not fire OnChange: calls = %d, want 2", got)
	}
}

func TestStore_ConcurrentGetUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	s := NewStore[testDoc](path)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.Update(func(d *testDoc) error {
				d.Count++
				if d.Tags == nil {
					d.Tags = map[string]string{}
				}
				d.Tags["k"] = "v"
				return nil
			})
		}()
		go func() {
			defer wg.Done()
			// Read through the returned value to exercise the copy path.
			d := s.Get()
			if d.Tags != nil {
				_ = d.Tags["k"]
			}
			if d.Child != nil {
				_ = d.Child.Value
			}
		}()
	}
	wg.Wait()

	if s.Get().Count != 20 {
		t.Errorf("count = %d, want 20", s.Get().Count)
	}
}

func TestStore_UpdateFailureLeavesDataUnchanged(t *testing.T) {
	dir := t.TempDir()
	s := NewStore[testDoc](filepath.Join(dir, "e.json"))

	wantErr := os.ErrClosed
	if err := s.Update(func(d *testDoc) error {
		d.Name = "should-not-persist"
		return wantErr
	}); err == nil {
		t.Fatal("expected Update to return the callback error")
	}

	// In-memory data must be unchanged (callback applied then rolled back).
	if got := s.Get(); got.Name != "" {
		t.Errorf("in-memory data mutated by failed Update: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "e.json")); !os.IsNotExist(err) {
		t.Errorf("file should not exist after failed Update")
	}
}

func TestStore_ModelsRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, models.RegistryFilename)
	s := NewStore[models.RegistryData](path)

	if err := s.Update(func(r *models.RegistryData) error {
		r.PrimaryModel = "alpha"
		r.Catalogue = append(r.Catalogue, models.ModelRegistryEntry{Name: "alpha", ProviderID: "local"})
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	s2 := NewStore[models.RegistryData](path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.Get()
	if got.PrimaryModel != "alpha" || len(got.Catalogue) != 1 {
		t.Errorf("registry round-trip mismatch: %+v", got)
	}
}
