package netaudit

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

func TestHistory_RouterBlocksAddUpOverFlushesAndKeepTheTenBusiestPorts(t *testing.T) {
	store := &memHistory{}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	h := historyAt(store, &now)
	if err := h.RecordBlocks("2026-11-04", 5, []models.PortCount{{Port: 22, Count: 3}, {Port: 3306, Count: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := h.RecordBlocks("2026-11-04", 4, []models.PortCount{{Port: 22, Count: 1}, {Port: 23, Count: 3}}); err != nil {
		t.Fatal(err)
	}
	day := store.h.Days[0]
	if day.Date != "2026-11-04" || day.Blocked != 9 {
		t.Fatalf("day = %+v", day)
	}
	want := []models.PortCount{{Port: 22, Count: 4}, {Port: 23, Count: 3}, {Port: 3306, Count: 2}}
	if !slices.Equal(day.BlockedPorts, want) {
		t.Errorf("ports = %+v, want %+v", day.BlockedPorts, want)
	}
	var many []models.PortCount
	for p := 1; p <= 30; p++ {
		many = append(many, models.PortCount{Port: p, Count: p})
	}
	_ = h.RecordBlocks("2026-11-04", 1, many)
	got := store.h.Days[0].BlockedPorts
	if len(got) != MaxBlockedPorts || got[0].Port != 30 {
		t.Errorf("ports kept = %+v", got)
	}
}

func TestHistory_RouterBlocksGoToTheDayTheyWereHeardNotTheDayOfTheWrite(t *testing.T) {
	store := &memHistory{}
	now := time.Date(2026, 11, 5, 0, 1, 0, 0, time.UTC)
	h := historyAt(store, &now)
	if err := h.RecordBlocks("2026-11-04", 7, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.h.Days) != 1 || store.h.Days[0].Date != "2026-11-04" || store.h.Days[0].Blocked != 7 {
		t.Errorf("days = %+v", store.h.Days)
	}
}

func TestHistory_NothingToAddIsNotWritten(t *testing.T) {
	store := &memHistory{}
	now := time.Now()
	h := historyAt(store, &now)
	if err := h.RecordBlocks("2026-11-04", 0, nil); err != nil || store.writes != 0 {
		t.Errorf("writes = %d, err = %v", store.writes, err)
	}
}

type fakeBlockSink struct {
	mu    sync.Mutex
	calls []blockCall
	err   error
}
type blockCall struct {
	date  string
	total int
	ports []models.PortCount
}

func (f *fakeBlockSink) RecordBlocks(date string, total int, ports []models.PortCount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, blockCall{date, total, slices.Clone(ports)})
	return f.err
}

func (f *fakeBlockSink) snapshot() []blockCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeBlockSink) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func tcp(port int) routerlog.Drop { return routerlog.Drop{Proto: "tcp", Port: port} }

// runBlocks starts the counter's writer with a short interval and returns a stop that waits for it to end.
func runBlocks(t *testing.T, b *Blocks, every time.Duration) (stop func()) {
	t.Helper()
	b.every = every
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

// waitForCalls waits until the sink has at least n writes.
func waitForCalls(t *testing.T, sink *fakeBlockSink, n int) []blockCall {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls := sink.snapshot(); len(calls) >= n {
			return calls
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited for %d writes, got %+v", n, sink.snapshot())
	return nil
}

func TestBlocks_CountingWritesNothingByItself(t *testing.T) {
	sink := &fakeBlockSink{}
	b := NewBlocks(sink, nil)
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	b.Add(tcp(22), t0)
	b.Add(tcp(22), t0.Add(time.Hour))
	if calls := sink.snapshot(); len(calls) != 0 {
		t.Fatalf("a drop wrote the history; only the writer and the end of a day do: %+v", calls)
	}
}

func TestBlocks_WhatWasCountedIsWrittenOnTheTimerEvenWhenNoFurtherDropArrives(t *testing.T) {
	sink := &fakeBlockSink{}
	b := NewBlocks(sink, nil)
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	b.Add(tcp(22), t0)
	b.Add(tcp(22), t0.Add(time.Minute))
	b.Add(routerlog.Drop{Proto: "icmp"}, t0.Add(2*time.Minute))
	b.Add(tcp(3306), t0.Add(3*time.Minute))
	stop := runBlocks(t, b, 10*time.Millisecond)
	defer stop()

	c := waitForCalls(t, sink, 1)[0]
	if c.date != "2026-11-04" || c.total != 4 || !slices.Equal(c.ports, []models.PortCount{{Port: 22, Count: 2}, {Port: 3306, Count: 1}}) {
		t.Errorf("call = %+v", c)
	}
	// The next write carries only what came after: the counter is emptied by a write, and an empty one writes nothing.
	time.Sleep(50 * time.Millisecond)
	b.Add(tcp(22), t0.Add(12*time.Minute))
	calls := waitForCalls(t, sink, 2)
	if len(calls) != 2 || calls[1].total != 1 {
		t.Errorf("calls = %+v", calls)
	}
}

func TestBlocks_WhatWasCountedIsWrittenWhenTheApplicationStops(t *testing.T) {
	sink := &fakeBlockSink{}
	b := NewBlocks(sink, nil)
	stop := runBlocks(t, b, time.Hour)
	b.Add(tcp(22), time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	stop()
	if calls := sink.snapshot(); len(calls) != 1 || calls[0].total != 1 {
		t.Fatalf("the counts of the last minutes were lost at shutdown: %+v", calls)
	}
}

func TestBlocks_ADayThatEndsIsWrittenUnderItsOwnDateBeforeTheNewDayStarts(t *testing.T) {
	sink := &fakeBlockSink{}
	b := NewBlocks(sink, nil)
	b.Add(tcp(22), time.Date(2026, 11, 4, 23, 58, 0, 0, time.UTC))
	b.Add(tcp(23), time.Date(2026, 11, 5, 0, 1, 0, 0, time.UTC))
	if calls := sink.snapshot(); len(calls) != 1 || calls[0].date != "2026-11-04" || calls[0].total != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if calls := sink.snapshot(); len(calls) != 2 || calls[1].date != "2026-11-05" || calls[1].total != 1 {
		t.Errorf("calls = %+v", calls)
	}
}

func TestBlocks_AFailedWriteKeepsTheCountsForTheNextOne(t *testing.T) {
	sink := &fakeBlockSink{err: errors.New("disk full")}
	var mu sync.Mutex
	var reported []error
	heard := func() []error { mu.Lock(); defer mu.Unlock(); return slices.Clone(reported) }
	b := NewBlocks(sink, func(err error) { mu.Lock(); reported = append(reported, err); mu.Unlock() })
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	b.Add(tcp(22), t0)
	if err := b.Flush(); err == nil {
		t.Fatal("the failure is returned")
	}
	sink.fail(nil)
	b.Add(tcp(22), t0.Add(time.Minute))
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	calls := sink.snapshot()
	last := calls[len(calls)-1]
	if last.total != 2 {
		t.Errorf("the first count was lost: %+v", last)
	}
	// A timed write that fails is reported, and never stops counting.
	sink.fail(errors.New("again"))
	b.Add(tcp(22), t0.Add(20*time.Minute))
	stop := runBlocks(t, b, 10*time.Millisecond)
	defer stop()
	waitForCalls(t, sink, len(sink.snapshot())+1)
	stop()
	if len(heard()) == 0 {
		t.Error("a timed write that failed was not reported")
	}
}

func TestBlocks_MemoryDoesNotGrowWithTheNumberOfDifferentPorts(t *testing.T) {
	sink := &fakeBlockSink{}
	b := NewBlocks(sink, nil)
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	for p := 1; p <= 5000; p++ {
		b.Add(tcp(p), t0)
	}
	if n := b.distinctPorts(); n > maxCountedPorts {
		t.Errorf("tracking %d ports, above the cap %d", n, maxCountedPorts)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if sink.calls[0].total != 5000 {
		t.Errorf("every attempt is still counted: %d", sink.calls[0].total)
	}
	if len(sink.calls[0].ports) > 2*MaxBlockedPorts {
		t.Errorf("wrote %d ports", len(sink.calls[0].ports))
	}
}

// distinctPorts is how many ports are being counted now; the test above reads it to show the table is bounded.
func (b *Blocks) distinctPorts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.ports)
}
