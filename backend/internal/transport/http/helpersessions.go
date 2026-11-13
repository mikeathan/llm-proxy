package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"time"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/session"
)

// The routes of bounded sessions (Constitution I.17). A person confirms the passphrase once and approves an exact list; the core then serves
// a stack's matching requests without a step-up each time, until the time is up, the uses are spent, the action is switched off or someone
// ends it. Starting one is a step-up route; listing and ending are not, because ending only takes authority away.
const (
	helperSessionsPath = "/admin/api/helper/sessions"
	// HelperSessionsPath is where sessions are listed (GET), started (POST, step-up) and all ended (DELETE).
	HelperSessionsPath = helperSessionsPath
	// HelperSessionPath is the pattern that ends one session (DELETE).
	HelperSessionPath = helperSessionsPath + "/{id}"
)

// HelperSessions is the session store a route reads and changes. grant.Sessions satisfies it.
type HelperSessions interface {
	Begin(session.Spec) (session.View, error)
	Open() (open, ended []session.View)
	End(id string) error
	EndAll(reason string) int
}

// HelperSessionDeps are what the session routes need.
type HelperSessionDeps struct {
	Sessions HelperSessions
	// On says whether an action is switched on in the host settings: a session cannot be started for an action that is off.
	On func(action string) bool
	// Stacks are the stacks that are given a port which serves sessions; a session names one of them.
	Stacks []string
}

// sessionBody is what a person approves. Hours may be fractional; MaxUses is optional.
type sessionBody struct {
	Stack    string  `json:"stack"`
	Label    string  `json:"label"`
	Hours    float64 `json:"hours"`
	MaxUses  int     `json:"max_uses"`
	Requests []struct {
		Action string          `json:"action"`
		Params json.RawMessage `json:"params"`
	} `json:"requests"`
}

// HelperSessionsList answers GET: the open sessions and the last few that ended, with why.
func HelperSessionsList(deps HelperSessionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		open, ended := deps.Sessions.Open()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Open  []session.View `json:"open"`
			Ended []session.View `json:"ended"`
			// Limits are the hard limits, so the page can say what may be asked.
			Limits sessionLimits `json:"limits"`
		}{Open: nonNil(open), Ended: nonNil(ended), Limits: sessionLimits{
			MaxHours: session.MaxTTL.Hours(), MaxUses: session.MaxUses, MaxRequests: session.MaxRequests, MaxOpen: session.DefaultMaxOpen,
		}})
	}
}

type sessionLimits struct {
	MaxHours    float64 `json:"max_hours"`
	MaxUses     int     `json:"max_uses"`
	MaxRequests int     `json:"max_requests"`
	MaxOpen     int     `json:"max_open"`
}

func nonNil(v []session.View) []session.View {
	if v == nil {
		return []session.View{}
	}
	return v
}

// HelperSessionsBegin answers POST (a step-up route): the body is decoded strictly, every request must be an action the catalogue marks standing
// that is switched on, with parameters the catalogue accepts, and the time and uses are held to the hard limits. It answers the session as
// listed; there is no token to answer, because the session is the core's.
func HelperSessionsBegin(deps HelperSessionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body sessionBody
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			WriteJSONError(w, http.StatusBadRequest, "the session request is not valid")
			return
		}
		spec, status, msg := deps.spec(body)
		if msg != "" {
			WriteJSONError(w, status, msg)
			return
		}
		view, err := deps.Sessions.Begin(spec)
		switch {
		case errors.Is(err, session.ErrBad):
			WriteJSONError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, session.ErrTooMany):
			WriteJSONError(w, http.StatusConflict, "as many sessions are open as may be: end one first")
		case err != nil:
			WriteJSONError(w, http.StatusInternalServerError, "the session could not be started")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(view)
		}
	}
}

// spec turns what a person approved into a grant.SessionSpec, or the status and sentence that refuse it.
func (d HelperSessionDeps) spec(b sessionBody) (session.Spec, int, string) {
	if !slices.Contains(d.Stacks, b.Stack) {
		return session.Spec{}, http.StatusBadRequest, "that stack does not take sessions"
	}
	if math.IsNaN(b.Hours) || b.Hours <= 0 || b.Hours > session.MaxTTL.Hours() {
		return session.Spec{}, http.StatusBadRequest, "the time must be more than 0 and at most 12 hours"
	}
	spec := session.Spec{Stack: b.Stack, Label: b.Label, TTL: time.Duration(b.Hours * float64(time.Hour))}
	perHour := 0
	for _, rq := range b.Requests {
		req, hourly, status, msg := d.request(rq.Action, rq.Params)
		if msg != "" {
			return spec, status, msg
		}
		spec.Requests = append(spec.Requests, req)
		perHour = max(perHour, hourly)
	}
	spec.MaxUses = b.MaxUses
	if spec.MaxUses == 0 { // the most the actions' own hourly caps allow in the time, never past the hard limit
		spec.MaxUses = min(session.MaxUses, int(math.Ceil(b.Hours))*perHour)
	}
	return spec, 0, ""
}

// request checks one approved entry: an action the catalogue marks standing, switched on, with parameters the catalogue accepts. hourly is the
// action's own cap an hour.
func (d HelperSessionDeps) request(action string, params json.RawMessage) (req session.Request, hourly, status int, msg string) {
	a, err := catalog.Lookup(action)
	switch {
	case err != nil:
		return req, 0, http.StatusBadRequest, "a request names an action that does not exist"
	case !a.Standing:
		return req, 0, http.StatusForbidden, "that action needs your passphrase each time and cannot run under a session"
	case d.On == nil || !d.On(a.ID):
		return req, 0, http.StatusForbidden, "that action is off: turn it on in Settings first (it needs your passphrase)"
	}
	prepared, err := a.Prepare(bytes.TrimSpace(params))
	if err != nil {
		status = http.StatusBadRequest
		if errors.Is(err, catalog.ErrScope) {
			status = http.StatusForbidden
		}
		return req, 0, status, privilegedRefusal(err)
	}
	return session.Request{Action: a.ID, Canonical: prepared.Canonical}, a.Caps.MaxPerHour, 0, ""
}

// HelperSessionEnd answers DELETE of one session. Ending needs no step-up: it only takes authority away.
func HelperSessionEnd(deps HelperSessionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch err := deps.Sessions.End(r.PathValue("id")); {
		case errors.Is(err, session.ErrNoSuch):
			WriteJSONError(w, http.StatusNotFound, "no open session has that id")
		case err != nil:
			WriteJSONError(w, http.StatusInternalServerError, "the session could not be ended")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// HelperSessionsEndAll answers DELETE of the collection: every open session ends.
func HelperSessionsEndAll(deps HelperSessionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		deps.Sessions.EndAll("ended by a person")
		w.WriteHeader(http.StatusNoContent)
	}
}
