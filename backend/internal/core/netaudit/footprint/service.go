package footprint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// Why a look was refused before any request was made.
var (
	// ErrBusy means a look is already running; looks run one at a time, so no service is asked twice at once.
	ErrBusy = errors.New("a footprint look is already running")
	// ErrNoSuchWatch means the label is not one of the watched domains or addresses.
	ErrNoSuchWatch = errors.New("that domain or address is not being watched")
)

// CooldownError says a watch was looked at too recently to look again by hand.
type CooldownError struct{ Remaining time.Duration }

func (e *CooldownError) Error() string {
	return fmt.Sprintf("this was just checked; try again in %d seconds", int(e.Remaining.Seconds())+1)
}

const (
	// CheckNowCooldown is how soon a watch can be looked at again by hand, so a button pressed in a loop is not a flood of requests.
	CheckNowCooldown = time.Minute
	lookTimeout      = 45 * time.Second
	scheduleEvery    = 15 * time.Minute
)

// Store keeps the latest results. *persistence.FootprintStore satisfies it.
type Store interface {
	Read() ([]models.FootprintResult, error)
	Write([]models.FootprintResult) error
}

// The four sensors, as the service uses them.
type (
	CertSource interface {
		Certs(ctx context.Context, domain string) ([]int64, error)
	}
	RegistrySource interface {
		Expiry(ctx context.Context, domain string) (time.Time, error)
	}
	MailSource interface {
		Look(ctx context.Context, domain string) (MailRecords, error)
	}
	BreachSource interface {
		Look(ctx context.Context, address string) (names []string, found int, err error)
	}
)

// Deps are what the service needs; the application wires them in one place.
type Deps struct {
	Config   func() models.FootprintConfig
	Store    Store
	Certs    CertSource
	Registry RegistrySource
	Mail     MailSource
	Breaches BreachSource
	Now      func() time.Time
	OnError  func(error)
}

// Service looks at the watched domains and addresses, one watch at a time, and keeps the latest result of each check. The audit's
// report only reads those results; it never asks an outside service.
type Service struct {
	deps Deps
	mu   sync.Mutex // held for the whole of a look
}

func NewService(deps Deps) *Service {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{deps: deps}
}

// Results are the stored results of the watches that still exist.
func (s *Service) Results() ([]models.FootprintResult, error) {
	all, err := s.deps.Store.Read()
	if err != nil {
		return nil, err
	}
	return currentOnly(s.deps.Config(), all), nil
}

// Check looks at one watch now, for every service that is switched on, and returns the watch's results. byHand is the operator's
// button: it is refused within the cooldown of the last look. A busy service is refused, never queued.
func (s *Service) Check(ctx context.Context, label string, byHand bool) ([]models.FootprintResult, error) {
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	cfg := s.deps.Config()
	w, ok := findWatch(cfg, label)
	if !ok {
		return nil, ErrNoSuchWatch
	}
	all, err := s.deps.Store.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read the footprint results: %w", err)
	}
	all = currentOnly(cfg, all)
	now := s.deps.Now()
	if byHand {
		if left := CheckNowCooldown - now.Sub(lastAttempt(all, label)); left > 0 {
			return nil, &CooldownError{Remaining: left}
		}
	}
	mine := s.lookAll(ctx, cfg, w, all, now)
	next := replaceWatch(all, label, mine)
	if err := s.deps.Store.Write(next); err != nil {
		return nil, fmt.Errorf("failed to save the footprint results: %w", err)
	}
	return mine, nil
}

// RunDue looks at every watch whose schedule is due, one after another. It is called from one loop in the application; a watch that
// is only looked at by hand is never due.
func (s *Service) RunDue(ctx context.Context) {
	cfg := s.deps.Config()
	all, err := s.deps.Store.Read()
	if err != nil {
		s.report(err)
		return
	}
	now := s.deps.Now()
	for _, w := range watches(cfg) {
		if ctx.Err() != nil {
			return
		}
		if len(w.checks) == 0 || !due(w.frequency, lastAttempt(all, w.label), now) {
			continue
		}
		if _, err := s.Check(ctx, w.label, false); err != nil && !errors.Is(err, ErrBusy) {
			s.report(err)
		}
	}
}

// Run is the loop: every few minutes it looks at what is due, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(scheduleEvery)
	defer t.Stop()
	for {
		s.RunDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) report(err error) {
	if s.deps.OnError != nil {
		s.deps.OnError(err)
	}
}

