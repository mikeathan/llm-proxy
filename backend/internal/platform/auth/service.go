package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors a caller tells apart. None of them says more than the caller may know: a wrong passphrase is one error, whatever
// was wrong.
var (
	ErrNotSetUp       = errors.New("sign-in has not been set up")
	ErrAlreadySetUp   = errors.New("sign-in is already set up; run `llm-proxy auth reset` on the server to start again")
	ErrSetupClosed    = errors.New("there is no setup code to use; run `llm-proxy auth setup-code` on the server")
	ErrBadSetupCode   = errors.New("that setup code is not right")
	ErrBadCredentials = errors.New("that passphrase is not right")
	ErrNotSignedIn    = errors.New("not signed in")
	ErrNotFound       = errors.New("not found")
)

// LockedError says how long to wait after too many wrong passphrases.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("too many wrong passphrases; try again in %d minutes", int(e.RetryAfter.Minutes())+1)
}

// Config holds the lifetimes. They are named here, not scattered.
type Config struct {
	IdleTimeout      time.Duration // a session unused for this long ends
	AbsoluteTimeout  time.Duration // and no session lasts longer than this, however active
	ElevationWindow  time.Duration // how long a passphrase re-entry unlocks the destructive actions
	SetupCodeTTL     time.Duration
	MaxSetupAttempts int
}

func DefaultConfig() Config {
	return Config{IdleTimeout: DefaultIdleHours * time.Hour, AbsoluteTimeout: DefaultAbsoluteDays * 24 * time.Hour, ElevationWindow: 5 * time.Minute, SetupCodeTTL: 15 * time.Minute, MaxSetupAttempts: 5}
}

// The most a setting may ask for (exported so the settings page can show them). A session must always end, so there is a
// ceiling even for an operator who wants long ones.
const (
	MaxIdleHours    = 24 * 30
	MaxAbsoluteDays = 90
)

// DefaultIdleHours and DefaultAbsoluteDays are the lifetimes when nothing is set (DefaultConfig).
const (
	DefaultIdleHours    = 24
	DefaultAbsoluteDays = 14
)

// ValidateLifetimes checks the two lifetimes a person chose on the settings page: both whole numbers from 1 up to the
// ceiling, and the idle limit no longer than the absolute one (a session cannot be idle longer than it can live).
func ValidateLifetimes(idleHours, absoluteDays int) error {
	switch {
	case idleHours < 1 || idleHours > MaxIdleHours:
		return fmt.Errorf("the idle limit must be from 1 to %d hours", MaxIdleHours)
	case absoluteDays < 1 || absoluteDays > MaxAbsoluteDays:
		return fmt.Errorf("the absolute limit must be from 1 to %d days", MaxAbsoluteDays)
	case idleHours > absoluteDays*24:
		return fmt.Errorf("the idle limit (%d hours) cannot be longer than the absolute limit (%d days)", idleHours, absoluteDays)
	}
	return nil
}

// ConfigFrom builds the lifetimes from the operator's settings (server.auth in settings.yml): the idle limit in hours and
// the absolute limit in days. A zero or negative value means "the default"; a value past the ceiling is cut to it; and
// the idle limit never exceeds the absolute one.
func ConfigFrom(idleHours, absoluteDays int) Config {
	c := DefaultConfig()
	if idleHours > 0 {
		c.IdleTimeout = time.Duration(min(idleHours, MaxIdleHours)) * time.Hour
	}
	if absoluteDays > 0 {
		c.AbsoluteTimeout = time.Duration(min(absoluteDays, MaxAbsoluteDays)) * 24 * time.Hour
	}
	c.IdleTimeout = min(c.IdleTimeout, c.AbsoluteTimeout)
	return c
}

const (
	maxEvents        = 20
	maxNameLength    = 60
	maxSessions      = 50
	maxTokens        = 50
	maxBrowserLength = 80
	// touchEvery limits how often a use of a session or token is written to disk.
	touchEvery = time.Minute
)

