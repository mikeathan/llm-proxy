package assistanttest

import (
	"database/sql"
	"os"
	"testing"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
)

// NewMemoryStoreAndDB opens a memory store on a temporary database, removed when the test ends. It also returns the raw
// database, for tests that must backdate timestamps (same-second inserts tie on updated_at).
func NewMemoryStoreAndDB(t *testing.T) (*memory.Store, *sql.DB) {
	t.Helper()
	f, err := os.CreateTemp("", "agent-memory-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.DB().Close() })

	memStore, err := memory.New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return memStore, p.DB()
}
