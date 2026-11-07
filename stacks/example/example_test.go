package example

import (
	"context"
	"encoding/json"
	"testing"

	"llm-proxy-stacks/sdk"
)

func TestManifestIsValid(t *testing.T) {
	if err := New().Manifest().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEveryDeclaredToolIsCalledByNameWithJSON(t *testing.T) {
	s := New()
	if err := s.Start(context.Background(), stacksdk.Host{}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range s.Manifest().Tools {
		out, err := s.CallTool(context.Background(), spec.Name, json.RawMessage(`{"text":"hi"}`))
		if err != nil || string(out) != `{"text":"hi"}` {
			t.Errorf("%s: %s, %v", spec.Name, out, err)
		}
	}
	if _, err := s.CallTool(context.Background(), "nope", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an undeclared tool is not_found, got %v", err)
	}
	if _, err := s.CallTool(context.Background(), ToolEcho, json.RawMessage(`{`)); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("bad arguments are invalid, got %v", err)
	}
}

func TestEveryDeclaredRouteAnswersWithPlainData(t *testing.T) {
	s := New()
	routes := s.Manifest().Routes
	if len(routes) != 2 {
		t.Fatalf("the reference stack declares one read and one step-up write, got %+v", routes)
	}
	got, err := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: "/admin/api/example/echo/hi", Params: map[string]string{"text": "hi"}})
	if err != nil || got.Status != 200 || string(got.Body) != `{"text":"hi"}` {
		t.Errorf("GET = %+v, %v", got, err)
	}
	got, err = s.Handle(context.Background(), stacksdk.Request{Method: "PUT", Path: "/admin/api/example/echo", Body: []byte(`{"text":"yo"}`)})
	if err != nil || got.Status != 200 || string(got.Body) != `{"text":"yo"}` {
		t.Errorf("PUT = %+v, %v", got, err)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content type = %q", got.ContentType)
	}
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: "/nope"}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an undeclared route is not_found, got %v", err)
	}
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: "PUT", Path: "/admin/api/example/echo", Body: []byte(`{`)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("a bad body is invalid, got %v", err)
	}
}

func TestTheWriteRouteNeedsTheStepUp(t *testing.T) {
	for _, r := range New().Manifest().Routes {
		if (r.Method == "PUT") != (r.Level() == stacksdk.AccessStepUp) {
			t.Errorf("%s %s declares access %q", r.Method, r.Path, r.Level())
		}
	}
}