// State is what the UI needs before anyone has signed in.
type State struct {
	// Required: a passphrase exists, so every admin request needs a credential.
	Required bool
	// SetupOpen: a setup code has been made and has not expired, so the first passphrase can be set from the UI.
	SetupOpen bool
}

// Service is the operator's sign-in.
type Service struct {
	store   *Store
	hasher  *Hasher
	cfg     Config
	cfgFn   func() Config // when set, read at every request so a settings change applies at once
	now     func() time.Time
	limiter *limiter
}

func NewService(store *Store, hasher *Hasher, cfg Config) *Service {
	return &Service{store: store, hasher: hasher, cfg: cfg, now: time.Now, limiter: newLimiter()}
}

// WithConfig makes the lifetimes follow a setting that can change while the server runs.
func (s *Service) WithConfig(fn func() Config) *Service {
	s.cfgFn = fn
	return s
}

// Config is the lifetimes in force now.
func (s *Service) Config() Config {
	if s.cfgFn != nil {
		return s.cfgFn()
	}
	return s.cfg
}

// State reports whether sign-in is required. An unreadable store is an error, never "not required": the caller refuses.
func (s *Service) State() (State, error) {
	var st State
	err := s.store.View(func(r Record) {
		st.Required = r.Passphrase != ""
		st.SetupOpen = !st.Required && r.SetupCode != nil && s.now().Before(r.SetupCode.ExpiresAt)
	})
	return st, err
}

// IssueSetupCode makes the one-time code for the first passphrase. It runs on the server, as the server's OS user, and
// is the only way to start: the code is returned to the caller (a terminal), never logged.
func (s *Service) IssueSetupCode() (string, error) {
	code, err := newSetupCode()
	if err != nil {
		return "", err
	}
	err = s.store.Update(func(r *Record) error {
		if r.Passphrase != "" {
			return ErrAlreadySetUp
		}
		r.SetupCode = &SetupCode{Hash: hashSecret(code), ExpiresAt: s.now().Add(s.Config().SetupCodeTTL)}
		s.record(r, "setup_code_issued", "", "")
		return nil
	})
	return code, err
}

// Setup sets the first passphrase, from the setup code. A refused passphrase does not spend the code; a few wrong codes
// cancel it, so it cannot be guessed.
func (s *Service) Setup(ctx context.Context, code, passphrase, addr string) error {
	var closed error
	var hash string
	err := s.store.Update(func(r *Record) error {
		if r.Passphrase != "" {
			return ErrAlreadySetUp
		}
		if r.SetupCode == nil || !s.now().Before(r.SetupCode.ExpiresAt) {
			r.SetupCode = nil
			closed = ErrSetupClosed
			return nil
		}
		given := hashSecret(normaliseSetupCode(code))
		if subtle.ConstantTimeCompare([]byte(given), []byte(r.SetupCode.Hash)) != 1 {
			r.SetupCode.Attempts++
			s.record(r, "setup_code_wrong", addr, "")
			if r.SetupCode.Attempts >= s.Config().MaxSetupAttempts {
				r.SetupCode = nil
			}
			closed = ErrBadSetupCode
			return nil // keep the attempt count even though the request failed
		}
		if err := CheckStrength(passphrase); err != nil {
			closed = err
			return nil
		}
		h, err := s.hasher.Hash(passphrase)
		if err != nil {
			return err
		}
		hash = h
		r.Passphrase, r.SetupCode = hash, nil
		s.record(r, "setup", addr, "")
		return nil
	})
	if err != nil {
		return err
	}
	return closed
}

