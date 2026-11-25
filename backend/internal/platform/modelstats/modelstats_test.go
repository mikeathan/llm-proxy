package modelstats

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore(t *testing.T) (*Store, *clock, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model-stats.json")
	c := &clock{t: time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local)}
	s, err := Open(path, c.now)
	if err != nil {
		t.Fatal(err)
	}
	return s, c, path
}

func TestRecord_CountsCallsTokensFailuresAndSpeedPerModel(t *testing.T) {
	s, _, _ := newStore(t)
	s.Record(Call{Model: "Qwen3.5", Local: true, PromptTokens: 100, CompletionTokens: 50, TokensPerSec: 40})
	s.Record(Call{Model: "Qwen3.5", Local: true, PromptTokens: 10, CompletionTokens: 5, TokensPerSec: 60})
	s.Record(Call{Model: "Qwen3.5", Local: true, Failed: true})
	s.Record(Call{Model: "deepseek", PromptTokens: 7, CompletionTokens: 3})
	rep := s.Snapshot(7)
	if len(rep.Today) != 2 || rep.Today[0].Model != "Qwen3.5" {
		t.Fatalf("today: %+v", rep.Today)
	}
	q := rep.Today[0]
	if q.Calls != 3 || q.Failed != 1 || q.PromptTokens != 110 || q.CompletionTokens != 55 || q.TokensPerSec != 50 || !q.Local {
		t.Errorf("qwen: %+v", q)
	}
	if d := rep.Today[1]; d.Local || d.Calls != 1 || d.TokensPerSec != 0 {
		t.Errorf("a cloud call has no speed: %+v", d)
	}
	if last := rep.Days[len(rep.Days)-1]; last.Local != 3 || last.Cloud != 1 || last.Date != "2026-11-24" {
		t.Errorf("today's totals: %+v", last)
	}
}

func TestSnapshot_GivesEachDayOldestFirstWithZeroDaysFilledIn(t *testing.T) {
	s, c, _ := newStore(t)
	s.Record(Call{Model: "a", Local: true})
	c.t = c.t.AddDate(0, 0, 2)
	s.Record(Call{Model: "b"})
	rep := s.Snapshot(3)
	if len(rep.Days) != 3 || rep.Days[0].Local != 1 || rep.Days[1] != (DayTotal{Date: "2026-11-25"}) || rep.Days[2].Cloud != 1 {
		t.Errorf("days: %+v", rep.Days)
	}
	if len(rep.Today) != 1 || rep.Today[0].Model != "b" {
		t.Errorf("today is only today's: %+v", rep.Today)
	}
	if got := len(s.Snapshot(0).Days); got != 1 {
		t.Errorf("at least one day: %d", got)
	}
	if got := len(s.Snapshot(1000).Days); got != RetainDays {
		t.Errorf("at most %d days: %d", RetainDays, got)
	}
}

func TestRecord_KeepsOneDayTableBoundedAndDropsOldDays(t *testing.T) {
	s, c, _ := newStore(t)
	for i := range MaxModelsPerDay + 20 {
		s.Record(Call{Model: strings.Repeat("m", 1) + string(rune('A'+i%26)) + strings.Repeat("x", i)})
	}
	if got := len(s.Snapshot(1).Today); got != MaxModelsPerDay+1 {
		t.Errorf("rows %d, want %d (the table plus %q)", got, MaxModelsPerDay, OtherModels)
	}
	var calls int
	for _, u := range s.Snapshot(1).Today {
		calls += u.Calls
	}
	if calls != MaxModelsPerDay+20 {
		t.Errorf("a call was lost: %d", calls)
	}
	c.t = c.t.AddDate(0, 0, RetainDays)
	s.Record(Call{Model: "new"})
	if _, old := s.days["2026-11-24"]; old {
		t.Error("an old day is still kept")
	}
}

