package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testIndexHTML = `<div id="app"></div>`

func TestServeAdminUI(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: []byte(testIndexHTML)},
		"theme-boot.js":      {Data: []byte("boot()")},
		"assets/index-ab.js": {Data: []byte("app()")},
	}

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"root serves the app", "/admin/", http.StatusOK, testIndexHTML},
		{"existing root file", "/admin/theme-boot.js", http.StatusOK, "boot()"},
		{"existing asset", "/admin/assets/index-ab.js", http.StatusOK, "app()"},
		{"deep link serves the app", "/admin/workspaces/ws/assistant/c1", http.StatusOK, testIndexHTML},
		{"deep file path with an extension serves the app", "/admin/workspaces/ws/files/docs/notes.md", http.StatusOK, testIndexHTML},
		{"missing hashed asset is a 404", "/admin/assets/index-old.js", http.StatusNotFound, ""},
		{"missing root file is a 404", "/admin/favicon.png", http.StatusNotFound, ""},
		{"unknown API path is a 404", "/admin/api/nope", http.StatusNotFound, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serveAdminUI(fsys, rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
			if tc.wantStatus == http.StatusNotFound && strings.Contains(rec.Body.String(), testIndexHTML) {
				t.Fatal("a 404 must not fall back to index.html")
			}
		})
	}
}
