package lockdown

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

type memStore struct {
	rec models.Lockdown
	err error
}

func (m *memStore) Read() (models.Lockdown, error) { return m.rec, m.err }
func (m *memStore) Write(l models.Lockdown) error  { m.rec = l; return nil }

type fakeAuth struct{ kept, tokens string }

func (f *fakeAuth) RevokeOtherSessions(keep string) (int, error) { f.kept = keep; return 2, nil }
func (f *fakeAuth) RevokeAllTokens() (int, error)                { f.tokens = "all"; return 3, nil }

func TestManager_EngageAndReleaseAreKeptAndReported(t *testing.T) {
	store, auth := &memStore{}, &fakeAuth{}
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	m := New(store, auth, func() time.Time { return now })
	if err := m.Load(); err != nil || m.On() {
		t.Fatalf("a missing record is off: %v %v", m.On(), err)
	}
	st, err := m.Engage("session-a", false)
	if err != nil || !st.On || !m.On() || st.SignedOut != 2 || st.TokensRevoked != 0 || auth.kept != "session-a" || auth.tokens != "" {
		t.Fatalf("engage = %+v, %v (auth %+v)", st, err, auth)
	}
	if !store.rec.On || !store.rec.Since.Equal(now) {
		t.Errorf("stored %+v", store.rec)
	}
	if st, _ = m.Engage("session-a", true); st.TokensRevoked != 3 {
		t.Errorf("tokens revoked = %d; engaging again with tokens revokes them", st.TokensRevoked)
	}
	if st, err = m.Release(); err != nil || st.On || m.On() || store.rec.On {
		t.Fatalf("release = %+v, %v", st, err)
	}
}

// A record that cannot be read keeps the program locked: a damaged switch must not open what the operator closed.
func TestManager_UnreadableRecordFailsClosed(t *testing.T) {
	m := New(&memStore{err: errors.New("corrupt")}, &fakeAuth{}, time.Now)
	if err := m.Load(); err == nil || !m.On() {
		t.Fatalf("on = %v, err = %v; want locked and the error", m.On(), err)
	}
}
