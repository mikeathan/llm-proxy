package api

import (
	"context"
	"net/http"
	"strings"

	"llm-proxy/internal/platform/auth"
	"llm-proxy/internal/platform/logging"
)

// SessionCookie carries the signed-in browser's session id.
const SessionCookie = "llmproxy_session"

const (
	authAPIPrefix = "/admin/api/auth/"
	inferencePath = "/v1/"
)

// Authenticator is what the middleware needs of the sign-in. *auth.Service satisfies it.
type Authenticator interface {
	State() (auth.State, error)
	Session(id string) (auth.Session, bool)
	SessionAlive(id string) bool
	Elevated(auth.Session) bool
	Token(plain string) (auth.Token, bool)
	InferenceRequired() bool
}

// PrincipalKind says how a request proved who it is.
type PrincipalKind int

const (
	PrincipalSession PrincipalKind = iota + 1
	PrincipalToken
)

// Principal is who made an authenticated request.
type Principal struct {
	Kind      PrincipalKind
	SessionID string     // the cookie's value, for a session
	Token     auth.Token // for a script token
}

type principalKey struct{}
type stepUpKey struct{}

// StepUpGranted reports whether the request may do something that needs the passphrase again: the session entered it
// recently, or sign-in is not set up. A token never does. A handler that must decide from the request's own content (a
// settings save is only dangerous when it changes a program to run) asks this and calls RefuseStepUp. Without the
// middleware (a unit test calling a handler directly) there is nothing to ask of, so it is granted.
func StepUpGranted(ctx context.Context) bool {
	v, present := ctx.Value(stepUpKey{}).(bool)
	return !present || v
}

// RefuseStepUp answers a request that needed the passphrase and did not have it: a session is told to confirm it (the UI
// asks and repeats the request), a script token is told it cannot.
func RefuseStepUp(w http.ResponseWriter, r *http.Request) {
	if p, ok := PrincipalFrom(r.Context()); ok && p.Kind == PrincipalToken {
		WriteJSONError(w, http.StatusForbidden, tokenStepUpMessage)
		return
	}
	WriteJSONError(w, http.StatusForbidden, reauthRequired)
}

const (
	reauthRequired     = "reauth_required"
	tokenStepUpMessage = "this action needs the passphrase; do it in the interface"
)

// PrincipalFrom returns who made the request, when it was authenticated (it is absent while sign-in is not set up).
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// StepUpRoute is a write that needs the passphrase again, shortly before.
type StepUpRoute struct{ Method, Path string }

// stepUpExact are the destructive writes, matched exactly.
var stepUpExact = []StepUpRoute{
	{http.MethodPost, "/admin/api/system/wipeout"},
	{http.MethodPost, "/admin/api/system/factory-reset"},
	{http.MethodPost, "/admin/api/system/clear-runtime-data"},
	{http.MethodPut, "/admin/api/host"}, // the sandbox and terminal switches
	{http.MethodPost, "/admin/api/auth/tokens"},
	{http.MethodPut, "/admin/api/auth/session-length"}, // a longer session weakens sign-in
}

// stepUpPrefixes are groups where every write needs it: stored secrets, and adding or changing an MCP server.
var stepUpPrefixes = []string{"/admin/api/secrets/", "/admin/api/mcp"}

// StepUpRoutes lists the exact step-up routes, so a test can check that each is a real route.
func StepUpRoutes() []StepUpRoute { return append([]StepUpRoute(nil), stepUpExact...) }

// StepUpPrefixes lists the groups whose writes need it.
func StepUpPrefixes() []string { return append([]string(nil), stepUpPrefixes...) }

