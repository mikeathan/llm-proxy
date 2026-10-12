package models

import (
	"strings"
	"testing"
	"time"
)

func TestParseInstant(t *testing.T) {
	got, err := ParseInstant("when", " 2026-10-12T09:00:00+01:00 ")
	if err != nil || !got.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)) || got.Location() != time.UTC {
		t.Fatalf("got %v err=%v, want 08:00 UTC", got, err)
	}
	for _, bad := range []string{"", "tomorrow", "2026-10-12", "2026-10-12T09:00:00"} {
		_, err := ParseInstant("fire_at", bad)
		if err == nil || !strings.Contains(err.Error(), "fire_at") || !strings.Contains(err.Error(), "RFC 3339") {
			t.Errorf("%q: err = %v, want one naming the field and RFC 3339", bad, err)
		}
	}
}