// Login checks the passphrase and starts a session; it returns the session id (what the cookie carries), which is stored
// only as a hash. A right passphrase from a locked-out address waits too: the lock cannot be tested for.
func (s *Service) Login(ctx context.Context, passphrase, addr, browser string) (string, error) {
	hash, err := s.passphraseHash()
	if err != nil {
		return "", err
	}
	if err := s.checkPassphrase(ctx, hash, passphrase, addr, "login_failed"); err != nil {
		return "", err
	}
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	now := s.now()
	err = s.store.Update(func(r *Record) error {
		r.Sessions = append(s.live(r.Sessions), Session{ID: hashSecret(id), CreatedAt: now, LastSeen: now, Address: addr, Browser: trim(browser, maxBrowserLength)})
		if len(r.Sessions) > maxSessions {
			r.Sessions = r.Sessions[len(r.Sessions)-maxSessions:]
		}
		s.record(r, "login", addr, "")
		return nil
	})
	return id, err
}

func (s *Service) passphraseHash() (string, error) {
	var hash string
	if err := s.store.View(func(r Record) { hash = r.Passphrase }); err != nil {
		return "", err
	}
	if hash == "" {
		return "", ErrNotSetUp
	}
	return hash, nil
}

// checkPassphrase applies the lockout, checks the passphrase, and counts a failure.
func (s *Service) checkPassphrase(ctx context.Context, hash, passphrase, addr, failEvent string) error {
	if wait := s.limiter.wait(addr, s.now()); wait > 0 {
		return &LockedError{RetryAfter: wait}
	}
	ok, err := s.hasher.Verify(ctx, hash, passphrase)
	if err != nil {
		return err
	}
	if !ok {
		s.limiter.fail(addr, s.now())
		_ = s.store.Update(func(r *Record) error { s.record(r, failEvent, addr, ""); return nil })
		return ErrBadCredentials
	}
	s.limiter.succeed(addr)
	return nil
}

// Session finds a live session by the id its cookie carries, and notes that it was used. An unreadable store accepts nothing.
func (s *Service) Session(id string) (Session, bool) {
	found, ok := s.lookup(id)
	if !ok {
		return Session{}, false
	}
	hash := found.ID
	if s.now().Sub(found.LastSeen) >= touchEvery {
		found.LastSeen = s.now()
		_ = s.store.Update(func(r *Record) error {
			for i := range r.Sessions {
				if r.Sessions[i].ID == hash {
					r.Sessions[i].LastSeen = found.LastSeen
				}
			}
			return nil
		})
	}
	return found, true
}

// SessionAlive reports whether the session is still valid, without using it. An open stream asks this at intervals: a
// check that counted as use would keep a tab nobody is looking at signed in for ever.
func (s *Service) SessionAlive(id string) bool {
	_, ok := s.lookup(id)
	return ok
}

// lookup finds the live session for a cookie value. It writes nothing.
func (s *Service) lookup(id string) (Session, bool) {
	if id == "" {
		return Session{}, false
	}
	hash := hashSecret(id)
	var found Session
	var ok bool
	if err := s.store.View(func(r Record) {
		for _, sess := range r.Sessions {
			if sess.ID == hash && s.alive(sess) {
				found, ok = sess, true
			}
		}
	}); err != nil || !ok {
		return Session{}, false
	}
	return found, true
}

func (s *Service) alive(sess Session) bool {
	now := s.now()
	return now.Sub(sess.LastSeen) < s.Config().IdleTimeout && now.Sub(sess.CreatedAt) < s.Config().AbsoluteTimeout
}

// live drops the sessions that have ended.
func (s *Service) live(all []Session) []Session {
	kept := make([]Session, 0, len(all)+1)
	for _, sess := range all {
		if s.alive(sess) {
			kept = append(kept, sess)
		}
	}
	return kept
}

// Elevated reports whether the session re-entered the passphrase recently enough for the destructive actions.
func (s *Service) Elevated(sess Session) bool { return s.now().Before(sess.ElevatedUntil) }

