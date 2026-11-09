package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
)

// helperPathPrefix is where the privileged helper's actions are called: one fixed route per catalogue action, so each can be a step-up
// route (the step-up table matches exact paths). The routes are the core's own, not a stack's: they run an action the operator asked for.
const helperPathPrefix = "/admin/api/helper/"

// HelperRoutes are the routes of the privileged helper's actions, one per catalogue row. Every one is a step-up write; a test holds
// stepUpExact to this list, so an action added to the catalogue without its step-up entry fails the build.
func HelperRoutes() []StepUpRoute {
	var routes []StepUpRoute
	for _, a := range catalog.All() {
		routes = append(routes, StepUpRoute{Method: http.MethodPost, Path: helperPathPrefix + a.ID})
	}
	return routes
}

// HelperResult is what a helper route answers: the action's bytes, which are the program's raw output and are never shown as markup,
// and what the service's parser made of them (Constitution I.11: untrusted data is parsed here, never in the privileged process).
// ParseError says why Parsed is missing; the raw bytes stay so the operator can still read what came back.
type HelperResult struct {
	Kind       string `json:"kind"`
	Data       []byte `json:"data"`
	Truncated  bool   `json:"truncated,omitempty"`
	Parsed     any    `json:"parsed,omitempty"`
	ParseError string `json:"parse_error,omitempty"`
}

// HelperParser turns one action's raw output into typed, bounded fields. The composition root supplies it, so this package does not
// import a program's parser.
type HelperParser func(action string, data []byte) (any, error)

// HelperKeeper is told of every successfully parsed result, with the action and the request body the grant was minted for (already
// checked against the catalogue), so the composition root can keep what a scan found. It is not called for output that was cut or unreadable.
type HelperKeeper func(action string, params []byte, parsed any)

// HelperDeps are what a helper route needs: the one-use grant mint, the privileged port, the operator's switch, and the parser of the
// output (nil answers the raw bytes only), and what to do with a parsed result (nil keeps nothing).
type HelperDeps struct {
	Grants GrantMinter
	Run    stacksdk.Privileged
	On     func(action string) bool
	Parse  HelperParser
	Keep   HelperKeeper
	// Defaults fills in what only the service knows (the interface a capture listens on) before the grant is minted, so the grant is bound to
	// the complete parameters. It returns the body to use; nil leaves the body as it came.
	Defaults HelperDefaults
	// OmitRaw says which actions never send their raw output on to the browser (a capture's output is other devices' traffic, which the
	// service reads and nothing else does). The answer then carries the parsed result or the reason there is none, and no bytes.
	OmitRaw func(action string) bool
}

// HelperDefaults completes one action's request body. An error is answered as a bad request with its text.
type HelperDefaults func(action string, body []byte) ([]byte, error)

const parseCutMessage = "the output was cut at its size limit, so it was not read"

// parsed runs the parser on a whole output; a cut output is never parsed, because a guess from half an XML document would read as a result.
func (d HelperDeps) parsed(action string, res stacksdk.PrivilegedResult) (any, string) {
	switch {
	case d.Parse == nil:
		return nil, ""
	case res.Truncated:
		return nil, parseCutMessage
	}
	v, err := d.Parse(action, res.Data)
	if err != nil {
		return nil, err.Error()
	}
	return v, ""
}

