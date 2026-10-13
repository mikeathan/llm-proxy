package reminders

import (
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestNextFire_CronAcrossDST(t *testing.T) {
	r := Reminder{Cron: "0 9 * * 1", Timezone: "Europe/London"}
	// Monday 2026-10-19 09:00 BST (UTC+1) = 08:00Z; clocks go back 2026-10-25.
	first, ok := NextFire(r, time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	if !ok || !first.Equal(time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("first = %v ok=%v, want 08:00Z", first, ok)
	}
	second, ok := NextFire(r, first)
	if !ok || !second.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("second = %v ok=%v, want 09:00Z (GMT)", second, ok)
	}
}

func TestNextFire_OneOff(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	r := Reminder{FireAt: at}
	if got, ok := NextFire(r, at.Add(-time.Minute)); !ok || !got.Equal(at) {
		t.Fatalf("before: %v %v", got, ok)
	}
	if _, ok := NextFire(r, at); ok {
		t.Fatal("one-off must not fire again once fired")
	}
}

func TestNextFire_UntilExhausted(t *testing.T) {
	until := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	r := Reminder{Cron: "0 9 * * *", Timezone: "UTC", Until: &until}
	if _, ok := NextFire(r, time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)); ok {
		t.Fatal("expected no further fire after until")
	}
}

func TestResolveZone_Order(t *testing.T) {
	if l, _ := ResolveZone("Asia/Tokyo", "Europe/London"); l.String() != "Asia/Tokyo" {
		t.Errorf("requested should win, got %s", l)
	}
	if l, _ := ResolveZone("", "Europe/London"); l.String() != "Europe/London" {
		t.Errorf("owner zone should be next, got %s", l)
	}
	// No zone at all: UTC by name. time.Local's own name is "Local", which is not a
	// zone a user or a browser can read.
	if l, err := ResolveZone("", ""); err != nil || l.String() != "UTC" {
		t.Errorf("fallback should be UTC, got %v %v", l, err)
	}
}

func TestResolveZone_Invalid(t *testing.T) {
	_, err := ResolveZone("Mars/Base", "")
	if err == nil || !strings.Contains(err.Error(), "Mars/Base") {
		t.Fatalf("want error naming the bad zone, got %v", err)
	}
}

func TestValidateNew_Rejects(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	pastUntil := now.Add(-time.Hour)
	cases := map[string]Reminder{
		"past time":     {Message: "x", FireAt: past},
		"two years out": {Message: "x", FireAt: now.AddDate(2, 0, 0)},
		"blank message": {Message: "  \n", FireAt: now.Add(time.Hour)},
		"every minute":  {Message: "x", FireAt: now.Add(time.Hour), Cron: "* * * * *", Timezone: "UTC"},
		"bad cron":      {Message: "x", FireAt: now.Add(time.Hour), Cron: "nope", Timezone: "UTC"},
		"until in past": {Message: "x", FireAt: now.Add(time.Hour), Cron: "0 9 * * *", Timezone: "UTC", Until: &pastUntil},
	}
	for name, r := range cases {
		if err := ValidateNew(r, now); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	ok := Reminder{Message: "call bank", FireAt: now.Add(time.Hour)}
	if err := ValidateNew(ok, now); err != nil {
		t.Errorf("valid reminder rejected: %v", err)
	}
	okCron := Reminder{Message: "x", FireAt: now.Add(time.Hour), Cron: "0 9 * * 1", Timezone: "UTC"}
	if err := ValidateNew(okCron, now); err != nil {
		t.Errorf("valid recurring rejected: %v", err)
	}
}

func TestChatZone_Order(t *testing.T) {
	tests := []struct{ name, owner, browser, want string }{
		{"owner setting wins", "Europe/London", "Atlantic/Canary", "Europe/London"},
		{"browser when no owner setting", "", "Atlantic/Canary", "Atlantic/Canary"},
		{"invalid owner is skipped", "Mars/Base", "Atlantic/Canary", "Atlantic/Canary"},
		{"invalid browser is skipped", "", "Local", "UTC"},
		{"nothing known is UTC", "", "", "UTC"},
	}
	for _, tc := range tests {
		if got := ChatZone(tc.owner, tc.browser); got != tc.want {
			t.Errorf("%s: ChatZone(%q, %q) = %q, want %q", tc.name, tc.owner, tc.browser, got, tc.want)
		}
	}
}
