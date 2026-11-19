package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/platform/datausage"
)

func TestStorageHandlers_AnswersTheReportOrAFixedError(t *testing.T) {
	h := NewStorageHandlers(func(context.Context) (datausage.Report, error) {
		return datausage.Report{Rows: []datausage.Row{{ID: "runs", Bytes: 5}}, Total: 5, Level: datausage.LevelOK}, nil
	})
	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/admin/api/system/storage", nil))
	var got datausage.Report
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &got) != nil || got.Total != 5 || got.Rows[0].ID != "runs" {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	h = NewStorageHandlers(func(context.Context) (datausage.Report, error) {
		return datausage.Report{}, errors.New("/secret/path: denied")
	})
	rr = httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/admin/api/system/storage", nil))
	if rr.Code != http.StatusInternalServerError || !json.Valid(rr.Body.Bytes()) || string(rr.Body.Bytes()) == "" || containsPath(rr.Body.String()) {
		t.Errorf("%d %s: a failure says a fixed thing, never the path", rr.Code, rr.Body.String())
	}
}

func containsPath(s string) bool {
	for i := 0; i+7 <= len(s); i++ {
		if s[i:i+7] == "/secret" {
			return true
		}
	}
	return false
}