func needsStepUp(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	for _, r := range stepUpExact {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	for _, p := range stepUpPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// publicAPI are the only /admin/api routes that need no credential: they are how someone signs in.
func publicAPI(method, path string) bool {
	switch path {
	case authAPIPrefix + "status":
		return method == http.MethodGet
	case authAPIPrefix + "login", authAPIPrefix + "setup", authAPIPrefix + "logout":
		return method == http.MethodPost
	}
	return false
}

// Authenticate makes the admin API and the model proxy answer only to someone signed in, once there is something to sign
// in with. Until a passphrase is set the admin API stays open (the upgrade state; the UI shows a banner), and /v1 stays
// open until the first inference key is made. The UI shell and the webhooks are not touched: the shell holds no data, and a
// webhook carries its own secret. A store that cannot be read refuses; it never reads as "open".
func Authenticate(next http.Handler, a Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, inferencePath):
			authenticateInference(w, r, next, a)
		case strings.HasPrefix(r.URL.Path, "/admin/api/"):
			authenticateAdmin(w, r, next, a)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func authenticateInference(w http.ResponseWriter, r *http.Request, next http.Handler, a Authenticator) {
	if !a.InferenceRequired() {
		next.ServeHTTP(w, r)
		return
	}
	if _, ok := a.Token(bearerToken(r)); !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		WriteJSONError(w, http.StatusUnauthorized, "a model key is required")
		return
	}
	next.ServeHTTP(w, r)
}

func authenticateAdmin(w http.ResponseWriter, r *http.Request, next http.Handler, a Authenticator) {
	if publicAPI(r.Method, r.URL.Path) {
		next.ServeHTTP(w, r)
		return
	}
	state, err := a.State()
	if err != nil {
		logging.Error("sign-in state could not be read; refusing the request", "error", err)
		WriteJSONError(w, http.StatusServiceUnavailable, "sign-in could not be checked")
		return
	}
	if !state.Required {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), stepUpKey{}, true)))
		return
	}
	p, sess, ok := principalOf(r, a)
	if !ok {
		WriteJSONError(w, http.StatusUnauthorized, "sign in required")
		return
	}
	if needsStepUp(r.Method, r.URL.Path) && !stepUpAllowed(w, p, sess, a) {
		return
	}
	ctx := context.WithValue(r.Context(), principalKey{}, p)
	ctx = context.WithValue(ctx, stepUpKey{}, p.Kind == PrincipalSession && a.Elevated(sess))
	ctx = context.WithValue(ctx, stillAuthorizedKey{}, func() bool { return credentialStillValid(r, p, a) })
	next.ServeHTTP(w, r.WithContext(ctx))
}

type stillAuthorizedKey struct{}

// StillAuthorized says whether the credential a request was let in with still works. A long-lived response (the live
// event stream) asks it at intervals and ends when it is false: a session signed out or revoked, a revoked token, or a
// sign-in store that cannot be read, because the request was checked once, when it began. It is true for a context the
// middleware did not authenticate (the API is open before a passphrase exists).
func StillAuthorized(ctx context.Context) bool {
	if check, ok := ctx.Value(stillAuthorizedKey{}).(func() bool); ok {
		return check()
	}
	return true
}

// credentialStillValid re-checks p without counting the check as a use of the session. Turning sign-in off with
// `llm-proxy auth reset` leaves a stream open: nothing is required any more.
func credentialStillValid(r *http.Request, p Principal, a Authenticator) bool {
	state, err := a.State()
	if err != nil {
		return false
	}
	if !state.Required {
		return true
	}
	if p.Kind == PrincipalSession {
		return a.SessionAlive(p.SessionID)
	}
	tok, ok := a.Token(bearerToken(r))
	return ok && tok.Scope == auth.ScopeAdmin
}

// stepUpAllowed answers a step-up route: a token cannot confirm a passphrase, and a session must have done so recently.
func stepUpAllowed(w http.ResponseWriter, p Principal, sess auth.Session, a Authenticator) bool {
	if p.Kind == PrincipalToken {
		WriteJSONError(w, http.StatusForbidden, tokenStepUpMessage)
		return false
	}
	if !a.Elevated(sess) {
		WriteJSONError(w, http.StatusForbidden, reauthRequired)
		return false
	}
	return true
}

// principalOf finds who the request is: a session cookie first, else an admin token.
func principalOf(r *http.Request, a Authenticator) (Principal, auth.Session, bool) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if sess, ok := a.Session(c.Value); ok {
			return Principal{Kind: PrincipalSession, SessionID: c.Value}, sess, true
		}
	}
	if tok, ok := a.Token(bearerToken(r)); ok && tok.Scope == auth.ScopeAdmin {
		return Principal{Kind: PrincipalToken, Token: tok}, auth.Session{}, true
	}
	return Principal{}, auth.Session{}, false
}

func bearerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
