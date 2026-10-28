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
	templateDepthField    = "**Depth:**"
	templateChecksField   = "**Checks:**"
	templateVulnsField    = "**Known vulnerabilities:**"
	templatePortsField    = "**Ports:**"
	templateTargetsField  = "**Targets:**"
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

// headerFields is every header line a playbook may carry and how it is read. A value that is not valid for its field
// declares nothing (Journal and Checks are "on" or "off"; Depth is "quick" or "thorough").
var headerFields = []struct {
	prefix string
	read   func(meta *models.TemplateMetadata, value string)
}{
	{templateNamePrefix, func(m *models.TemplateMetadata, v string) { m.Name = v }},
	{templateIDPrefix, func(m *models.TemplateMetadata, v string) { m.ID = v }},
	{templateCategoryField, func(m *models.TemplateMetadata, v string) { m.Category = v }},
	{templateSummaryField, func(m *models.TemplateMetadata, v string) { m.Description = v }},
	{templateAgentField, func(m *models.TemplateMetadata, v string) { m.Defaults.Profile = v }},
	{templateNetworkField, func(m *models.TemplateMetadata, v string) { m.Defaults.NetworkGrant = v }},
	{templateScheduleField, func(m *models.TemplateMetadata, v string) { m.Defaults.Schedule = v }},
	{templateJournalField, func(m *models.TemplateMetadata, v string) { m.Defaults.Journal = onOff(v) }},
	{templateChecksField, func(m *models.TemplateMetadata, v string) {
		if on := onOff(v); on != nil {
			m.Defaults.ScanSettings().Checks = on
		}
	}},
	{templateVulnsField, func(m *models.TemplateMetadata, v string) {
		if on := onOff(v); on != nil {
			m.Defaults.ScanSettings().KnownVulns = on
		}
	}},
	{templateDepthField, func(m *models.TemplateMetadata, v string) {
		if v == models.ScanDepthQuick || v == models.ScanDepthThorough {
			m.Defaults.ScanSettings().Depth = v
		}
	}},
	{templatePortsField, func(m *models.TemplateMetadata, v string) { m.Defaults.ScanSettings().Ports = v }},
	{templateTargetsField, func(m *models.TemplateMetadata, v string) {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				s := m.Defaults.ScanSettings()
				s.Targets = append(s.Targets, t)
			}
		}
	}},
}

// onOff reads "on" or "off"; anything else is not a declaration.
func onOff(v string) *bool {
	if v != "on" && v != "off" {
		return nil
	}
	on := v == "on"
	return &on
}

// applyHeaderLine reads one header line into the metadata.
func applyHeaderLine(meta *models.TemplateMetadata, line string) {
	for _, f := range headerFields {
		if strings.HasPrefix(line, f.prefix) {
			f.read(meta, strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, f.prefix)), "`"))
			return
		}
	}
}
