package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestLockdownStore_MissingFileIsOffAndRoundTrips(t *testing.T) {
	s := NewLockdownStore(t.TempDir())
	if got, err := s.Read(); err != nil || got.On {
		t.Fatalf("missing file = %+v, %v; want off", got, err)
	}
	in := models.Lockdown{On: true, Since: time.Unix(1, 0).UTC(), SignedOut: 2}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Read(); err != nil || !got.On || got.SignedOut != 2 {
		t.Fatalf("read back %+v, %v", got, err)
	}
}
