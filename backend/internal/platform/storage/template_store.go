package storage

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
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

// shippedManifestFile records the hash of each shipped template as last written, so a later sync can tell an
// untouched copy (safe to refresh) from one the operator edited. It is not a .md file, so it is never listed.
const shippedManifestFile = ".shipped.json"

// extractShipped syncs the embedded default templates into the store.
func (s *TemplateStore) extractShipped() { s.syncTemplates(shipped.FS) }

// syncTemplates seeds missing templates and refreshes the ones still identical to what was last seeded. A file
// the operator edited, or one with no record of what was seeded, is left alone.
func (s *TemplateStore) syncTemplates(src fs.FS) {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		logging.Warn("failed to create templates dir", "dir", s.baseDir, "error", err)
		return
	}
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		logging.Warn("failed to read embedded templates", "error", err)
		return
	}
	record := s.readShippedRecord()
	changed := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, rerr := fs.ReadFile(src, e.Name())
		if rerr != nil {
			continue
		}
		if s.syncOne(e.Name(), data, record) {
			changed = true
		}
	}
	if changed {
		s.writeShippedRecord(record)
	}
}

// syncOne applies the sync rules to one template and reports whether the record changed.
func (s *TemplateStore) syncOne(name string, shippedData []byte, record map[string]string) bool {
	dst := filepath.Join(s.baseDir, name)
	want := hashContent(shippedData)
	onDisk, err := os.ReadFile(dst)
	switch {
	case os.IsNotExist(err):
		return s.writeTemplate(dst, name, shippedData, want, record)
	case err != nil:
		logging.Warn("failed to read template", "name", name, "error", err)
		return false
	case hashContent(onDisk) == want:
		record[name] = want
		return true
	case record[name] == hashContent(onDisk):
		return s.writeTemplate(dst, name, shippedData, want, record)
	default:
		logging.Info("template differs from the shipped version and was left as is", "name", name)
		return false
	}
}

func (s *TemplateStore) writeTemplate(dst, name string, data []byte, hash string, record map[string]string) bool {
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		logging.Warn("failed to write template", "name", name, "error", err)
		return false
	}
	record[name] = hash
	return true
}

func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readShippedRecord returns the manifest; a missing or unreadable one is empty, which only ever means "leave
// the files alone".
func (s *TemplateStore) readShippedRecord() map[string]string {
	record := map[string]string{}
	data, err := os.ReadFile(filepath.Join(s.baseDir, shippedManifestFile))
	if err != nil {
		return record
	}
	if err := json.Unmarshal(data, &record); err != nil || record == nil {
		logging.Warn("unreadable shipped-template record, ignoring it", "error", err)
		return map[string]string{}
	}
	return record
}

func (s *TemplateStore) writeShippedRecord(record map[string]string) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return
	}
	if err := WriteAtomic(filepath.Join(s.baseDir, shippedManifestFile), "shipped-*.json.tmp", data, ClassUserContent); err != nil {
		logging.Warn("failed to record shipped templates", "error", err)
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
