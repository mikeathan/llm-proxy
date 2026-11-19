package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// The routes the stack declares, unchanged from before it existed: the screen and its toasts read them as they always did.
const (
	RouteList   = "/admin/api/reminders"
	RouteEvents = "/admin/api/reminders/events"
	RouteItem   = "/admin/api/reminders/{id}"
	RoutePause  = "/admin/api/reminders/{id}/pause"
	RouteResume = "/admin/api/reminders/{id}/resume"
	RouteSkip   = "/admin/api/reminders/{id}/skip"
)

// recentLimit bounds the finished-reminder list shown in the UI.
const recentLimit = 20

// eventsLimit bounds one page of reminder events: what fired while the UI was closed is capped to the newest.
const eventsLimit = 20

// maxBody bounds a request body, as the core's own JSON routes do.
const maxBody = 4 * 1024 * 1024

type reminderView struct {
	ID       int64  `json:"id"`
	Message  string `json:"message"`
	FireAt   string `json:"fire_at"`
	Cron     string `json:"cron,omitempty"`
	Until    string `json:"until,omitempty"`
	Timezone string `json:"timezone"`
	// Connector is the channel as chosen ("" = automatic, "in-app", or a connector type); Channel is where it will actually go.
	Connector string `json:"connector"`
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
}

// reminderEventView is one outcome the web UI is told about: a reminder was delivered, was late, failed or was missed.
type reminderEventView struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
	Event   string `json:"event"`
	// At is when the outcome was saved, in unix milliseconds: the cursor a client sends back.
	At      int64  `json:"at"`
	Channel string `json:"channel"`
}

// editRequest is the edit form: the whole editable state of a reminder. Give fire_at for a one-off or cron (with optional until) for a recurring one.
type editRequest struct {
	Message   string `json:"message"`
	FireAt    string `json:"fire_at"`
	Cron      string `json:"cron"`
	Until     string `json:"until"`
	Connector string `json:"connector"`
}

// createRequest is the new-reminder form: the edit form plus the timezone the time is in (the browser's); without one the owner's zone is used.
type createRequest struct {
	editRequest
	Timezone string `json:"timezone"`
}

func (a *api) view(r Reminder) reminderView {
	v := reminderView{
		ID: r.ID, Message: r.Message, FireAt: r.FireAt.UTC().Format(time.RFC3339), Cron: r.Cron,
		Timezone: r.Timezone, Connector: r.Connector, Channel: a.svc.Channel(r.Connector),
		Status: string(r.Status), LastError: r.LastError,
	}
	if r.Until != nil {
		v.Until = r.Until.UTC().Format(time.RFC3339)
	}
	return v
}

func (a *api) views(rs []Reminder) []reminderView {
	out := make([]reminderView, 0, len(rs))
	for _, r := range rs {
		out = append(out, a.view(r))
	}
	return out
}

func (e editRequest) toReminder() (Reminder, error) {
	in := Reminder{Message: e.Message, Cron: e.Cron, Connector: e.Connector}
	var err error
	if e.FireAt != "" {
		if in.FireAt, err = ParseInstant("fire_at", e.FireAt); err != nil {
			return in, err
		}
	}
	if e.Until != "" {
		until, err := ParseInstant("until", e.Until)
		if err != nil {
			return in, err
		}
		in.Until = &until
	}
	return in, nil
}

// api answers the routes over the service. zone resolves the zone a client sent, and off says the module is switched off.
type api struct {
	svc  *Service
	zone func(requested string) (*time.Location, error)
	log  func(level stacksdk.Level, msg string, fields ...string)
}

func answer(status int, v any) stacksdk.Response {
	body, err := json.Marshal(v)
	if err != nil {
		return failure(http.StatusInternalServerError, "the answer could not be written")
	}
	return stacksdk.Response{Status: status, ContentType: "application/json", Body: append(body, '\n')}
}

func failure(status int, msg string) stacksdk.Response {
	return answer(status, map[string]string{"error": msg})
}

func noContent() stacksdk.Response { return stacksdk.Response{Status: http.StatusNoContent} }