// due says whether a watch's schedule has come round since its last attempt. A failed attempt counts, so a service that is down is
// asked once per interval and not in a retry storm.
func due(frequency string, last, now time.Time) bool {
	switch frequency {
	case models.FootprintDaily:
		return now.Sub(last) >= 24*time.Hour
	case models.FootprintWeekly:
		return now.Sub(last) >= 7*24*time.Hour
	}
	return false
}

func findWatch(cfg models.FootprintConfig, label string) (watch, bool) {
	for _, w := range watches(cfg) {
		if w.label == label {
			return w, true
		}
	}
	return watch{}, false
}

// lastAttempt is the latest attempt on any check of a watch; the zero time when it was never looked at.
func lastAttempt(all []models.FootprintResult, label string) time.Time {
	var last time.Time
	for _, r := range all {
		if r.Watch == label && r.At.After(last) {
			last = r.At
		}
	}
	return last
}

func currentOnly(cfg models.FootprintConfig, all []models.FootprintResult) []models.FootprintResult {
	live := map[string]bool{}
	for _, w := range watches(cfg) {
		live[w.label] = true
	}
	var out []models.FootprintResult
	for _, r := range all {
		if live[r.Watch] {
			out = append(out, r)
		}
	}
	return out
}

func replaceWatch(all []models.FootprintResult, label string, mine []models.FootprintResult) []models.FootprintResult {
	var out []models.FootprintResult
	for _, r := range all {
		if r.Watch != label {
			out = append(out, r)
		}
	}
	return append(out, mine...)
}

func previous(all []models.FootprintResult, label, check string) models.FootprintResult {
	for _, r := range all {
		if r.Watch == label && r.Check == check {
			return r
		}
	}
	return models.FootprintResult{}
}

// lookAll makes each switched-on look for one watch and returns its results.
func (s *Service) lookAll(ctx context.Context, cfg models.FootprintConfig, w watch, all []models.FootprintResult, now time.Time) []models.FootprintResult {
	var out []models.FootprintResult
	for _, check := range w.checks {
		prev := previous(all, w.label, check)
		lctx, cancel := context.WithTimeout(ctx, lookTimeout)
		r, err := s.look(lctx, cfg, w.label, check, prev, now)
		cancel()
		if err != nil {
			prev.Watch, prev.Check, prev.Status, prev.Reason, prev.At = w.label, check, models.FootprintCouldNotLook, ReasonOf(err), now
			out = append(out, prev)
			continue
		}
		out = append(out, r)
	}
	return out
}

// look makes one look. A returned error means it could not be made; the caller keeps the earlier facts.
func (s *Service) look(ctx context.Context, cfg models.FootprintConfig, label, check string, prev models.FootprintResult, now time.Time) (models.FootprintResult, error) {
	r := models.FootprintResult{Watch: label, Check: check, Status: models.FootprintLooked, At: now, LookedAt: now}
	switch check {
	case models.FootprintCheckCerts:
		ids, err := s.deps.Certs.Certs(ctx, strings.TrimPrefix(label, models.DomainFindingPrefix))
		if err != nil {
			return r, err
		}
		r.NewCerts, r.CertIDs = NewCerts(prev.CertIDs, ids), ids
	case models.FootprintCheckExpiry:
		at, err := s.deps.Registry.Expiry(ctx, strings.TrimPrefix(label, models.DomainFindingPrefix))
		if err != nil {
			return r, err
		}
		r.ExpiresAt = at
	case models.FootprintCheckMail:
		m, err := s.deps.Mail.Look(ctx, strings.TrimPrefix(label, models.DomainFindingPrefix))
		if err != nil {
			return r, err
		}
		r.SPF, r.DMARC = m.SPF, m.DMARC
	case models.FootprintCheckBreach:
		names, found, err := s.deps.Breaches.Look(ctx, addressOf(cfg, label))
		if err != nil {
			return r, err
		}
		for _, n := range names {
			r.Breaches = append(r.Breaches, models.FootprintBreach{Name: n})
		}
		if found > len(names) { // names that were not plain words still count as breaches
			r.Breaches = append(r.Breaches, make([]models.FootprintBreach, found-len(names))...)
		}
	}
	return r, nil
}

func addressOf(cfg models.FootprintConfig, label string) string {
	for _, e := range cfg.Emails {
		if models.EmailFindingPrefix+e.ID == label {
			return e.Address
		}
	}
	return ""
}
