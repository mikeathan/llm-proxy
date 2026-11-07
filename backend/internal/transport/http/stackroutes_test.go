package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

// fakeStack is a stack that declares the routes it is given and answers with fn. The core must not import a real stack, even in tests.
type fakeStack struct {
	stacksdk.Stack // unused methods panic on purpose: the mount may call only Manifest and Handle
	routes         []stacksdk.RouteSpec
	fn             func(stacksdk.Request) (stacksdk.Response, error)
	got            stacksdk.Request
}

func (f *fakeStack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{ID: "fake", Title: "Fake", Routes: f.routes}
}

func (f *fakeStack) Handle(_ context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	f.got = req
	return f.fn(req)
}

func ok200(stacksdk.Request) (stacksdk.Response, error) {
	return stacksdk.Response{Status: 200, ContentType: "application/json", Body: []byte(`{"ok":true}`)}, nil
}

func mounted(t *testing.T, s *fakeStack) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := MountStackRoutes(mux, s); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return mux
}

func TestMountStackRoutes_TheRequestAndTheAnswerPassAsPlainData(t *testing.T) {
	s := &fakeStack{routes: []stacksdk.RouteSpec{{Method: "PUT", Path: "/admin/api/fake/things/{id}"}}, fn: func(stacksdk.Request) (stacksdk.Response, error) {
		return stacksdk.Response{Status: 201, ContentType: "text/plain", Body: []byte("made")}, nil
	}}
	r := httptest.NewRequest("PUT", "/admin/api/fake/things/42?x=1&y=2", strings.NewReader(`{"a":1}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mounted(t, s).ServeHTTP(rec, r)

	want := stacksdk.Request{Method: "PUT", Path: "/admin/api/fake/things/42", Query: "x=1&y=2", Params: map[string]string{"id": "42"}, ContentType: "application/json", Body: []byte(`{"a":1}`)}
	if s.got.Method != want.Method || s.got.Path != want.Path || s.got.Query != want.Query || s.got.ContentType != want.ContentType || string(s.got.Body) != string(want.Body) || s.got.Params["id"] != "42" {
		t.Errorf("request = %+v, want %+v", s.got, want)
	}
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "text/plain" || rec.Body.String() != "made" {
		t.Errorf("answer = %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestMountStackRoutes_AFailureIsACodeAndAMessage(t *testing.T) {
	for code, status := range map[stacksdk.Code]int{
		stacksdk.CodeInvalid: 400, stacksdk.CodeNotFound: 404, stacksdk.CodeDenied: 403,
		stacksdk.CodeUnavailable: 503, stacksdk.CodeTimeout: 504, stacksdk.CodeInternal: 500,
	} {
		s := &fakeStack{routes: []stacksdk.RouteSpec{{Method: "GET", Path: "/admin/api/fake/x"}}, fn: func(stacksdk.Request) (stacksdk.Response, error) {
			return stacksdk.Response{}, stacksdk.Errorf(code, "why")
		}}
		rec := httptest.NewRecorder()
		mounted(t, s).ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/fake/x", nil))
		if rec.Code != status || !strings.Contains(rec.Body.String(), `"error":"why"`) {
			t.Errorf("%s: %d %s, want %d with the message", code, rec.Code, rec.Body.String(), status)
		}
	}
	s := &fakeStack{routes: []stacksdk.RouteSpec{{Method: "GET", Path: "/admin/api/fake/x"}}, fn: func(stacksdk.Request) (stacksdk.Response, error) {
		return stacksdk.Response{}, io.ErrUnexpectedEOF
	}}
	rec := httptest.NewRecorder()
	mounted(t, s).ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/fake/x", nil))
	if rec.Code != 500 {
		t.Errorf("an error that is not an *Error is a 500, got %d", rec.Code)
	}
}

func TestMountStackRoutes_ABodyOverTheLimitIsRefusedBeforeTheStackSeesIt(t *testing.T) {
	s := &fakeStack{routes: []stacksdk.RouteSpec{{Method: "PUT", Path: "/admin/api/fake/x"}}, fn: ok200}
	rec := httptest.NewRecorder()
	mounted(t, s).ServeHTTP(rec, httptest.NewRequest("PUT", "/admin/api/fake/x", strings.NewReader(strings.Repeat("x", maxBodySize+1))))
	if rec.Code != http.StatusRequestEntityTooLarge || s.got.Method != "" {
		t.Errorf("status %d, the stack saw %q", rec.Code, s.got.Method)
	}
}

func TestMountStackRoutes_ARouteTheCoreCannotKeepSafeIsRefused(t *testing.T) {
	bad := map[string]stacksdk.RouteSpec{
		"outside the admin API":     {Method: "GET", Path: "/v1/models"},
		"the sign-in routes":        {Method: "GET", Path: "/admin/api/auth/status"},
		"under the sign-in routes":  {Method: "POST", Path: "/admin/api/auth/anything"},
		"a step-up read":            {Method: "GET", Path: "/admin/api/fake/x", Access: stacksdk.AccessStepUp},
		"a step-up with a wildcard": {Method: "PUT", Path: "/admin/api/fake/{id}", Access: stacksdk.AccessStepUp},
	}
	for name, route := range bad {
		err := MountStackRoutes(http.NewServeMux(), &fakeStack{routes: []stacksdk.RouteSpec{route}, fn: ok200})
		if err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	// A route the core already serves is an error, not a panic.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/fake/x", func(http.ResponseWriter, *http.Request) {})
	if err := MountStackRoutes(mux, &fakeStack{routes: []stacksdk.RouteSpec{{Method: "GET", Path: "/admin/api/fake/x"}}, fn: ok200}); err == nil {
		t.Error("a route that is already registered must be refused")
	}
}

// The step-up a stack declares is enforced by the same middleware as the core's own, so a stack cannot lower it.
func TestMountStackRoutes_AStepUpRouteNeedsThePassphraseAgain(t *testing.T) {
	s := &fakeStack{routes: []stacksdk.RouteSpec{
		{Method: "GET", Path: "/admin/api/fake/read"},
		{Method: "PUT", Path: "/admin/api/fake/write", Access: stacksdk.AccessStepUp},
		{Method: "PUT", Path: "/admin/api/fake/plain"},
	}, fn: ok200}
	extra := StackStepUpRoutes(s.Manifest())
	if len(extra) != 1 || extra[0] != (StepUpRoute{"PUT", "/admin/api/fake/write"}) {
		t.Fatalf("step-up routes = %+v", extra)
	}
	h := AuthenticateWith(mounted(t, s), newFake(), extra)
	for _, c := range []struct {
		name, method, path string
		mod                func(*http.Request)
		want               int
	}{
		{"no sign-in", "PUT", "/admin/api/fake/write", nil, 401},
		{"a plain session", "PUT", "/admin/api/fake/write", cookie("sess"), 403},
		{"an admin token", "PUT", "/admin/api/fake/write", bearer("lp_admin"), 403},
		{"an elevated session", "PUT", "/admin/api/fake/write", cookie("elevated"), 200},
		{"a plain write is admin", "PUT", "/admin/api/fake/plain", cookie("sess"), 200},
		{"a read is admin", "GET", "/admin/api/fake/read", cookie("sess"), 200},
		{"a read needs sign-in", "GET", "/admin/api/fake/read", nil, 401},
	} {
		if got := req(h, c.method, c.path, c.mod); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func BenchmarkStackRoute(b *testing.B) {
	s := &fakeStack{routes: []stacksdk.RouteSpec{{Method: "GET", Path: "/admin/api/fake/x"}}, fn: ok200}
	mux := http.NewServeMux()
	if err := MountStackRoutes(mux, s); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/admin/api/fake/x", nil))
	}
}

// The same request through a hand-written handler that does the same work, so the cost of the mount shows as a difference.
func BenchmarkDirectHandlerForComparison(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/fake/x", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})
	b.ReportAllocs()
	for b.Loop() {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/admin/api/fake/x", nil))
	}
}