func TestRecord_ModelNamesAreOnePlainShortLine(t *testing.T) {
	s, _, _ := newStore(t)
	s.Record(Call{Model: "  weird\x00 \n name\t "})
	s.Record(Call{Model: ""})
	s.Record(Call{Model: strings.Repeat("é", 500)})
	for _, u := range s.Snapshot(1).Today {
		if strings.ContainsAny(u.Model, "\x00\n") || len([]rune(u.Model)) > maxModelName {
			t.Errorf("not clean: %q", u.Model)
		}
	}
	if s.Snapshot(1).Today[0].Model == "" {
		t.Error("empty name")
	}
}

func TestStore_FlushAndOpenKeepTheCounts(t *testing.T) {
	s, c, path := newStore(t)
	s.Record(Call{Model: "a", Local: true, PromptTokens: 5, TokensPerSec: 30})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("file: %v %v", info, err)
	}
	again, err := Open(path, c.now)
	if err != nil {
		t.Fatal(err)
	}
	got := again.Snapshot(1).Today
	if len(got) != 1 || got[0].Calls != 1 || got[0].PromptTokens != 5 || got[0].TokensPerSec != 30 || !got[0].Local {
		t.Errorf("after reopening: %+v", got)
	}
}

func TestOpen_AFileThatCannotBeReadStartsEmptyAndSaysWhy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-stats.json")
	if err := os.WriteFile(path, []byte("{not json"), fileMode); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, time.Now)
	if err == nil || s == nil || len(s.Snapshot(1).Today) != 0 {
		t.Fatalf("store %v err %v", s, err)
	}
	s.Record(Call{Model: "a"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if again, err := Open(path, time.Now); err != nil || len(again.Snapshot(1).Today) != 1 {
		t.Errorf("the next write did not replace the bad file: %v", err)
	}
}

func TestRecord_WritesAtMostOnceAMinuteAndFlushWritesTheRest(t *testing.T) {
	s, c, path := newStore(t)
	s.Record(Call{Model: "a"})
	if _, err := os.Stat(path); err == nil {
		t.Fatal("written on the first call, inside the minute")
	}
	c.t = c.t.Add(saveEvery)
	s.Record(Call{Model: "a"})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("not written after a minute: %v", err)
	}
	stat1, _ := os.Stat(path)
	s.Record(Call{Model: "a"})
	if stat2, _ := os.Stat(path); !stat2.ModTime().Equal(stat1.ModTime()) {
		t.Error("written again inside the minute")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if again, _ := Open(path, c.now); again.Snapshot(1).Today[0].Calls != 3 {
		t.Error("Flush did not write the last call")
	}
	if err := s.Flush(); err != nil {
		t.Errorf("a second Flush with nothing new: %v", err)
	}
}

func TestStore_IsSafeForConcurrentCalls(t *testing.T) {
	s, _, _ := newStore(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				s.Record(Call{Model: "a", Local: true})
				_ = s.Snapshot(7)
			}
		}()
	}
	wg.Wait()
	if got := s.Snapshot(1).Today[0].Calls; got != 1000 {
		t.Errorf("calls %d", got)
	}
}

// Record and Flush write from different goroutines. A snapshot taken earlier must never replace a newer one that is already on disk, or the newest counts are lost.
func TestStore_AnOlderSnapshotNeverReplacesANewerOne(t *testing.T) {
	s, c, path := newStore(t)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				s.mu.Lock()
				c.t = c.t.Add(saveEvery) // every call is due for a write, so writes race
				s.mu.Unlock()
				s.Record(Call{Model: "a", Local: true})
				if (g+i)%7 == 0 {
					_ = s.Flush()
				}
			}
		}()
	}
	wg.Wait()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path, c.now)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk, inMemory int
	for _, u := range again.Snapshot(RetainDays).Days {
		onDisk += u.Local
	}
	for _, u := range s.Snapshot(RetainDays).Days {
		inMemory += u.Local
	}
	if inMemory != 800 || onDisk != inMemory {
		t.Errorf("in memory %d, on disk %d, want both 800", inMemory, onDisk)
	}
}
