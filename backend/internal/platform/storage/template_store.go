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
	// Optional header fields: the automation settings a playbook wants (see models.TemplateDefaults).
	templateAgentField    = "**Agent:**"
	templateNetworkField  = "**Network:**"
	templateJournalField  = "**Journal:**"
	templateScheduleField = "**Schedule:**"
	templateSummaryField  = "**Summary:**"
	// The header ends at the first section heading.
	templateSectionPrefix = "###"
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
			ID:          meta.ID,
			Name:        meta.Name,
			Category:    meta.Category,
			Description: meta.Description,
			Content:     string(content),
			Defaults:    meta.Defaults,
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

// parseMetadata reads a playbook's header: everything before its first section heading. Name, ID and Category are
// the required fields; Summary and the Agent, Network, Journal and Schedule lines are optional (see
// models.TemplateDefaults).
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
		if strings.HasPrefix(line, templateSectionPrefix) {
			break
		}
		applyHeaderLine(&meta, line)
	}
	return meta, nil
}

// applyHeaderLine reads one header line into the metadata. Journal is "on" or "off"; any other value is not a
// declaration.
func applyHeaderLine(meta *models.TemplateMetadata, line string) {
	value := func(prefix string) (string, bool) {
		if !strings.HasPrefix(line, prefix) {
			return "", false
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), "`"), true
	}
	switch {
	case strings.HasPrefix(line, templateNamePrefix):
		meta.Name = strings.TrimSpace(strings.TrimPrefix(line, templateNamePrefix))
	case strings.HasPrefix(line, templateIDPrefix):
		meta.ID, _ = value(templateIDPrefix)
	case strings.HasPrefix(line, templateCategoryField):
		meta.Category, _ = value(templateCategoryField)
	case strings.HasPrefix(line, templateSummaryField):
		meta.Description, _ = value(templateSummaryField)
	case strings.HasPrefix(line, templateAgentField):
		meta.Defaults.Profile, _ = value(templateAgentField)
	case strings.HasPrefix(line, templateNetworkField):
		meta.Defaults.NetworkGrant, _ = value(templateNetworkField)
	case strings.HasPrefix(line, templateScheduleField):
		meta.Defaults.Schedule, _ = value(templateScheduleField)
	case strings.HasPrefix(line, templateJournalField):
		if v, _ := value(templateJournalField); v == "on" || v == "off" {
			on := v == "on"
			meta.Defaults.Journal = &on
		}
	}
}
