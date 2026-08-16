package storage

import (
	"bufio"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"

	shipped "llm-proxy/data/templates"
)

// TemplateStore manages the library of task templates.
type TemplateStore struct {
	baseDir string
}

func NewTemplateStore(dir string) *TemplateStore {
	s := &TemplateStore{baseDir: dir}
	s.extractShipped()
	return s
}

// extractShipped copies embedded default templates that are missing on disk. It
// never overwrites an existing file, so user-edited templates survive upgrades.
func (s *TemplateStore) extractShipped() {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		logging.Warn("failed to create templates dir", "dir", s.baseDir, "error", err)
		return
	}
	entries, err := shipped.FS.ReadDir(".")
	if err != nil {
		logging.Warn("failed to read embedded templates", "error", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dst := filepath.Join(s.baseDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, rerr := shipped.FS.ReadFile(e.Name())
		if rerr != nil {
			continue
		}
		if werr := os.WriteFile(dst, data, 0o600); werr != nil {
			logging.Warn("failed to extract template", "name", e.Name(), "error", werr)
		}
	}
}

// List returns metadata for all available templates.
func (s *TemplateStore) List() ([]models.TemplateMetadata, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.TemplateMetadata{}, nil
		}
		return nil, err
	}

	var list []models.TemplateMetadata
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			meta, err := s.parseMetadata(filepath.Join(s.baseDir, entry.Name()))
			if err == nil {
				list = append(list, meta)
			}
		}
	}
	return list, nil
}

// Get returns the full template content.
func (s *TemplateStore) Get(id string) (models.Template, error) {
	// For simplicity, we assume the filename is id + .md or we find it in the list
	entries, _ := os.ReadDir(s.baseDir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			path := filepath.Join(s.baseDir, entry.Name())
			meta, err := s.parseMetadata(path)
			if err == nil && meta.ID == id {
				content, err := os.ReadFile(path)
				if err != nil {
					return models.Template{}, err
				}
				return models.Template{
					ID:       meta.ID,
					Name:     meta.Name,
					Category: meta.Category,
					Content:  string(content),
				}, nil
			}
		}
	}
	return models.Template{}, os.ErrNotExist
}

func (s *TemplateStore) parseMetadata(path string) (models.TemplateMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return models.TemplateMetadata{}, err
	}
	defer file.Close()

	meta := models.TemplateMetadata{
		ID: filepath.Base(path), // Fallback ID
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## Task:") {
			meta.Name = strings.TrimSpace(strings.TrimPrefix(line, "## Task:"))
		} else if strings.HasPrefix(line, "**ID:**") {
			meta.ID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "**ID:**")), "`")
		} else if strings.HasPrefix(line, "**Category:**") {
			meta.Category = strings.TrimSpace(strings.TrimPrefix(line, "**Category:**"))
		}

		// Optimization: stop if we have all 3
		if meta.Name != "" && meta.ID != "" && meta.Category != "" {
			break
		}
	}

	return meta, nil
}
