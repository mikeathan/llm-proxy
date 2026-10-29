package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodPass = "correct horse battery"

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestService(t *testing.T) (*Service, *clock, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	ck := &clock{t: time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)}
	s := NewService(OpenStore(path), NewHasher(testParams), DefaultConfig())
	s.now = ck.now
	return s, ck, path
}

// setUp runs the whole first-run path and returns a signed-in session id.
func setUp(t *testing.T, s *Service) string {
	t.Helper()
	code, err := s.IssueSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(context.Background(), code, goodPass, "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	id, err := s.Login(context.Background(), goodPass, "10.0.0.1", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestService_FirstRunStateAndSetup(t *testing.T) {
	s, _, _ := newTestService(t)
	if st, _ := s.State(); st.Required || st.SetupOpen {
		t.Errorf("a fresh install is open with no setup code: %+v", st)
	}
	code, err := s.IssueSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := s.State(); st.Required || !st.SetupOpen {
		t.Errorf("after the code is issued: %+v", st)
	}
	if err := s.Setup(context.Background(), code, goodPass, "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.State(); !st.Required || st.SetupOpen {
		t.Errorf("after setup, sign-in is required and the code is spent: %+v", st)
	}
}

func TestService_SetupCodeIsSingleUseExpiresAndIsCancelledByGuessing(t *testing.T) {
	s, ck, _ := newTestService(t)
	ctx := context.Background()

	code, _ := s.IssueSetupCode()
	if err := s.Setup(ctx, "WRONG-CODE", goodPass, "x"); !errors.Is(err, ErrBadSetupCode) {
		t.Errorf("wrong code: %v", err)
	}
	ck.advance(DefaultConfig().SetupCodeTTL + time.Second)
	if err := s.Setup(ctx, code, goodPass, "x"); !errors.Is(err, ErrSetupClosed) {
		t.Errorf("an expired code: %v", err)
	}

	code, _ = s.IssueSetupCode()
	for i := 0; i < DefaultConfig().MaxSetupAttempts; i++ {
		_ = s.Setup(ctx, "WRONG-CODE", goodPass, "x")
	}
	if err := s.Setup(ctx, code, goodPass, "x"); !errors.Is(err, ErrSetupClosed) {
		t.Errorf("a code that was guessed at is cancelled even when the next try is right: %v", err)
	}

	code, _ = s.IssueSetupCode()
	if err := s.Setup(ctx, strings.ToLower(code), goodPass, "x"); err != nil {
		t.Fatalf("a right code in lower case: %v", err)
	}
	if err := s.Setup(ctx, code, "another passphrase", "x"); !errors.Is(err, ErrAlreadySetUp) && !errors.Is(err, ErrSetupClosed) {
		t.Errorf("setting up twice: %v", err)
	}
}

func TestService_SetupRefusesAWeakPassphraseWithoutSpendingTheCode(t *testing.T) {
	s, _, _ := newTestService(t)
	code, _ := s.IssueSetupCode()
	if err := s.Setup(context.Background(), code, "short", "x"); err == nil {
		t.Fatal("a weak passphrase was accepted")
	}
	if err := s.Setup(context.Background(), code, goodPass, "x"); err != nil {
		t.Errorf("the code was spent by a refusal: %v", err)
	}
}

func TestService_IssueSetupCodeRefusesWhenAlreadySetUp(t *testing.T) {
	s, _, _ := newTestService(t)
	setUp(t, s)
	if _, err := s.IssueSetupCode(); !errors.Is(err, ErrAlreadySetUp) {
		t.Errorf("err = %v, want ErrAlreadySetUp (reset first)", err)
	}
}

func TestService_LoginAndSessions(t *testing.T) {
	s, ck, path := newTestService(t)
	id := setUp(t, s)

	if _, ok := s.Session(id); !ok {
		t.Fatal("a fresh session was not accepted")
	}
	if _, ok := s.Session("not-a-session"); ok {
		t.Error("an unknown id was accepted")
	}
	raw, _ := readFile(t, path)
	if strings.Contains(raw, id) || strings.Contains(raw, goodPass) {
		t.Error("the auth file holds a session id or the passphrase in clear")
	}

	ck.advance(DefaultConfig().IdleTimeout - time.Minute)
	if _, ok := s.Session(id); !ok {
		t.Error("a session was dropped before its idle limit")
	}
	ck.advance(DefaultConfig().IdleTimeout + time.Minute)
	if _, ok := s.Session(id); ok {
		t.Error("an idle session was still accepted")
	}
}

func TestService_AbsoluteLifetimeEndsAnActiveSession(t *testing.T) {
	s, ck, _ := newTestService(t)
	id := setUp(t, s)
	for ck.t.Sub(time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)) < DefaultConfig().AbsoluteTimeout+time.Hour {
		ck.advance(DefaultConfig().IdleTimeout / 2) // always active
		if _, ok := s.Session(id); !ok {
			if ck.t.Sub(time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)) < DefaultConfig().AbsoluteTimeout {
				t.Fatal("ended early")
			}
			return
		}
	}
	t.Error("an always-active session outlived the absolute limit")
}

