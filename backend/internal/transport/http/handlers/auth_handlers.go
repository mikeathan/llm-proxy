package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/internal/platform/auth"
	"llm-proxy/internal/platform/logging"
	api "llm-proxy/internal/transport/http"
)

// AuthService is the sign-in as the handlers use it. *auth.Service satisfies it.
type AuthService interface {
	State() (auth.State, error)
	Setup(ctx context.Context, code, passphrase, addr string) error
	Login(ctx context.Context, passphrase, addr, browser string) (string, error)
	Session(id string) (auth.Session, bool)
	Elevated(auth.Session) bool
	Reauth(ctx context.Context, id, passphrase, addr string) (string, error)
	Logout(id string)
	Sessions(currentID string) []auth.SessionView
	RevokeSession(short string) error
	RevokeAllSessions() error
	ChangePassphrase(ctx context.Context, sessionID, old, next, addr string) error
	CreateToken(name string, scope auth.Scope) (string, auth.Token, error)
	Tokens() []auth.TokenView
	RevokeToken(id string) error
	Events() []auth.Event
	Config() auth.Config
}

// AuthHandlers serves the sign-in routes under /admin/api/auth/. Only status, setup, login and logout are open to someone
// who is not signed in (the middleware decides); everything else here assumes a signed-in caller.
type AuthHandlers struct {
	svc AuthService
}

func NewAuthHandlers(svc AuthService) *AuthHandlers { return &AuthHandlers{svc: svc} }

type passphraseBody struct {
	Passphrase string `json:"passphrase"`
}

// Status says whether sign-in is required, whether the first passphrase can be set up, and whether this browser is signed
// in. It says nothing else: it is open to anyone who can reach the server.
func (h *AuthHandlers) Status(w http.ResponseWriter, r *http.Request) {
	state, err := h.svc.State()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "sign-in could not be checked")
		return
	}
	sess, signedIn := h.session(r)
	respondJSON(w, map[string]bool{
		"required": state.Required, "setup_open": state.SetupOpen,
		"authenticated": signedIn, "elevated": signedIn && h.svc.Elevated(sess), "encrypted": isHTTPS(r),
	})
}