// decode reads a JSON body the way the core's own routes do: the content type must be exactly application/json and the body is bounded.
func decode(req stacksdk.Request, v any) (stacksdk.Response, bool) {
	if req.ContentType != "application/json" {
		return failure(http.StatusUnsupportedMediaType, "Content-Type must be application/json"), false
	}
	if len(req.Body) > maxBody {
		return failure(http.StatusRequestEntityTooLarge, "the request body is too large"), false
	}
	if err := json.Unmarshal(req.Body, v); err != nil {
		return failure(http.StatusBadRequest, "invalid json: "+err.Error()), false
	}
	return stacksdk.Response{}, true
}

func (a *api) id(req stacksdk.Request) (int64, stacksdk.Response, bool) {
	raw := req.Params["id"]
	if raw == "" {
		return 0, failure(http.StatusBadRequest, "id is required"), false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, failure(http.StatusBadRequest, "invalid id"), false
	}
	return id, stacksdk.Response{}, true
}

// refusal is the status and text for a failure of the service; ok is true when err is nil.
func (a *api) refusal(err error, what string) (stacksdk.Response, bool) {
	switch {
	case err == nil:
		return stacksdk.Response{}, true
	case errors.Is(err, ErrOff):
		return failure(http.StatusConflict, ErrOff.Error()), false
	case errors.Is(err, ErrNotFound):
		return failure(http.StatusNotFound, "reminder not found"), false
	case errors.Is(err, ErrInvalid):
		return failure(http.StatusBadRequest, err.Error()), false
	}
	a.log(stacksdk.LevelWarn, "reminder "+what+" failed", "error", err.Error())
	return failure(http.StatusInternalServerError, "failed to "+what+" reminder"), false
}

func (a *api) list(ctx context.Context) stacksdk.Response {
	open, err := a.svc.List(ctx)
	if err != nil {
		a.log(stacksdk.LevelWarn, "reminders list failed", "error", err.Error())
		return failure(http.StatusInternalServerError, "failed to list reminders")
	}
	recent, err := a.svc.Recent(ctx, recentLimit)
	if err != nil {
		a.log(stacksdk.LevelWarn, "recent reminders failed", "error", err.Error())
		return failure(http.StatusInternalServerError, "failed to list reminders")
	}
	return answer(http.StatusOK, map[string][]reminderView{"open": a.views(open), "recent": a.views(recent)})
}

// events answers GET /admin/api/reminders/events?after=<ms>: the outcomes saved after that time that the web UI should be told about, newest
// first, and the cursor to ask from next (always the server's own newest event time, so a client holding a stale one after a reset or a clock
// change recovers). With no cursor it returns no items, only the newest event time, so a client that has just opened announces what happens from
// then on and not the history.
func (a *api) events(ctx context.Context, query string) stacksdk.Response {
	latest, err := a.svc.LatestEventAt(ctx)
	if err != nil {
		a.log(stacksdk.LevelWarn, "reminder events failed", "error", err.Error())
		return failure(http.StatusInternalServerError, "failed to read reminder events")
	}
	cursor := latest.UnixMilli()
	if latest.IsZero() {
		cursor = 0
	}
	items := []reminderEventView{}
	if raw := queryValue(query, "after"); raw != "" {
		after, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || after < 0 {
			return failure(http.StatusBadRequest, "after must be a time in unix milliseconds")
		}
		found, ferr := a.svc.EventsAfter(ctx, time.UnixMilli(after).UTC(), eventsLimit)
		if ferr != nil {
			a.log(stacksdk.LevelWarn, "reminder events failed", "error", ferr.Error())
			return failure(http.StatusInternalServerError, "failed to read reminder events")
		}
		for _, rem := range found {
			if rem.AlertsWebUI() {
				items = append(items, reminderEventView{ID: rem.ID, Message: rem.Message, Event: rem.Event, At: rem.EventAt.UnixMilli(), Channel: a.svc.Channel(rem.Connector)})
			}
		}
	}
	return answer(http.StatusOK, map[string]any{"cursor": cursor, "items": items})
}

