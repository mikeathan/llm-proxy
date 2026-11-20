package newsroom

import (
	"context"
	"testing"
	"time"
)

// The feed's days are the owner's: the zone the screen reports wins, then the stored setting, and a name nobody knows can only fall back.
func TestFeedZone_TheScreensZoneThenTheSettingThenUTC(t *testing.T) {
	s := start(t, `{"timezone":"Asia/Tokyo"}`, &rssFetch{})
	ctx := context.Background()
	cases := []struct{ asked, want string }{{"Europe/London", "Europe/London"}, {"", "Asia/Tokyo"}, {"Not/AZone", "Asia/Tokyo"}}
	for _, c := range cases {
		if got := s.feedZone(ctx, c.asked).String(); got != c.want {
			t.Errorf("asked %q: %s, want %s", c.asked, got, c.want)
		}
	}
	plain := start(t, `{}`, &rssFetch{})
	if got := plain.feedZone(ctx, "").String(); got != time.UTC.String() {
		t.Errorf("no zone anywhere is UTC, got %s", got)
	}
}
