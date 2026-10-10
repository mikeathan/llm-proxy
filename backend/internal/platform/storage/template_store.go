package storage

import (
	"bufio"
	"fmt"
	"io/fs"
	"llm-proxy/models"
	"os"
	"strings"
)

const (
	templateFileSuffix    = ".md"
	templateNamePrefix    = "## Task:"
	templateIDPrefix      = "**ID:**"
	templateCategoryField = "**Category:**"
)

// TemplateStore serves the task-template library (playbooks). The library is the set of playbooks shipped inside
// the binary, read from src and never copied to disk, so a new binary serves its own playbooks with nothing to
// refresh. A playbook added to a workspace is a separate copy and is not updated when the shipped one changes.
type TemplateStore struct {
	src fs.FS
}

func NewTemplateStore(src fs.FS) *TemplateStore {
	return &TemplateStore{src: src}
}

// List returns metadata for all available templates.
func (s *TemplateStore) List() ([]models.TemplateMetadata, error) {
	names, err := s.templateNames()
	if err != nil {
		return nil, err
	}
	list := []models.TemplateMetadata{}
	for _, name := range names {
		if meta, err := s.parseMetadata(name); err == nil {
			list = append(list, meta)
		}
	}
	return list, nil
}

// Get returns the full template content.
func (s *TemplateStore) Get(id string) (models.Template, error) {
	names, err := s.templateNames()
	if err != nil {
		return models.Template{}, err
	}
	for _, name := range names {
		meta, err := s.parseMetadata(name)
		if err != nil || meta.ID != id {
			continue
		}
		content, err := fs.ReadFile(s.src, name)
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
	return models.Template{}, os.ErrNotExist
}

// templateNames returns the top-level markdown files of the source, in name order.
func (s *TemplateStore) templateNames() ([]string, error) {
	entries, err := fs.ReadDir(s.src, ".")
	if err != nil {
		return nil, fmt.Errorf("read template library: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), templateFileSuffix) {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (s *TemplateStore) parseMetadata(name string) (models.TemplateMetadata, error) {
	file, err := s.src.Open(name)
	if err != nil {
		return models.TemplateMetadata{}, err
	}
	defer file.Close()

	meta := models.TemplateMetadata{
		ID: name, // Fallback ID
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, templateNamePrefix) {
			meta.Name = strings.TrimSpace(strings.TrimPrefix(line, templateNamePrefix))
		} else if strings.HasPrefix(line, templateIDPrefix) {
			meta.ID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, templateIDPrefix)), "`")
		} else if strings.HasPrefix(line, templateCategoryField) {
			meta.Category = strings.TrimSpace(strings.TrimPrefix(line, templateCategoryField))
		}

		// Optimization: stop if we have all 3
		if meta.Name != "" && meta.ID != "" && meta.Category != "" {
			break
		}
	}

	return meta, nil
}