// Reauth checks the passphrase again and opens the elevation window. The session id changes (a new id on every step up),
// so an id that leaked earlier is not the one that is elevated.
func (s *Service) Reauth(ctx context.Context, id, passphrase, addr string) (string, error) {
	if _, ok := s.Session(id); !ok {
		return "", ErrNotSignedIn
	}
	hash, err := s.passphraseHash()
	if err != nil {
		return "", err
	}
	if err := s.checkPassphrase(ctx, hash, passphrase, addr, "reauth_failed"); err != nil {
		return "", err
	}
	fresh, err := newSessionID()
	if err != nil {
		return "", err
	}
	old, now := hashSecret(id), s.now()
	err = s.store.Update(func(r *Record) error {
		for i := range r.Sessions {
			if r.Sessions[i].ID == old {
				r.Sessions[i].ID, r.Sessions[i].LastSeen, r.Sessions[i].ElevatedUntil = hashSecret(fresh), now, now.Add(s.Config().ElevationWindow)
				s.record(r, "reauth", addr, "")
				return nil
			}
		}
		return ErrNotSignedIn
	})
	return fresh, err
}

// SessionView is a session as the Security page lists it; it never carries the id the cookie holds.
type SessionView struct {
	ID        string    `json:"id"`
	Address   string    `json:"address,omitempty"`
	Browser   string    `json:"browser,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	Current   bool      `json:"current"`
}

// Sessions lists the live sessions, marking the one whose cookie id is given.
func (s *Service) Sessions(currentID string) []SessionView {
	current := hashSecret(currentID)
	var out []SessionView
	_ = s.store.View(func(r Record) {
		for _, sess := range s.live(r.Sessions) {
			out = append(out, SessionView{ID: shortID(sess.ID), Address: sess.Address, Browser: sess.Browser, CreatedAt: sess.CreatedAt, LastSeen: sess.LastSeen, Current: currentID != "" && sess.ID == current})
		}
	})
	return out
}

// Logout ends one session; an unknown id is not an error.
func (s *Service) Logout(id string) {
	hash := hashSecret(id)
	_ = s.store.Update(func(r *Record) error {
		r.Sessions = removeSessions(r.Sessions, func(x Session) bool { return x.ID == hash })
		s.record(r, "logout", "", "")
		return nil
	})
}

// RevokeSession ends the session the list showed under this id.
func (s *Service) RevokeSession(short string) error {
	return s.store.Update(func(r *Record) error {
		before := len(r.Sessions)
		r.Sessions = removeSessions(r.Sessions, func(x Session) bool { return shortID(x.ID) == short })
		if len(r.Sessions) == before {
			return ErrNotFound
		}
		s.record(r, "session_revoked", "", "")
		return nil
	})
}

// RevokeAllSessions is "sign out everywhere".
func (s *Service) RevokeAllSessions() error {
	return s.store.Update(func(r *Record) error {
		r.Sessions = nil
		s.record(r, "signed_out_everywhere", "", "")
		return nil
	})
}

func removeSessions(all []Session, drop func(Session) bool) []Session {
	kept := all[:0:0]
	for _, sess := range all {
		if !drop(sess) {
			kept = append(kept, sess)
		}
	}
	return kept
}

// ChangePassphrase sets a new passphrase after checking the old one, and ends every other session.
func (s *Service) ChangePassphrase(ctx context.Context, sessionID, old, next, addr string) error {
	hash, err := s.passphraseHash()
	if err != nil {
		return err
	}
	if err := s.checkPassphrase(ctx, hash, old, addr, "passphrase_change_failed"); err != nil {
		return err
	}
	if err := CheckStrength(next); err != nil {
		return err
	}
	newHash, err := s.hasher.Hash(next)
	if err != nil {
		return err
	}
	keep := hashSecret(sessionID)
	return s.store.Update(func(r *Record) error {
		r.Passphrase = newHash
		r.Sessions = removeSessions(r.Sessions, func(x Session) bool { return x.ID != keep })
		s.record(r, "passphrase_changed", addr, "")
		return nil
	})
}

// Reset clears the passphrase, every session and every token, so setup can start again. It runs on the server as the
// server's OS user; there is deliberately no HTTP route to it.
func (s *Service) Reset() error {
	return s.store.Update(func(r *Record) error {
		events := r.Events
		*r = Record{Events: events}
		s.record(r, "reset", "", "")
		return nil
	})
}

// TokenView is a token as the Security page lists it: no hash, no secret.
type TokenView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     Scope     `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// CreateToken makes a script token. The plain token is returned once; only its hash is kept.
func (s *Service) CreateToken(name string, scope Scope) (string, Token, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxNameLength {
		return "", Token{}, fmt.Errorf("a token needs a name of 1 to %d characters", maxNameLength)
	}
	if scope != ScopeAdmin && scope != ScopeInference {
		return "", Token{}, fmt.Errorf("a token's scope is %q or %q", ScopeAdmin, ScopeInference)
	}
	plain, hash, err := newToken()
	if err != nil {
		return "", Token{}, err
	}
	tok := Token{ID: shortID(hash), Name: name, Scope: scope, Hash: hash, CreatedAt: s.now()}
	err = s.store.Update(func(r *Record) error {
		if len(r.Tokens) >= maxTokens {
			return fmt.Errorf("at most %d tokens; revoke one first", maxTokens)
		}
		r.Tokens = append(r.Tokens, tok)
		s.record(r, "token_created", "", name)
		return nil
	})
	return plain, tok, err
}

// Token finds the token a request carries. The comparison is constant time.
func (s *Service) Token(plain string) (Token, bool) {
	if !strings.HasPrefix(plain, TokenPrefix) {
		return Token{}, false
	}
	given := hashSecret(plain)
	var found Token
	var ok bool
	if err := s.store.View(func(r Record) {
		for _, t := range r.Tokens {
			if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(given)) == 1 {
				found, ok = t, true
			}
		}
	}); err != nil || !ok {
		return Token{}, false
	}
	if s.now().Sub(found.LastUsed) >= touchEvery {
		_ = s.store.Update(func(r *Record) error {
			for i := range r.Tokens {
				if r.Tokens[i].ID == found.ID {
					r.Tokens[i].LastUsed = s.now()
				}
			}
			return nil
		})
	}
	return found, true
}

