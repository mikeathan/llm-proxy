package app

import (
	"path/filepath"

	"llm-proxy/internal/platform/auth"
)

// authFileName holds the operator's sign-in (passphrase hash, script tokens, sessions) beside the settings. The file
// stays across a factory reset on purpose: a reset must not be a way to turn sign-in off.
const authFileName = "auth.json"

// AuthFilePath is where the sign-in is kept under a data root. The command-line tool (`llm-proxy auth …`) opens the same
// file, as the server's OS user.
func AuthFilePath(rootDir string) string { return filepath.Join(rootDir, authFileName) }

// wireAuth opens the sign-in. It never fails at start: an unreadable file is reported on the first request, which refuses. A
// lockout and a sign-in from a new address are told through the Network alerts, which must be wired first.
func (s *AppServices) wireAuth(rootDir string) {
	s.auth = auth.NewService(auth.OpenStore(AuthFilePath(rootDir)), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig()).WithConfig(func() auth.Config {
		a := s.AppCtx.GetSystem().Server.Auth
		return auth.ConfigFrom(a.IdleHours, a.AbsoluteDays)
	})
	if s.netaudit != nil && s.netaudit.SignIns != nil {
		s.auth.WithAlerts(func(a auth.SignInAlert) { s.netaudit.SignIns.Tell(string(a)) })
	}
	// Lock down ends the other sessions (and, if asked, the tokens) through the sign-in.
	if l := s.AppCtx.Lockdown(); l != nil {
		l.SetAuth(s.auth)
	}
}
