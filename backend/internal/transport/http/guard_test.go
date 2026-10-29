package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func guarded(allowed ...string) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return Guard(ok, GuardConfig{Hostname: "vertex", AllowedHosts: func() []string { return allowed }})
}

func do(h http.Handler, method, target string, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "127.0.0.1:4001"
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGuard_HostMatrix(t *testing.T) {
	h := guarded("llm.example.net")
	cases := map[string]int{
		"127.0.0.1:4001":      http.StatusOK,
		"192.168.50.60:4001":  http.StatusOK,
		"[::1]:4001":          http.StatusOK,
		"localhost:4001":      http.StatusOK,
		"vertex":              http.StatusOK,
		"VERTEX.local:4001":   http.StatusOK,
		"llm.example.net":     http.StatusOK,
		"evil.example":        http.StatusMisdirectedRequest, // DNS rebinding needs a name
		"evil.example:4001":   http.StatusMisdirectedRequest,
		"vertex.evil.example": http.StatusMisdirectedRequest,
	}
	for host, want := range cases {
		rec := do(h, http.MethodGet, "/admin/api/state", func(r *http.Request) { r.Host = host })
		if rec.Code != want {
			t.Errorf("Host %q = %d, want %d", host, rec.Code, want)
		}
	}
}

func TestGuard_HostRuleOnlyAppliesToTheBrowserSurface(t *testing.T) {
	h := guarded()
	// The model proxy and the webhooks are reached by names this server cannot know (a client's DNS name, a tunnel).
	for _, path := range []string{"/v1/models", "/api/v1/webhooks/telegram"} {
		if rec := do(h, http.MethodGet, path, func(r *http.Request) { r.Host = "tunnel.example" }); rec.Code != http.StatusOK {
			t.Errorf("%s with an unknown Host = %d, want 200", path, rec.Code)
		}
	}
	if rec := do(h, http.MethodGet, "/admin/", func(r *http.Request) { r.Host = "evil.example" }); rec.Code != http.StatusMisdirectedRequest {
		t.Errorf("the UI shell with a foreign name = %d, want 421", rec.Code)
	}
}

func TestGuard_FetchMetadataOnEveryMethodOfTheAdminAPI(t *testing.T) {
	h := guarded()
	cases := []struct {
		method, site string
		want         int
	}{
		{http.MethodGet, "", http.StatusOK},                  // a script or curl sends none
		{http.MethodGet, "same-origin", http.StatusOK},       // the UI
		{http.MethodGet, "none", http.StatusOK},              // typed into the address bar
		{http.MethodGet, "cross-site", http.StatusForbidden}, // an <img> on another site: GETs have side effects
		{http.MethodGet, "same-site", http.StatusForbidden},
		{http.MethodPost, "cross-site", http.StatusForbidden}, // the body-less cross-site POST
		{http.MethodPost, "same-origin", http.StatusOK},
		{http.MethodDelete, "same-site", http.StatusForbidden},
	}
	for _, c := range cases {
		rec := do(h, c.method, "/admin/api/providers/test", func(r *http.Request) {
			if c.site != "" {
				r.Header.Set("Sec-Fetch-Site", c.site)
			}
		})
		if rec.Code != c.want {
			t.Errorf("%s with Sec-Fetch-Site %q = %d, want %d", c.method, c.site, rec.Code, c.want)
		}
	}
}

func TestGuard_AForeignOriginWithoutFetchMetadataIsRefusedOnWrites(t *testing.T) {
	h := guarded()
	foreign := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	if rec := do(h, http.MethodPost, "/admin/api/system/wipeout", foreign); rec.Code != http.StatusForbidden {
		t.Errorf("POST with a foreign Origin = %d, want 403", rec.Code)
	}
	same := func(r *http.Request) {
		r.Host = "192.168.50.60:4001"
		r.Header.Set("Origin", "http://192.168.50.60:4001")
	}
	if rec := do(h, http.MethodPost, "/admin/api/system/wipeout", same); rec.Code != http.StatusOK {
		t.Errorf("POST with the page's own Origin = %d, want 200", rec.Code)
	}
}

func TestGuard_OtherSurfacesKeepTheirCrossSiteBehaviour(t *testing.T) {
	h := guarded()
	// Webhooks are posted by Telegram and friends; the model proxy by other programs. Neither is a browser surface.
	for _, path := range []string{"/api/v1/webhooks/telegram", "/v1/chat/completions"} {
		rec := do(h, http.MethodPost, path, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") })
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", path, rec.Code)
		}
	}
}

func TestGuard_SecurityHeaders(t *testing.T) {
	h := guarded()
	api := do(h, http.MethodGet, "/admin/api/state", nil).Header()
	for k, want := range map[string]string{
		"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer",
		"Content-Security-Policy": "frame-ancestors 'none'", "Cache-Control": "no-store",
	} {
		if got := api.Get(k); got != want {
			t.Errorf("/admin/api %s = %q, want %q", k, got, want)
		}
	}
	shell := do(h, http.MethodGet, "/admin/", nil).Header()
	if shell.Get("X-Frame-Options") != "DENY" || shell.Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Errorf("the shell must refuse framing too: %v", shell)
	}
	if shell.Get("Cache-Control") == "no-store" {
		t.Error("the shell's assets may be cached")
	}
	if got := do(h, http.MethodGet, "/v1/models", nil).Header().Get("X-Frame-Options"); got != "" {
		t.Errorf("/v1 is not a page; X-Frame-Options = %q", got)
	}
}
