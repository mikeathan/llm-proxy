package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequirePathParams_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("workspace_id", "ws1")
	r.SetPathValue("session", "s2")

	vals, ok := RequirePathParams(nil, r, "workspace_id", "session")
	if !ok {
		t.Fatal("expected ok=true for present params")
	}
	if vals[0] != "ws1" || vals[1] != "s2" {
		t.Fatalf("unexpected values: %v", vals)
	}
}

func TestRequirePathParams_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("workspace_id", "ws1")
	// "session" left unset

	w := httptest.NewRecorder()
	vals, ok := RequirePathParams(w, r, "workspace_id", "session")
	if ok {
		t.Fatal("expected ok=false for missing param")
	}
	if vals != nil {
		t.Fatalf("expected nil values on failure, got %v", vals)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "session is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestRequirePathParams_SingleMissing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	// "workspace_id" unset

	w := httptest.NewRecorder()
	_, ok := RequirePathParams(w, r, "workspace_id")
	if ok {
		t.Fatal("expected ok=false for single missing param")
	}
	if !strings.Contains(w.Body.String(), "workspace_id is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestRequireQueryParam_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?workspaceID=ws1", nil)

	v, ok := RequireQueryParam(nil, r, "workspaceID")
	if !ok {
		t.Fatal("expected ok=true for present query param")
	}
	if v != "ws1" {
		t.Fatalf("unexpected value: %q", v)
	}
}

func TestRequireQueryParam_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	w := httptest.NewRecorder()
	_, ok := RequireQueryParam(w, r, "workspaceID")
	if ok {
		t.Fatal("expected ok=false for missing query param")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "workspaceID query parameter is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestRequirePathParamsMsg_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("workspace", "ws1")
	r.SetPathValue("session", "s2")

	vals, ok := RequirePathParamsMsg(nil, r, "workspace and session are required", "workspace", "session")
	if !ok {
		t.Fatal("expected ok=true for present params")
	}
	if vals[0] != "ws1" || vals[1] != "s2" {
		t.Fatalf("unexpected values: %v", vals)
	}
}

func TestRequirePathParamsMsg_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("workspace", "ws1")
	// "session" left unset

	w := httptest.NewRecorder()
	_, ok := RequirePathParamsMsg(w, r, "workspace and session are required", "workspace", "session")
	if ok {
		t.Fatal("expected ok=false for missing param")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "workspace and session are required") {
		t.Fatalf("unexpected body: %s", body)
	}
}
