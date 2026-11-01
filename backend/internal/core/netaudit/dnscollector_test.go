package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/dnscheck"
	"llm-proxy/models"
)

type fakeDNS struct {
	mu    sync.Mutex
	by    map[string][]netip.Addr
	err   error
	asked []string
}

func (f *fakeDNS) LookupA(_ context.Context, name string) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, name)
	if f.err != nil {
		return nil, f.err
	}
	return f.by[name], nil
}

func (f *fakeDNS) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

type memDNSStore struct {
	items    []models.DNSComparison
	readErr  error
	writeErr error
	writes   int
}

func (m *memDNSStore) Read() ([]models.DNSComparison, error) { return m.items, m.readErr }
func (m *memDNSStore) Write(v []models.DNSComparison) error {
	m.writes++
	if m.writeErr != nil {
		return m.writeErr
	}
	m.items = v
	return nil
}

func dnsIPs(in ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range in {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

var dnsNow = time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC)

func newTestCollector(router, outside *fakeDNS, store *memDNSStore, names ...string) (*DNSCollector, *[]error) {
	var errs []error
	c := NewDNSCollector(DNSCollectorDeps{
		Settings: func() DNSSettings { return DNSSettings{Enabled: true, Every: time.Hour, Names: names} },
		Router:   router,
		Outside:  outside,
		Store:    store,
		Now:      func() time.Time { return dnsNow },
		OnError:  func(err error) { errs = append(errs, err) },
	})
	return c, &errs
}

func TestDNSCollector_LogsAMatchWhateverTheOrderAndAdvisesNothing(t *testing.T) {
	router := &fakeDNS{by: map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8", "8.8.4.4")}}
	outside := &fakeDNS{by: map[string][]netip.Addr{"dns.google": dnsIPs("8.8.4.4", "8.8.8.8")}}
	store := &memDNSStore{}
	c, errs := newTestCollector(router, outside, store, "dns.google")
	c.Tick(context.Background())

	if len(store.items) != 1 {
		t.Fatalf("got %+v", store.items)
	}
	got := store.items[0]
	if got.Outcome != models.DNSMatch || got.Name != "dns.google" || !got.At.Equal(dnsNow) || !got.Overlap {
		t.Errorf("got %+v", got)
	}
	if !slices.Equal(got.Router, []string{"8.8.8.8", "8.8.4.4"}) || !slices.Equal(got.Outside, []string{"8.8.4.4", "8.8.8.8"}) {
		t.Errorf("both answers are logged as given: %+v", got)
	}
	if len(*errs) != 0 || c.Status().Problem != "" || !c.Status().LastRun.Equal(dnsNow) {
		t.Errorf("errors %v, status %+v", *errs, c.Status())
	}
}

func TestDNSCollector_ADifferentAnswerIsLoggedWithBothSetsAndWhetherTheyOverlap(t *testing.T) {
	router := &fakeDNS{by: map[string][]netip.Addr{"a.example": dnsIPs("203.0.113.9"), "b.example": dnsIPs("8.8.8.8", "203.0.113.9")}}
	outside := &fakeDNS{by: map[string][]netip.Addr{"a.example": dnsIPs("8.8.8.8"), "b.example": dnsIPs("8.8.8.8", "8.8.4.4")}}
	store := &memDNSStore{}
	c, _ := newTestCollector(router, outside, store, "a.example", "b.example")
	c.Tick(context.Background())

	if len(store.items) != 2 || store.items[0].Outcome != models.DNSDiffer || store.items[1].Outcome != models.DNSDiffer {
		t.Fatalf("got %+v", store.items)
	}
	if store.items[0].Overlap || !store.items[1].Overlap {
		t.Errorf("overlap: %+v", store.items)
	}
	if !slices.Equal(store.items[0].Router, []string{"203.0.113.9"}) || !slices.Equal(store.items[0].Outside, []string{"8.8.8.8"}) {
		t.Errorf("got %+v", store.items[0])
	}
}

// A side that gave nothing is "cannot compare", never a match and never a difference.
func TestDNSCollector_ASideThatFailedIsCannotCompareWithAFixedReason(t *testing.T) {
	good := &fakeDNS{by: map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8")}}
	cases := []struct {
		name            string
		router, outside *fakeDNS
		reason          string
	}{
		{"router unknown", &fakeDNS{err: dnscheck.ErrNoRouter}, good, models.DNSReasonNoRouter},
		{"router silent", &fakeDNS{err: errors.New("read udp: i/o timeout 10.1.2.3 secret-device-name")}, good, models.DNSReasonRouterSilent},
		{"outside failed", good, &fakeDNS{err: errors.New("503 from the service: <script>")}, models.DNSReasonOutsideFails},
		{"outside unreadable", good, &fakeDNS{err: dnscheck.ErrBadAnswer}, models.DNSReasonOutsideFails},
	}
	for _, tc := range cases {
		store := &memDNSStore{}
		c, _ := newTestCollector(tc.router, tc.outside, store, "dns.google")
		c.Tick(context.Background())
		if len(store.items) != 1 || store.items[0].Outcome != models.DNSCannotCompare || store.items[0].Reason != tc.reason {
			t.Errorf("%s: got %+v", tc.name, store.items)
			continue
		}
		if store.items[0].Router != nil && tc.router.err != nil || store.items[0].Outside != nil && tc.outside.err != nil {
			t.Errorf("%s: a failed side has no addresses: %+v", tc.name, store.items[0])
		}
	}
}

// With internet access off nothing can be compared and nothing is logged: a log full of "cannot compare" rows would be noise.
func TestDNSCollector_InternetOffLogsNothingAndSaysSo(t *testing.T) {
	router := &fakeDNS{by: map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8")}}
	outside := &fakeDNS{err: dnscheck.ErrInternetOff}
	store := &memDNSStore{}
	c, _ := newTestCollector(router, outside, store, "dns.google", "one.one.one.one")
	c.Tick(context.Background())

	if len(store.items) != 0 || store.writes != 0 {
		t.Errorf("nothing may be logged: %+v", store.items)
	}
	if outside.count() != 1 {
		t.Errorf("the outside resolver is asked once, not for every name: %d", outside.count())
	}
	if got := c.Status().Problem; got != DNSProblemInternetOff {
		t.Errorf("problem = %q", got)
	}
	outside.err = nil
	outside.by = map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8"), "one.one.one.one": dnsIPs("1.1.1.1")}
	router.by["one.one.one.one"] = dnsIPs("1.1.1.1")
	c.Tick(context.Background())
	if c.Status().Problem != "" || len(store.items) != 2 {
		t.Errorf("after access came back: %+v %+v", c.Status(), store.items)
	}
}

func TestDNSCollector_DisabledOrNoNamesAsksNobody(t *testing.T) {
	router, outside := &fakeDNS{}, &fakeDNS{}
	store := &memDNSStore{}
	c, _ := newTestCollector(router, outside, store, "dns.google")
	c.deps.Settings = func() DNSSettings { return DNSSettings{Enabled: false, Names: []string{"dns.google"}} }
	c.Tick(context.Background())
	c.deps.Settings = func() DNSSettings { return DNSSettings{Enabled: true} }
	c.Tick(context.Background())
	if router.count()+outside.count() != 0 || store.writes != 0 {
		t.Errorf("asked %d %d, wrote %d", router.count(), outside.count(), store.writes)
	}
}

func TestDNSCollector_TheLogIsBoundedAndKeepsTheNewest(t *testing.T) {
	var old []models.DNSComparison
	for i := 0; i < MaxDNSComparisons; i++ {
		old = append(old, models.DNSComparison{Name: fmt.Sprintf("n%d.example", i), Outcome: models.DNSMatch})
	}
	store := &memDNSStore{items: old}
	same := map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8")}
	c, _ := newTestCollector(&fakeDNS{by: same}, &fakeDNS{by: same}, store, "dns.google")
	c.Tick(context.Background())
	if len(store.items) != MaxDNSComparisons {
		t.Fatalf("len = %d", len(store.items))
	}
	if store.items[0].Name != "n1.example" || store.items[len(store.items)-1].Name != "dns.google" {
		t.Errorf("first %q last %q", store.items[0].Name, store.items[len(store.items)-1].Name)
	}
}

func TestDNSCollector_StoreTroubleIsReportedOnceAndNeverHidesTheOutcome(t *testing.T) {
	same := map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8")}
	store := &memDNSStore{writeErr: errors.New("disk full")}
	c, errs := newTestCollector(&fakeDNS{by: same}, &fakeDNS{by: same}, store, "dns.google")
	c.Tick(context.Background())
	c.Tick(context.Background())
	if len(*errs) != 1 {
		t.Errorf("a repeating failure is reported once: %v", *errs)
	}
	store.writeErr = nil
	c.Tick(context.Background())
	store.writeErr = errors.New("disk full")
	c.Tick(context.Background())
	if len(*errs) != 2 {
		t.Errorf("after it ended, a new failure is reported again: %v", *errs)
	}

	broken := &memDNSStore{readErr: errors.New("corrupt")}
	c, errs = newTestCollector(&fakeDNS{by: same}, &fakeDNS{by: same}, broken, "dns.google")
	c.Tick(context.Background())
	if broken.writes != 0 || len(*errs) != 1 {
		t.Errorf("an unreadable log is not overwritten: writes %d, errors %v", broken.writes, *errs)
	}
}

func TestDNSCollector_SummaryCountsAndListsTheNewestDifferences(t *testing.T) {
	var items []models.DNSComparison
	at := dnsNow.Add(-48 * time.Hour)
	add := func(o models.DNSOutcome, overlap bool, name string) {
		at = at.Add(time.Minute)
		items = append(items, models.DNSComparison{At: at, Name: name, Outcome: o, Overlap: overlap})
	}
	add(models.DNSMatch, true, "m1")
	add(models.DNSCannotCompare, false, "c1")
	for i := 0; i < MaxDNSDifferences+3; i++ {
		add(models.DNSDiffer, i%2 == 0, fmt.Sprintf("d%d", i))
	}
	add(models.DNSMatch, true, "m2")
	c, _ := newTestCollector(nil, nil, &memDNSStore{items: items}, "x")
	got, err := c.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if got.Comparisons != 2+MaxDNSDifferences+3 || got.Matches != 2 || got.Differs != MaxDNSDifferences+3 || got.CannotCompare != 1 {
		t.Errorf("counts %+v", got)
	}
	if got.Overlapping != (MaxDNSDifferences+3+1)/2 {
		t.Errorf("overlapping = %d", got.Overlapping)
	}
	if !got.First.Equal(items[0].At) || !got.Last.Equal(items[len(items)-1].At) {
		t.Errorf("first %v last %v", got.First, got.Last)
	}
	if len(got.Differences) != MaxDNSDifferences || got.Differences[0].Name != fmt.Sprintf("d%d", MaxDNSDifferences+2) {
		t.Errorf("newest first, bounded: %d, first %q", len(got.Differences), got.Differences[0].Name)
	}
	empty, _ := newTestCollector(nil, nil, &memDNSStore{}, "x")
	if s, _ := empty.Summary(); s.Differences == nil || s.Comparisons != 0 {
		t.Errorf("an empty log still lists no differences as an empty list: %+v", s)
	}
}

func TestDNSCollector_RunLooksOnlyWhileEnabledAndStopsWithTheContext(t *testing.T) {
	same := map[string][]netip.Addr{"dns.google": dnsIPs("8.8.8.8")}
	router := &fakeDNS{by: same}
	store := &memDNSStore{}
	c, _ := newTestCollector(router, &fakeDNS{by: same}, store, "dns.google")
	c.deps.Settings = func() DNSSettings {
		return DNSSettings{Enabled: false, Every: time.Millisecond, Names: []string{"dns.google"}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if router.count() != 0 {
		t.Error("a disabled collector asks nobody")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run must stop when ctx ends")
	}

	c.deps.Settings = func() DNSSettings { return DNSSettings{Enabled: true, Every: time.Hour, Names: []string{"dns.google"}} }
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for router.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if router.count() != 1 {
		t.Errorf("an enabled collector looks once at once, then waits its interval: %d", router.count())
	}
}
