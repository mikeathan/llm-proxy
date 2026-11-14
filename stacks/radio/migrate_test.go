package radio

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A file made by v0.86.0 has the first schema and user_version 0, and rows. Opening it with this release must upgrade it and keep the rows.
func TestAFileFromTheFirstReleaseIsUpgradedInPlace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "radio.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO devices(tag, owner, sealed, label, class, visits, first_day, last_day) VALUES(x'0102', 1, x'ff', 'phone', '', 3, 10, 12)`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()

	st, err := OpenStore(p, 0)
	if err != nil {
		t.Fatalf("the upgrade failed: %v", err)
	}
	defer st.Close()
	d, ok, err := st.Device(context.Background(), []byte{1, 2})
	if err != nil || !ok || d.Label != "phone" || d.Visits != 3 {
		t.Fatalf("the old row must survive: %+v %v %v", d, ok, err)
	}
	if err := st.SeenDevice(context.Background(), Seen{Tag: []byte{9}, Signal: -50, SignalOK: true, At: time.Unix(86400*20, 0)}); err != nil {
		t.Fatalf("a write after the upgrade: %v", err)
	}
	var v int
	if err := st.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != schemaVersion {
		t.Errorf("user_version %d, want %d (%v)", v, schemaVersion, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenStore(p, 0) // a second open must not run the steps again
	if err != nil {
		t.Fatalf("a second open: %v", err)
	}
	_ = again.Close()
}
