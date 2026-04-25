package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

func TestAdminRegistryHandler(t *testing.T) {
	registry := models.RegistryData{
		Catalogue: []models.ModelRegistryEntry{
			{Name: "alpha", Port: 8081},
		},
	}
	admin := &mocks.MockAdminService{
		GetRegistryFunc: func() models.RegistryData {
			return registry
		},
	}
	handler := newAdminHandlers(&mocks.MockManager{}, admin)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/registry", nil)
	rr := httptest.NewRecorder()

	handler.AdminRegistryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	catalogue := resp["catalogue"].([]any)
	if len(catalogue) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(catalogue))
	}
	entry := catalogue[0].(map[string]any)
	if entry["name"] != "alpha" {
		t.Fatalf("expected alpha, got %v", entry["name"])
	}
	if int(entry["port"].(float64)) != 8081 {
		t.Fatalf("expected 8081, got %v", entry["port"])
	}
}

func TestAdminRegistryPutHandler(t *testing.T) {
	var captured models.RegistryData
	admin := &mocks.MockAdminService{
		UpdateRegistryFunc: func(fn func(*models.RegistryData)) error {
			fn(&captured)
			return nil
		},
		GetRegistryFunc: func() models.RegistryData {
			return captured
		},
	}
	handler := newAdminHandlers(&mocks.MockManager{}, admin)

	body := `{
		"catalogue": [
			{"name": "beta", "port": 9002}
		],
		"providers": {},
		"mcp_servers": []
	}`
	req := httptest.NewRequest(http.MethodPut, "/admin/api/registry", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminRegistryPutHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if len(captured.Catalogue) != 1 {
		t.Fatalf("expected 1 entry in captured registry, got %d", len(captured.Catalogue))
	}
	if captured.Catalogue[0].Name != "beta" {
		t.Fatalf("expected beta, got %s", captured.Catalogue[0].Name)
	}
	if captured.Catalogue[0].Port != 9002 {
		t.Fatalf("expected 9002, got %d", captured.Catalogue[0].Port)
	}
}