// Tokens lists the tokens without their hashes.
func (s *Service) Tokens() []TokenView {
	out := []TokenView{}
	_ = s.store.View(func(r Record) {
		for _, t := range r.Tokens {
			out = append(out, TokenView{ID: t.ID, Name: t.Name, Scope: t.Scope, CreatedAt: t.CreatedAt, LastUsed: t.LastUsed})
		}
	})
	return out
}

// RevokeToken deletes a token.
func (s *Service) RevokeToken(id string) error {
	return s.store.Update(func(r *Record) error {
		before := len(r.Tokens)
		kept := r.Tokens[:0:0]
		for _, t := range r.Tokens {
			if t.ID != id {
				kept = append(kept, t)
			}
		}
		if len(kept) == before {
			return ErrNotFound
		}
		r.Tokens = kept
		s.record(r, "token_revoked", "", "")
		return nil
	})
}

// InferenceRequired reports whether /v1 needs a key: it does once any inference token exists, and not before, so an
// install that never makes one keeps working as it does today.
func (s *Service) InferenceRequired() bool {
	required := false
	_ = s.store.View(func(r Record) {
		for _, t := range r.Tokens {
			if t.Scope == ScopeInference {
				required = true
			}
		}
	})
	return required
}

// Events returns the last sign-in events, newest last.
func (s *Service) Events() []Event {
	var out []Event
	_ = s.store.View(func(r Record) { out = r.Events })
	return out
}

// record adds an event to the record, keeping the last few. It never takes a secret.
func (s *Service) record(r *Record, kind, addr, detail string) {
	r.Events = append(r.Events, Event{At: s.now(), Kind: kind, Address: addr, Detail: detail})
	if len(r.Events) > maxEvents {
		r.Events = r.Events[len(r.Events)-maxEvents:]
	}
}

func trim(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