// queryValue is the first value of a key in a raw query string, "" when it is not there or the query does not parse.
func queryValue(raw, key string) string {
	for _, part := range strings.Split(raw, "&") {
		k, v, _ := strings.Cut(part, "=")
		if k == key {
			if dec, err := url.QueryUnescape(v); err == nil {
				return dec
			}
			return ""
		}
	}
	return ""
}

func (a *api) create(ctx context.Context, req stacksdk.Request) stacksdk.Response {
	var body createRequest
	if resp, ok := decode(req, &body); !ok {
		return resp
	}
	in, err := body.toReminder()
	if err != nil {
		return failure(http.StatusBadRequest, err.Error())
	}
	loc, err := a.zone(strings.TrimSpace(body.Timezone))
	if err != nil {
		return failure(http.StatusBadRequest, err.Error())
	}
	in.Timezone, in.CreatedBy = loc.String(), "ui"
	got, err := a.svc.Create(ctx, in)
	if resp, ok := a.refusal(err, "create"); !ok {
		return resp
	}
	return answer(http.StatusCreated, a.view(got))
}

func (a *api) edit(ctx context.Context, req stacksdk.Request) stacksdk.Response {
	id, resp, ok := a.id(req)
	if !ok {
		return resp
	}
	var body editRequest
	if resp, ok := decode(req, &body); !ok {
		return resp
	}
	in, err := body.toReminder()
	if err != nil {
		return failure(http.StatusBadRequest, err.Error())
	}
	got, err := a.svc.Edit(ctx, id, in)
	if resp, ok := a.refusal(err, "edit"); !ok {
		return resp
	}
	return answer(http.StatusOK, a.view(got))
}

func (a *api) act(ctx context.Context, req stacksdk.Request, what string, do func(context.Context, int64) (Reminder, error)) stacksdk.Response {
	id, resp, ok := a.id(req)
	if !ok {
		return resp
	}
	got, err := do(ctx, id)
	if resp, ok := a.refusal(err, what); !ok {
		return resp
	}
	return answer(http.StatusOK, a.view(got))
}

func (a *api) remove(ctx context.Context, req stacksdk.Request) stacksdk.Response {
	id, resp, ok := a.id(req)
	if !ok {
		return resp
	}
	if resp, ok := a.refusal(a.svc.Remove(ctx, id), "delete"); !ok {
		return resp
	}
	return noContent()
}

// handle routes one request to its answer. A path past the list route is /{id} or /{id}/{pause|resume|skip}; the id itself arrives in Params.
func (a *api) handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	switch {
	case req.Method == http.MethodGet && req.Path == RouteList:
		return a.list(ctx), nil
	case req.Method == http.MethodGet && req.Path == RouteEvents:
		return a.events(ctx, req.Query), nil
	case req.Method == http.MethodPost && req.Path == RouteList:
		return a.create(ctx, req), nil
	}
	if rest, found := strings.CutPrefix(req.Path, RouteList+"/"); found {
		if resp, ok := a.item(ctx, req, strings.Split(rest, "/")); ok {
			return resp, nil
		}
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

// item answers the routes that name one reminder by id: parts is the path after the list route, the id alone or the id and an action word.
func (a *api) item(ctx context.Context, req stacksdk.Request, parts []string) (stacksdk.Response, bool) {
	kind := "item"
	switch len(parts) {
	case 1:
	case 2:
		kind = parts[1]
	default:
		return stacksdk.Response{}, false
	}
	do := map[string]func() stacksdk.Response{
		http.MethodPut + " item":    func() stacksdk.Response { return a.edit(ctx, req) },
		http.MethodDelete + " item": func() stacksdk.Response { return a.remove(ctx, req) },
		http.MethodPost + " pause":  func() stacksdk.Response { return a.act(ctx, req, "pause", a.svc.Pause) },
		http.MethodPost + " resume": func() stacksdk.Response { return a.act(ctx, req, "resume", a.svc.Resume) },
		http.MethodPost + " skip":   func() stacksdk.Response { return a.act(ctx, req, "skip", a.svc.Skip) },
	}[req.Method+" "+kind]
	if do == nil {
		return stacksdk.Response{}, false
	}
	return do(), true
}
