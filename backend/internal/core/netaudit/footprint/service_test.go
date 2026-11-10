package footprint

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

type memStore struct{ rs []models.FootprintResult }

func (m *memStore) Read() ([]models.FootprintResult, error) { return m.rs, nil }
func (m *memStore) Write(r []models.FootprintResult) error  { m.rs = r; return nil }

type stubs struct {
	certs    []int64
	certErr  error
	expiry   time.Time
	mail     MailRecords
	names    []string
	found    int
	breachEr error
	asked    []string
}

func (s *stubs) Certs(_ context.Context, d string) ([]int64, error) {
	s.asked = append(s.asked, "certs:"+d)
	return s.certs, s.certErr
}
func (s *stubs) Expiry(_ context.Context, d string) (time.Time, error) {
	s.asked = append(s.asked, "expiry:"+d)
	return s.expiry, nil
}
func (s *stubs) Look(_ context.Context, d string) (MailRecords, error) {
	s.asked = append(s.asked, "mail:"+d)
	return s.mail, nil
}

type breachStub struct{ *stubs }

func (b breachStub) Look(_ context.Context, a string) ([]string, int, error) {
	b.asked = append(b.asked, "breach:"+a)
	return b.names, b.found, b.breachEr
}

func newService(c models.FootprintConfig, st *stubs, store *memStore, at *time.Time) *Service {
	return NewService(Deps{Config: func() models.FootprintConfig { return c }, Store: store, Certs: st, Registry: st, Mail: st, Breaches: breachStub{st}, Now: func() time.Time { return *at }})
}

func TestService_CheckLooksOnlyAtTheServicesSwitchedOn(t *testing.T) {
	c := cfg()
	c.MailRecordsOn, c.BreachesOn = false, false
	st := &stubs{certs: []int64{3, 2, 1}, expiry: now.Add(100 * day)}
	at := now
	s := newService(c, st, &memStore{}, &at)
	rs, err := s.Check(context.Background(), "domain:example.com", true)
	if err != nil || len(rs) != 2 || len(st.asked) != 2 {
		t.Fatalf("%v %v %v", rs, err, st.asked)
	}
}

func TestService_TheFirstCertLookIsTheBaselineAndTheSecondCountsNewOnes(t *testing.T) {
	st := &stubs{certs: []int64{1, 2}}
	at := now
	c := cfg()
	c.ExpiryOn, c.MailRecordsOn = false, false
	s := newService(c, st, &memStore{}, &at)
	first, _ := s.Check(context.Background(), "domain:example.com", false)
	st.certs = []int64{3, 2, 1}
	at = now.Add(2 * time.Minute)
	second, _ := s.Check(context.Background(), "domain:example.com", false)
	if first[0].NewCerts != 0 || second[0].NewCerts != 1 {
		t.Errorf("new certs: %d then %d", first[0].NewCerts, second[0].NewCerts)
	}
}

// A service that is down must not erase what was known: the last good facts stay, the attempt is recorded as could-not-look,
// and the watch stays fresh until the window runs out.
func TestService_AFailedLookKeepsTheLastGoodFactsAndSaysWhy(t *testing.T) {
	c := cfg()
	c.ExpiryOn, c.MailRecordsOn = false, false
	st := &stubs{certs: []int64{1, 2}}
	at := now
	s := newService(c, st, &memStore{}, &at)
	_, _ = s.Check(context.Background(), "domain:example.com", false)
	st.certErr = &LookError{Reason: ReasonDown}
	at = now.Add(time.Hour)
	rs, err := s.Check(context.Background(), "domain:example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	r := rs[0]
	if r.Status != models.FootprintCouldNotLook || r.Reason != ReasonDown || len(r.CertIDs) != 2 || !r.LookedAt.Equal(now) || !r.At.Equal(at) {
		t.Errorf("%+v", r)
	}
}

func TestService_CheckNowHasACooldownAndRefusesWhileBusy(t *testing.T) {
	st := &stubs{}
	at := now
	s := newService(cfg(), st, &memStore{}, &at)
	if _, err := s.Check(context.Background(), "domain:example.com", true); err != nil {
		t.Fatal(err)
	}
	var cool *CooldownError
	if _, err := s.Check(context.Background(), "domain:example.com", true); !errors.As(err, &cool) {
		t.Errorf("want a cooldown, got %v", err)
	}
	at = now.Add(2 * time.Minute)
	if _, err := s.Check(context.Background(), "domain:example.com", true); err != nil {
		t.Errorf("after the cooldown: %v", err)
	}
	s.mu.Lock()
	if _, err := s.Check(context.Background(), "domain:example.com", false); !errors.Is(err, ErrBusy) {
		t.Errorf("want busy, got %v", err)
	}
	s.mu.Unlock()
	if _, err := s.Check(context.Background(), "domain:nope.com", false); !errors.Is(err, ErrNoSuchWatch) {
		t.Errorf("want no such watch, got %v", err)
	}
}

func TestService_TheBreachLookGetsTheAddressAndKeepsOnlyCountsAndPlainNames(t *testing.T) {
	st := &stubs{names: []string{"Adobe"}, found: 3}
	at := now
	s := newService(cfg(), st, &memStore{}, &at)
	rs, _ := s.Check(context.Background(), "email:a1", true)
	if len(rs) != 1 || len(rs[0].Breaches) != 3 || rs[0].Breaches[0].Name != "Adobe" || st.asked[0] != "breach:jo@example.com" {
		t.Errorf("%+v %v", rs, st.asked)
	}
}

// The schedule: manual never runs on its own; daily and weekly run when due, once per interval even when the look failed.
func TestService_RunDueLooksOnlyAtWatchesThatAreDue(t *testing.T) {
	c := cfg()
	c.Domains = append(c.Domains, models.FootprintDomain{Name: "manual.org", Frequency: models.FootprintManual})
	st := &stubs{certs: []int64{1}, certErr: &LookError{Reason: ReasonDown}}
	at := now
	store := &memStore{}
	s := newService(c, st, store, &at)
	s.RunDue(context.Background())
	first := len(st.asked)
	if first == 0 {
		t.Fatal("nothing was due on the first pass")
	}
	for _, a := range st.asked {
		if a == "certs:manual.org" {
			t.Error("a manual watch must never run on its own")
		}
	}
	at = now.Add(time.Hour)
	s.RunDue(context.Background())
	if len(st.asked) != first {
		t.Errorf("a failed look must not be retried before its interval: %v", st.asked)
	}
	at = now.Add(25 * time.Hour)
	s.RunDue(context.Background())
	if len(st.asked) == first {
		t.Error("a daily watch is due after a day")
	}
}

func TestService_ResultsOfARemovedWatchAreDropped(t *testing.T) {
	st := &stubs{}
	at := now
	store := &memStore{rs: []models.FootprintResult{{Watch: "domain:gone.org", Check: models.FootprintCheckCerts}}}
	s := newService(cfg(), st, store, &at)
	got, _ := s.Results()
	if len(got) != 0 {
		t.Errorf("%v", got)
	}
}