// HelperHandler answers one action's route (Constitution I.11): the action must be turned on in Settings, the request body must be exactly
// that action's parameters (decoded strictly and checked against the catalogue), and only then is a one-use grant minted and the action
// run through the privileged port. The route is a step-up route, so the caller is a signed-in operator who confirmed the passphrase a
// moment ago; a token principal never gets here.
func HelperHandler(action string, deps HelperDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !deps.On(action) {
			WriteJSONError(w, http.StatusForbidden, "this action is off: turn it on in Settings (it needs your passphrase)")
			return
		}
		req, ok := stackRequest(w, r, nil)
		if !ok {
			return
		}
		if deps.Defaults != nil {
			body, err := deps.Defaults(action, req.Body)
			if err != nil {
				WriteJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			req.Body = body
		}
		if !attachGrant(w, &req, action, deps.Grants) {
			return
		}
		res, err := deps.Run.Run(r.Context(), stacksdk.PrivilegedRequest{Grant: req.Grant, Action: action, Params: req.Body})
		if err != nil {
			if r.Context().Err() != nil {
				return // the operator left; nothing to answer
			}
			WriteJSONError(w, statusOf(stacksdk.CodeOf(err)), messageOf(err))
			return
		}
		out := HelperResult{Kind: res.Kind, Data: res.Data, Truncated: res.Truncated}
		if deps.OmitRaw != nil && deps.OmitRaw(action) {
			out.Data = nil
		}
		out.Parsed, out.ParseError = deps.parsed(action, res)
		if out.Parsed != nil && deps.Keep != nil {
			deps.Keep(action, req.Body, out.Parsed)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// HelperAction is one action of the catalogue as the Network page shows it.
type HelperAction struct {
	ID string `json:"id"`
	// On is the operator's switch (sandboxing.helper_actions).
	On bool `json:"on"`
	// Installed is whether the root-owned list the install wrote names the action; null when no helper is installed.
	Installed *bool `json:"installed"`
}

// HelperStatus is what the Network page needs to say plainly where the privileged helper stands and what to do next. It reports; the
// operator's root command installs (the service can never install or widen it).
type HelperStatus struct {
	// Supported is false on a host that has no helper (only Linux with systemd does).
	Supported bool `json:"supported"`
	// Installed is true when the helper's socket exists.
	Installed bool `json:"installed"`
	// Ready is true when a call could start: installed, and the sandbox reports file confinement and Unix-socket denial.
	Ready bool `json:"ready"`
	// NotReady says why in one plain sentence when it is not.
	NotReady string         `json:"not_ready,omitempty"`
	Actions  []HelperAction `json:"actions"`
	// DeclaredRanges are the scan ranges in the Network settings: the service holds every helper call to them, and the helper holds no
	// range of its own, so nothing here needs the command line.
	DeclaredRanges []string `json:"declared_ranges"`
	// InstallCommand and HelpPath are what to run and where to read, shown when the helper is not installed.
	InstallCommand string `json:"install_command"`
	HelpPath       string `json:"help_path"`
}

// HelperInstallCommand is the one command that installs the helper, and HelperHelpPath the page that explains it.
const (
	HelperInstallCommand = "sudo ./setup.sh --install --build --unattended --helper"
	HelperHelpPath       = "docs/service_setup.md#privileged-helper-optional-linux-off-by-default"
)

// HelperSource is where the status comes from; the composition root builds it from the sandbox, the settings and the install's drop-in.
type HelperSource struct {
	Supported func() bool
	Installed func() bool
	// Interlock is nil when the sandbox enforces what a call needs.
	Interlock func() error
	// Gates returns the actions the install's root-owned list names; ok is false when it cannot be read.
	Gates func() (actions []string, ok bool)
	// Declared returns the scan ranges in the Network settings.
	Declared func() []string
	On       func(action string) bool
}

// HelperStatusHandler answers GET /admin/api/helper.
func HelperStatusHandler(src HelperSource) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(src.status())
	}
}

func (src HelperSource) status() HelperStatus {
	st := HelperStatus{
		Supported: src.Supported(), Installed: src.Installed(), Actions: []HelperAction{},
		DeclaredRanges: normaliseRanges(src.Declared()), InstallCommand: HelperInstallCommand, HelpPath: HelperHelpPath,
	}
	allowed, gates := src.Gates()
	for _, a := range catalog.All() {
		act := HelperAction{ID: a.ID, On: src.On(a.ID)}
		if st.Installed && gates {
			in := slices.Contains(allowed, a.ID)
			act.Installed = &in
		}
		st.Actions = append(st.Actions, act)
	}
	switch {
	case !st.Supported:
		st.NotReady = "The privileged helper needs Linux with systemd."
	case !st.Installed:
		st.NotReady = "The privileged helper is not installed."
	default:
		if err := src.Interlock(); err != nil {
			st.NotReady = "The sandbox is not enforcing file confinement and Unix-socket denial, so the service will not call the helper."
		} else {
			st.Ready = true
		}
	}
	return st
}

// normaliseRanges sorts and de-duplicates, and never returns nil, so two lists with the same ranges compare equal and encode as [].
func normaliseRanges(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		return []string{}
	}
	return out
}
