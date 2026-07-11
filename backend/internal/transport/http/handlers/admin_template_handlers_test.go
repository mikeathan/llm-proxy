package handlers

import (
	"encoding/json"
	"llm-proxy/models"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type mockAdminService struct {
	AdminService
	templates []models.TemplateMetadata
	template  models.Template
	err       error
}

func (m *mockAdminService) ListTemplates() ([]models.TemplateMetadata, error) {
	return m.templates, m.err
}

func (m *mockAdminService) GetTemplate(id string) (models.Template, error) {
	if m.template.ID == id {
		return m.template, nil
	}
	return models.Template{}, os.ErrNotExist
}

func TestAdminTemplateHandlers(t *testing.T) {
	mock := &mockAdminService{
		templates: []models.TemplateMetadata{
			{ID: "t1", Name: "Template 1", Category: "C1"},
		},
		template: models.Template{
			ID:      "t1",
			Name:    "Template 1",
			Content: "Content 1",
		},
	}

	handlers := &AdminHandlers{admin: mock}

	t.Run("ListTemplates", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/api/templates", nil)
		rr := httptest.NewRecorder()

		handlers.ListTemplatesHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp []models.TemplateMetadata
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if len(resp) != 1 || resp[0].ID != "t1" {
			t.Errorf("unexpected response: %v", resp)
		}
	})

	t.Run("GetTemplate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/api/templates/t1", nil)
		rr := httptest.NewRecorder()

		handlers.GetTemplateHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp models.Template
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp.ID != "t1" || resp.Content != "Content 1" {
			t.Errorf("unexpected response: %v", resp)
		}
	})

	t.Run("GetTemplateNotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/api/templates/unknown", nil)
		rr := httptest.NewRecorder()

		handlers.GetTemplateHandler(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}
	})
}
