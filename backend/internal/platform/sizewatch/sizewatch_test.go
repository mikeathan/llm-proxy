package sizewatch

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSumDir(t *testing.T) {
	dir := t.TempDir()
	write := func(p, content string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o700)
	write("a.txt", "12345")
	write("sub/b.txt", "123")

	total, err := SumDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if total != 8 {
		t.Errorf("SumDir = %d, want 8", total)
	}
}

func TestCacheTTLAndRefresh(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCache(50 * time.Millisecond)
	now := time.Now()
	if got := c.Bytes(dir, now); got != 4 {
		t.Fatalf("first measure = %d, want 4", got)
	}
	// Cache hit before TTL: grow the file, still old size.
	if err := os.WriteFile(filepath.Join(dir, "f"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := c.Bytes(dir, now.Add(10*time.Millisecond)); got != 4 {
		t.Errorf("cached size = %d, want stale 4 (TTL not elapsed)", got)
	}
	// After TTL the walk re-runs and sees the growth.
	if got := c.Bytes(dir, now.Add(100*time.Millisecond)); got != 1000 {
		t.Errorf("refreshed size = %d, want 1000", got)
	}

	c.Forget(dir)
	if got := c.Bytes(dir, now.Add(200*time.Millisecond)); got != 1000 {
		t.Errorf("after Forget remeasure = %d, want 1000", got)
	}
}

// An unreadable root must surface as an error (never a silent 0), and the cache
// must then keep the last known size rather than reporting the workspace empty.
func TestSumDirUnreadableRootErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read mode-000 directories")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewCache(time.Minute)
	now := time.Now()
	if got := c.Bytes(dir, now); got != 4 {
		t.Fatalf("prime = %d, want 4", got)
	}

	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := SumDir(dir); err == nil {
		t.Error("SumDir on an unreadable root must return an error")
	}
	if got := c.Bytes(dir, now.Add(2*time.Minute)); got != 4 {
		t.Errorf("re-measure after root failure = %d, want last known 4", got)
	}
}

// Concurrent misses for the same root must share ONE measurement: a full
// workspace walk is the expensive part of the pre-spawn accounting check, and
// racing shell spawns must not each traverse the tree.
func TestCacheSingleFlightPerRoot(t *testing.T) {
	c := NewCache(time.Minute)
	var calls int32
	release := make(chan struct{})
	c.measure = func(root string) (int64, error) {
		atomic.AddInt32(&calls, 1)
		<-release // hold the first measurement so every racer queues on the gate
		return 42, nil
	}

	const racers = 8
	results := make(chan int64, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- c.Bytes("/ws", time.Now())
		}()
	}
	// Give the goroutines time to reach the in-flight gate, then let the
	// single measurement complete.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("measurement calls = %d, want 1 (single-flight per root)", got)
	}
	for size := range results {
		if size != 42 {
			t.Errorf("Bytes = %d, want 42 from the shared measurement", size)
		}
	}
}
