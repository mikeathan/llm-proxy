package reminders

import (
	"strings"
	"testing"
	"time"
)

func londonZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParseCommand(t *testing.T) {
	loc := londonZone(t)
	// Monday 12 Oct 2026, 20:29 BST.
	evening := time.Date(2026, 10, 12, 20, 29, 0, 0, loc)
	noon := time.Date(2026, 10, 12, 12, 0, 0, 0, loc)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, loc) }

	tests := []struct {
		name    string
		input   string
		now     time.Time
		wantAt  time.Time
		wantRep string
		wantMsg string
	}{
		{"weekday and time", "sunday 8pm eye test monday 11:35", evening, at(2026, 10, 18, 20, 0), "", "eye test monday 11:35"},
		{"tomorrow with time", "tomorrow 9am call the bank", evening, at(2026, 10, 13, 9, 0), "", "call the bank"},
		{"tomorrow default hour", "tomorrow call bank", evening, at(2026, 10, 13, 9, 0), "", "call bank"},
		{"tomorrow evening", "tomorrow evening water plants", evening, at(2026, 10, 13, 18, 0), "", "water plants"},
		{"tonight", "tonight take the bins out", noon, at(2026, 10, 12, 20, 0), "", "take the bins out"},
		{"today with time", "today 5pm stretch", noon, at(2026, 10, 12, 17, 0), "", "stretch"},
		{"in hours glued", "in 2h check the oven", evening, evening.Add(2 * time.Hour), "", "check the oven"},
		{"in minutes", "in 30 minutes tea", evening, evening.Add(30 * time.Minute), "", "tea"},
		{"in days", "in 2 days renew", evening, evening.Add(48 * time.Hour), "", "renew"},
		{"ordinal of month", "18th of october 8pm eye test", evening, at(2026, 10, 18, 20, 0), "", "eye test"},
		{"day month abbreviated", "18 oct 20:00 eye test", evening, at(2026, 10, 18, 20, 0), "", "eye test"},
		{"month day", "oct 18 8pm eye test", evening, at(2026, 10, 18, 20, 0), "", "eye test"},
		{"slash date is day first", "18/10 8pm eye test", evening, at(2026, 10, 18, 20, 0), "", "eye test"},
		{"iso date", "2026-10-18 20:00 eye test", evening, at(2026, 10, 18, 20, 0), "", "eye test"},
		{"date without time", "18th october eye test", evening, at(2026, 10, 18, 9, 0), "", "eye test"},
		{"past month rolls to next year", "18th of september pay rent", evening, at(2027, 9, 18, 9, 0), "", "pay rent"},
		{"explicit year", "1 mar 2027 10:00 renew", evening, at(2027, 3, 1, 10, 0), "", "renew"},
		{"same weekday is next week", "monday 11:35 optician", evening, at(2026, 10, 19, 11, 35), "", "optician"},
		{"next weekday", "next friday 5pm drinks", evening, at(2026, 10, 16, 17, 0), "", "drinks"},
		{"at keyword", "tomorrow at 9am call", evening, at(2026, 10, 13, 9, 0), "", "call"},
		{"case insensitive keeps message case", "Tomorrow 9AM Call Bank  now", evening, at(2026, 10, 13, 9, 0), "", "Call Bank  now"},
		{"noon", "tomorrow noon lunch", evening, at(2026, 10, 13, 12, 0), "", "lunch"},
		{"twelve am is midnight", "tomorrow 12am x", evening, at(2026, 10, 13, 0, 0), "", "x"},
		{"twelve pm is noon", "tomorrow 12pm x", evening, at(2026, 10, 13, 12, 0), "", "x"},
		{"colon forces the split", "tomorrow 9am: March for charity", evening, at(2026, 10, 13, 9, 0), "", "March for charity"},
		{"bare number is not a time", "18 oct 5 things to buy", evening, at(2026, 10, 18, 9, 0), "", "5 things to buy"},
		{"time alone, still ahead today", "5pm stretch", noon, at(2026, 10, 12, 17, 0), "", "stretch"},
		{"time alone with at", "at 5pm stretch", noon, at(2026, 10, 12, 17, 0), "", "stretch"},
		{"time alone, already passed rolls to tomorrow", "8pm call mum", evening, at(2026, 10, 13, 20, 0), "", "call mum"},
		{"morning time passed", "9am call", evening, at(2026, 10, 13, 9, 0), "", "call"},
		{"part of day alone", "evening water plants", noon, at(2026, 10, 12, 18, 0), "", "water plants"},
		{"days across the clock change keep the wall clock", "in 14 days renew", time.Date(2026, 10, 20, 9, 0, 0, 0, loc), at(2026, 11, 3, 9, 0), "", "renew"},
		{"every weekday", "every weekday 9am stand-up", evening, time.Time{}, "0 9 * * 1-5", "stand-up"},
		{"every day with minutes", "every day 8:30am vitamins", evening, time.Time{}, "30 8 * * *", "vitamins"},
		{"every monday default hour", "every monday weekly review", evening, time.Time{}, "0 9 * * 1", "weekly review"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCommand(tc.input, tc.now, loc)
			if err != nil {
				t.Fatalf("ParseCommand(%q): %v", tc.input, err)
			}
			if !got.At.Equal(tc.wantAt) {
				t.Errorf("At = %v, want %v", got.At, tc.wantAt)
			}
			if got.Cron != tc.wantRep {
				t.Errorf("Cron = %q, want %q", got.Cron, tc.wantRep)
			}
			if got.Message != tc.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tc.wantMsg)
			}
		})
	}
}

func TestParseCommandRejects(t *testing.T) {
	loc := londonZone(t)
	now := time.Date(2026, 10, 12, 20, 29, 0, 0, loc)
	tests := []struct {
		name, input, wantErr string
	}{
		{"empty", "", "when"},
		{"no message", "tomorrow 9am", "message"},
		{"date only", "tomorrow", "message"},
		{"unknown word", "blah 9am call", `"blah"`},
		{"bad hour", "tomorrow 13pm x", `"13pm"`},
		{"bad day", "31 feb 9am x", "31 feb"},
		{"bad minutes", "tomorrow 9:75am x", `"9:75am"`},
		{"zero amount", "in 0h x", "in 0h"},
		{"every needs a pattern", "every blue moon x", `"blue"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCommand(tc.input, now, loc)
			if err == nil {
				t.Fatalf("ParseCommand(%q) succeeded, want error", tc.input)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}