func TestService_WrongPassphraseAndLockout(t *testing.T) {
	s, ck, _ := newTestService(t)
	setUp(t, s)
	ctx := context.Background()
	for i := 0; i < maxFailuresPerAddress; i++ {
		if _, err := s.Login(ctx, "wrong passphrase!", "10.0.0.9", "x"); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	_, err := s.Login(ctx, goodPass, "10.0.0.9", "x") // even the right one waits
	var locked *LockedError
	if !errors.As(err, &locked) || locked.RetryAfter <= 0 {
		t.Fatalf("after %d failures: %v, want a lockout that says how long", maxFailuresPerAddress, err)
	}
	if _, err := s.Login(ctx, goodPass, "10.0.0.7", "x"); err != nil {
		t.Errorf("another address is not locked: %v", err)
	}
	ck.advance(failureWindow + time.Minute)
	if _, err := s.Login(ctx, goodPass, "10.0.0.9", "x"); err != nil {
		t.Errorf("the lockout never ended: %v", err)
	}
}

func TestService_LoginBeforeSetupIsRefused(t *testing.T) {
	s, _, _ := newTestService(t)
	if _, err := s.Login(context.Background(), goodPass, "x", "y"); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("err = %v, want ErrNotSetUp", err)
	}
}

func TestService_LogoutAndRevoke(t *testing.T) {
	s, _, _ := newTestService(t)
	a := setUp(t, s)
	b, _ := s.Login(context.Background(), goodPass, "10.0.0.2", "Safari")

	list := s.Sessions(a)
	if len(list) != 2 {
		t.Fatalf("sessions = %+v", list)
	}
	var current, other SessionView
	for _, v := range list {
		if v.Current {
			current = v
		} else {
			other = v
		}
	}
	if current.Browser != "Firefox" || other.Browser != "Safari" {
		t.Errorf("current %+v other %+v", current, other)
	}
	if err := s.RevokeSession(other.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Session(b); ok {
		t.Error("a revoked session still works")
	}
	if err := s.RevokeSession("nosuchsession"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	s.Logout(a)
	if _, ok := s.Session(a); ok {
		t.Error("a signed-out session still works")
	}
}

func TestService_RevokeAll(t *testing.T) {
	s, _, _ := newTestService(t)
	a := setUp(t, s)
	b, _ := s.Login(context.Background(), goodPass, "10.0.0.2", "Safari")
	if err := s.RevokeAllSessions(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a, b} {
		if _, ok := s.Session(id); ok {
			t.Error("a session survived sign out everywhere")
		}
	}
}

func TestService_ReauthRotatesTheSessionAndOpensAWindow(t *testing.T) {
	s, ck, _ := newTestService(t)
	old := setUp(t, s)
	ctx := context.Background()

	if sess, _ := s.Session(old); !sess.ElevatedUntil.IsZero() {
		t.Fatal("a new session starts un-elevated")
	}
	if _, err := s.Reauth(ctx, old, "wrong passphrase!", "10.0.0.1"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong passphrase: %v", err)
	}
	fresh, err := s.Reauth(ctx, old, goodPass, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if fresh == old {
		t.Error("the session id was not rotated")
	}
	if _, ok := s.Session(old); ok {
		t.Error("the old id still works after rotation")
	}
	sess, ok := s.Session(fresh)
	if !ok || !s.Elevated(sess) {
		t.Fatalf("not elevated: %+v", sess)
	}
	ck.advance(DefaultConfig().ElevationWindow + time.Second)
	sess, _ = s.Session(fresh)
	if s.Elevated(sess) {
		t.Error("the window never closed")
	}
}

func TestService_Tokens(t *testing.T) {
	s, _, path := newTestService(t)
	setUp(t, s)

	plain, tok, err := s.CreateToken("eval scripts", ScopeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, TokenPrefix) || tok.ID == "" {
		t.Fatalf("%q %+v", plain, tok)
	}
	raw, _ := readFile(t, path)
	if strings.Contains(raw, plain) {
		t.Error("the token is stored in clear")
	}
	got, ok := s.Token(plain)
	if !ok || got.Scope != ScopeAdmin || got.Name != "eval scripts" {
		t.Errorf("lookup: %+v %v", got, ok)
	}
	if _, ok := s.Token(plain + "x"); ok {
		t.Error("a near miss was accepted")
	}
	b, _ := json.Marshal(s.Tokens())
	if strings.Contains(string(b), tok.Hash) || strings.Contains(string(b), plain) {
		t.Error("the token list exposes a secret")
	}
	if s.InferenceRequired() {
		t.Error("an admin token must not turn on the inference key requirement")
	}
	_, inf, _ := s.CreateToken("the Mac", ScopeInference)
	if !s.InferenceRequired() {
		t.Error("an inference token turns the requirement on")
	}
	if err := s.RevokeToken(inf.ID); err != nil || s.InferenceRequired() {
		t.Errorf("revoke: %v, required %v", err, s.InferenceRequired())
	}
	if err := s.RevokeToken("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestService_TokenNamesAreChecked(t *testing.T) {
	s, _, _ := newTestService(t)
	setUp(t, s)
	for _, bad := range []string{"", "   ", strings.Repeat("x", maxNameLength+1)} {
		if _, _, err := s.CreateToken(bad, ScopeAdmin); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
	if _, _, err := s.CreateToken("ok", Scope("root")); err == nil {
		t.Error("an unknown scope was accepted")
	}
}

func TestService_ChangePassphrase(t *testing.T) {
	s, _, _ := newTestService(t)
	a := setUp(t, s)
	b, _ := s.Login(context.Background(), goodPass, "10.0.0.2", "Safari")
	ctx := context.Background()

	if err := s.ChangePassphrase(ctx, a, "wrong passphrase!", "a new long passphrase", "x"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong old: %v", err)
	}
	if err := s.ChangePassphrase(ctx, a, goodPass, "short", "x"); err == nil {
		t.Error("a weak new passphrase was accepted")
	}
	if err := s.ChangePassphrase(ctx, a, goodPass, "a new long passphrase", "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Session(b); ok {
		t.Error("another session survived a passphrase change")
	}
	if _, ok := s.Session(a); !ok {
		t.Error("the session that changed it was signed out")
	}
	if _, err := s.Login(ctx, goodPass, "x", "y"); !errors.Is(err, ErrBadCredentials) {
		t.Error("the old passphrase still works")
	}
	if _, err := s.Login(ctx, "a new long passphrase", "x", "y"); err != nil {
		t.Errorf("the new one does not: %v", err)
	}
}

func TestService_ResetClearsEverything(t *testing.T) {
	s, _, _ := newTestService(t)
	id := setUp(t, s)
	_, _, _ = s.CreateToken("t", ScopeAdmin)
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.State(); st.Required {
		t.Error("sign-in is still required after a reset")
	}
	if _, ok := s.Session(id); ok {
		t.Error("a session survived a reset")
	}
	if len(s.Tokens()) != 0 {
		t.Error("tokens survived a reset")
	}
}

func TestService_EventsKeepTheLastFewAndNeverASecret(t *testing.T) {
	s, _, path := newTestService(t)
	setUp(t, s)
	for i := 0; i < maxEvents+5; i++ {
		_, _ = s.Login(context.Background(), "wrong passphrase!", fmt.Sprintf("10.0.1.%d", i), "x") // one address each: no lockout
	}
	ev := s.Events()
	if len(ev) != maxEvents {
		t.Errorf("events = %d, want the last %d", len(ev), maxEvents)
	}
	raw, _ := readFile(t, path)
	if strings.Contains(raw, "wrong passphrase!") || strings.Contains(raw, goodPass) {
		t.Error("an event holds a passphrase")
	}
}

func TestService_AStoreThatCannotBeReadFailsClosed(t *testing.T) {
	s, _, path := newTestService(t)
	setUp(t, s)
	writeFile(t, path, "{broken")
	if _, err := s.State(); err == nil {
		t.Error("State must report an unreadable store, so the caller can refuse instead of treating it as open")
	}
	if _, ok := s.Session("anything"); ok {
		t.Error("a session was accepted from a store that cannot be read")
	}
}

func TestLimiter_ForgetsAddressesThatStoppedFailing(t *testing.T) {
	l := newLimiter()
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		l.fail(fmt.Sprintf("10.0.%d.1", i), now)
	}
	later := now.Add(failureWindow + time.Minute)
	for i := 0; i < 100; i++ {
		l.wait(fmt.Sprintf("10.0.%d.1", i), later)
	}
	if len(l.byAddr) != 0 {
		t.Errorf("%d addresses are still remembered after their failures aged out", len(l.byAddr))
	}
}

func TestConfigFrom_DefaultsAndBounds(t *testing.T) {
	def := DefaultConfig()
	if def.IdleTimeout != 24*time.Hour || def.AbsoluteTimeout != 14*24*time.Hour {
		t.Errorf("defaults = %v / %v, want 24h idle and 14 days absolute", def.IdleTimeout, def.AbsoluteTimeout)
	}
	if got := ConfigFrom(0, 0); got != def {
		t.Errorf("unset settings must give the defaults: %+v", got)
	}
	got := ConfigFrom(12, 30)
	if got.IdleTimeout != 12*time.Hour || got.AbsoluteTimeout != 30*24*time.Hour || got.ElevationWindow != def.ElevationWindow {
		t.Errorf("set values: %+v", got)
	}
	// Nonsense is pulled back into range, never accepted: a session that never ends is the one thing this must not allow.
	if got := ConfigFrom(-5, -1); got != def {
		t.Errorf("negative settings: %+v", got)
	}
	if got := ConfigFrom(100000, 100000); got.IdleTimeout != maxIdleHours*time.Hour || got.AbsoluteTimeout != maxAbsoluteDays*24*time.Hour {
		t.Errorf("huge settings must be capped: %+v", got)
	}
	if got := ConfigFrom(24*30, 7); got.IdleTimeout > got.AbsoluteTimeout {
		t.Errorf("idle %v must not exceed absolute %v", got.IdleTimeout, got.AbsoluteTimeout)
	}
}

func TestService_UsesTheConfigItIsGivenAtEachRequest(t *testing.T) {
	s, ck, _ := newTestService(t)
	id := setUp(t, s)
	short := DefaultConfig()
	short.IdleTimeout = time.Hour
	s.WithConfig(func() Config { return short }) // a settings change applies without a restart
	ck.advance(2 * time.Hour)
	if _, ok := s.Session(id); ok {
		t.Error("a session idle for longer than the new idle limit was accepted")
	}
}

// A stream checks its session without using it: an open tab that only listens must not keep a session alive past its
// idle limit, which is what a use of the session (Session) does.
func TestService_SessionAliveChecksWithoutUsingTheSession(t *testing.T) {
	s, ck, _ := newTestService(t)
	id := setUp(t, s)
	if !s.SessionAlive(id) || s.SessionAlive("not-a-session") || s.SessionAlive("") {
		t.Fatal("a fresh session must be alive, an unknown or empty id must not")
	}
	for range 3 {
		ck.advance(DefaultConfig().IdleTimeout / 2)
		s.SessionAlive(id)
	}
	if s.SessionAlive(id) {
		t.Error("checking kept an idle session alive")
	}
}

func TestService_SessionAliveEndsWithTheSession(t *testing.T) {
	s, _, _ := newTestService(t)
	id := setUp(t, s)
	s.Logout(id)
	if s.SessionAlive(id) {
		t.Error("a signed-out session was still alive")
	}
	other := setUp2Login(t, s)
	if err := s.RevokeAllSessions(); err != nil {
		t.Fatal(err)
	}
	if s.SessionAlive(other) {
		t.Error("a session survived signing out everywhere")
	}
}

func setUp2Login(t *testing.T, s *Service) string {
	t.Helper()
	id, err := s.Login(context.Background(), goodPass, "10.0.0.2", "Chrome")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