// Setup sets the first passphrase from the one-time setup code, and signs the operator in.
func (h *AuthHandlers) Setup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code       string `json:"code"`
		Passphrase string `json:"passphrase"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.svc.Setup(r.Context(), req.Code, req.Passphrase, clientAddr(r)); err != nil {
		h.fail(w, err)
		return
	}
	h.startSession(w, r, req.Passphrase)
}

// Login checks the passphrase and sets the session cookie.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req passphraseBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Passphrase == "" {
		writeJSONError(w, http.StatusBadRequest, "enter your passphrase")
		return
	}
	h.startSession(w, r, req.Passphrase)
}

func (h *AuthHandlers) startSession(w http.ResponseWriter, r *http.Request, passphrase string) {
	id, err := h.svc.Login(r.Context(), passphrase, clientAddr(r), browserName(r.UserAgent()))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.setSessionCookie(w, r, id)
	respondJSON(w, map[string]bool{"authenticated": true})
}

// Logout ends this browser's session and clears the cookie. Signing out when not signed in is fine.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(api.SessionCookie); err == nil {
		h.svc.Logout(c.Value)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// Reauth checks the passphrase again and opens the window for the destructive actions. The cookie changes.
func (h *AuthHandlers) Reauth(w http.ResponseWriter, r *http.Request) {
	var req passphraseBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	c, err := r.Cookie(api.SessionCookie)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "sign in required")
		return
	}
	fresh, err := h.svc.Reauth(r.Context(), c.Value, req.Passphrase, clientAddr(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.setSessionCookie(w, r, fresh)
	respondJSON(w, map[string]bool{"elevated": true})
}

// Sessions lists where the operator is signed in.
func (h *AuthHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
	current := ""
	if c, err := r.Cookie(api.SessionCookie); err == nil {
		current = c.Value
	}
	list := h.svc.Sessions(current)
	if list == nil {
		list = []auth.SessionView{}
	}
	respondJSON(w, list)
}

// RevokeSession ends one other session.
func (h *AuthHandlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeSession(r.URL.Query().Get("id")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevokeAllSessions is "sign out everywhere": it ends this browser's session too.
func (h *AuthHandlers) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeAllSessions(); err != nil {
		h.fail(w, err)
		return
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// ChangePassphrase sets a new passphrase after checking the old one; it ends every other session.
func (h *AuthHandlers) ChangePassphrase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	current := ""
	if c, err := r.Cookie(api.SessionCookie); err == nil {
		current = c.Value
	}
	if err := h.svc.ChangePassphrase(r.Context(), current, req.Old, req.New, clientAddr(r)); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tokens lists the script tokens (never their secret).
func (h *AuthHandlers) Tokens(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.svc.Tokens())
}

// CreateToken makes a script token and returns it once.
func (h *AuthHandlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	plain, tok, err := h.svc.CreateToken(req.Name, auth.Scope(req.Scope))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, map[string]any{"id": tok.ID, "name": tok.Name, "scope": tok.Scope, "token": plain})
}

// RevokeToken deletes a script token.
func (h *AuthHandlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeToken(r.URL.Query().Get("id")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Events lists the last sign-in events for the Security page.
func (h *AuthHandlers) Events(w http.ResponseWriter, r *http.Request) {
	ev := h.svc.Events()
	if ev == nil {
		ev = []auth.Event{}
	}
	respondJSON(w, ev)
}

func (h *AuthHandlers) session(r *http.Request) (auth.Session, bool) {
	c, err := r.Cookie(api.SessionCookie)
	if err != nil {
		return auth.Session{}, false
	}
	return h.svc.Session(c.Value)
}

// fail answers an error from the sign-in in words the person can act on. What is not recognised is logged and answered
// generically, so an internal detail (a file path) never reaches the page.
func (h *AuthHandlers) fail(w http.ResponseWriter, err error) {
	var locked *auth.LockedError
	var weak *auth.WeakError
	switch {
	case errors.As(err, &locked):
		w.Header().Set("Retry-After", strconv.Itoa(int(locked.RetryAfter/time.Second)+1))
		writeJSONError(w, http.StatusTooManyRequests, locked.Error())
	case errors.As(err, &weak):
		writeJSONError(w, http.StatusBadRequest, weak.Error())
	case errors.Is(err, auth.ErrBadCredentials), errors.Is(err, auth.ErrNotSignedIn):
		writeJSONError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, auth.ErrBadSetupCode):
		writeJSONError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, auth.ErrNotSetUp), errors.Is(err, auth.ErrAlreadySetUp), errors.Is(err, auth.ErrSetupClosed):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeJSONError(w, http.StatusServiceUnavailable, "the request was cancelled")
	default:
		logging.Error("sign-in failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "sign-in failed")
	}
}

// setSessionCookie sends the cookie for a session; it expires with the session's absolute limit.
func (h *AuthHandlers) setSessionCookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{
		Name: api.SessionCookie, Value: id, Path: "/admin", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: isHTTPS(r), MaxAge: int(h.svc.Config().AbsoluteTimeout / time.Second),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: api.SessionCookie, Value: "", Path: "/admin", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isHTTPS(r), MaxAge: -1})
}

// isHTTPS reports whether this request arrived over TLS. X-Forwarded-Proto is not believed: anyone can send it, and a
// reverse proxy's setting is a later, explicit step (server.trusted_proxies).
func isHTTPS(r *http.Request) bool { return r.TLS != nil }

// clientAddr is the address the connection came from, without the port.
func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// browserName turns a User-Agent into a short label for the sessions list, such as "Firefox on Mac". It is only a label.
func browserName(ua string) string {
	if ua == "" {
		return "Unknown browser"
	}
	name := firstMatch(ua, [][2]string{{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}, {"curl/", "curl"}})
	if name == "" {
		return "Unknown browser"
	}
	if os := firstMatch(ua, [][2]string{{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Windows", "Windows"}, {"Macintosh", "Mac"}, {"Linux", "Linux"}}); os != "" {
		return name + " on " + os
	}
	return name
}

func firstMatch(s string, table [][2]string) string {
	for _, e := range table {
		if strings.Contains(s, e[0]) {
			return e[1]
		}
	}
	return ""
}
