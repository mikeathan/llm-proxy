package app

import (
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/core/chat"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/internal/transport/http/handlers"
)

// Every route is classified, and the ones that are not behind the operator's sign-in are a short exact list. Plan:
// docs/PLANS/cross-cutting/admin-api-authentication.md (Phase 0). When sign-in is built, the same table decides which
// routes get wrapped; until then this test is what stops the next route being added outside the admin surface by accident.
const (
	classPublic    = "public"    // no data: the UI shell
	classInference = "inference" // /v1: the model proxy
	classWebhook   = "webhook"   // inbound connector calls, with their own secret
	classAdmin     = "admin"     // everything else under /admin/api
)

// publicRoutes are the only routes that need no credential once sign-in exists. The shell is public on purpose: it holds
// no data, and a protected shell would make a link from Telegram a cross-site navigation without the SameSite=Strict
// cookie. Adding to this list is a security decision.
var publicRoutes = []string{"GET /admin/"}

func classify(route string) string {
	_, path, _ := strings.Cut(route, " ")
	switch {
	case slices.Contains(publicRoutes, route):
		return classPublic
	case strings.HasPrefix(path, "/v1/"):
		return classInference
	case strings.HasPrefix(path, "/api/v1/webhooks/"):
		return classWebhook
	case strings.HasPrefix(path, "/admin/api/"):
		return classAdmin
	}
	return ""
}

// allRoutes builds the router with a zero handler of every kind (method values on them are never called) so that the
// optional groups (memory, reminders, recordings, dispatcher, webhooks) register too.
func allRoutes(t *testing.T) []string {
	t.Helper()
	hs := &HandlerSet{
		Admin: &handlers.AdminHandlers{}, System: &handlers.SystemHandlers{}, Secrets: &handlers.SecretsHandlers{},
		Process: &handlers.ProcessHandlers{}, MCP: &handlers.MCPHandlers{}, Model: &handlers.ModelHandlers{},
		Proxy: &handlers.ProxyHandlers{}, Turns: &chat.Service{}, Assistant: &handlers.AssistantMessageHandler{},
		ActiveRuns: &handlers.ActiveRunsHandler{}, Dispatcher: &handlers.DispatcherHandlers{},
		Recordings: &handlers.RecordingHandlers{}, Memory: &handlers.MemoryHandlers{}, Reminders: &handlers.ReminderHandlers{},
		Profiles: &handlers.ProfileHandlers{}, Helpers: &handlers.HelpersHandlers{}, Netaudit: &handlers.NetauditHandlers{},
		VulnData: &handlers.VulnDataHandlers{}, Commands: &handlers.CommandHandlers{}, Webhook: &handlers.WebhookHandler{},
	}
	router, ok := buildRouter(hs).(*api.Router)
	if !ok {
		t.Fatalf("buildRouter returned %T, want *api.Router", buildRouter(hs))
	}
	return router.Routes()
}

func TestRouteClassification_EveryRouteHasAClass(t *testing.T) {
	routes := allRoutes(t)
	if len(routes) < 100 {
		t.Fatalf("only %d routes registered: the optional groups are missing from this test", len(routes))
	}
	for _, r := range routes {
		if classify(r) == "" {
			t.Errorf("route %q has no class: it is outside /admin/api, /v1 and the webhook path. Put it under /admin/api, or add it to publicRoutes as a deliberate security decision", r)
		}
	}
}

func TestRouteClassification_OnlyTheShellIsPublic(t *testing.T) {
	var public []string
	for _, r := range allRoutes(t) {
		if classify(r) == classPublic {
			public = append(public, r)
		}
	}
	if !slices.Equal(public, publicRoutes) {
		t.Errorf("public routes = %v, want exactly %v", public, publicRoutes)
	}
}

func TestRouteClassification_TheDestructiveRoutesAreAdmin(t *testing.T) {
	routes := allRoutes(t)
	for _, want := range []string{
		"POST /admin/api/system/wipeout", "POST /admin/api/system/factory-reset", "POST /admin/api/system/clear-runtime-data",
		"POST /admin/api/system/restart", "PUT /admin/api/host", "PUT /admin/api/config", "PUT /admin/api/secrets/keys",
		"POST /admin/api/runtime/processes/{pid}/stop", "PUT /admin/api/netaudit/scope",
	} {
		if !slices.Contains(routes, want) {
			t.Errorf("expected route %q is not registered (renamed? update this list)", want)
			continue
		}
		if classify(want) != classAdmin {
			t.Errorf("%q must be an admin route, got %q", want, classify(want))
		}
	}
}
