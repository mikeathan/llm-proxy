package api

import (
	"encoding/json"
	"net/http"

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

// HelperResult is what a helper route answers: the action's bytes, which are the program's raw output and are never parsed or shown as markup
// by the core.
type HelperResult struct {
	Kind      string `json:"kind"`
	Data      []byte `json:"data"`
	Truncated bool   `json:"truncated,omitempty"`
}

// HelperHandler answers one action's route (Constitution I.11): the action must be turned on in Settings, the request body must be exactly
// that action's parameters (decoded strictly and checked against the catalogue), and only then is a one-use grant minted and the action
// run through the privileged port. The route is a step-up route, so the caller is a signed-in operator who confirmed the passphrase a
// moment ago; a token principal never gets here.
func HelperHandler(action string, minter GrantMinter, run stacksdk.Privileged, on func(action string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !on(action) {
			WriteJSONError(w, http.StatusForbidden, "this action is off: turn it on in Settings (it needs your passphrase)")
			return
		}
		req, ok := stackRequest(w, r, nil)
		if !ok || !attachGrant(w, &req, action, minter) {
			return
		}
		res, err := run.Run(r.Context(), stacksdk.PrivilegedRequest{Grant: req.Grant, Action: action, Params: req.Body})
		if err != nil {
			if r.Context().Err() != nil {
				return // the operator left; nothing to answer
			}
			WriteJSONError(w, statusOf(stacksdk.CodeOf(err)), messageOf(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HelperResult{Kind: res.Kind, Data: res.Data, Truncated: res.Truncated})
	}
}
