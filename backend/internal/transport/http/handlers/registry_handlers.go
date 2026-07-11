package handlers

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"llm-proxy/internal/core/llm/metadata"
	"llm-proxy/models"
)

// discoverModelFiles scans modelDir for GGUF files not yet registered in current,
// extracting metadata via native GGUF header parsing (fast, header-only).
func discoverModelFiles(ctx context.Context, modelDir string, current []models.ModelConfig) []adminAvailableModel {
	if modelDir == "" {
		return nil
	}

	if info, err := os.Stat(modelDir); err != nil || !info.IsDir() {
		return nil
	}

	seenNames := make(map[string]struct{}, len(current))
	seenPaths := make(map[string]struct{}, len(current))
	for _, m := range current {
		seenNames[m.Name] = struct{}{}
		if m.Path != "" {
			seenPaths[m.Path] = struct{}{}
		}
	}

	scanner := metadata.NewGGUFScanner()
	var found []adminAvailableModel
	_ = filepath.WalkDir(modelDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".gguf" {
			return nil
		}
		fullPath := path
		if _, ok := seenPaths[fullPath]; ok {
			return nil
		}

		// Use native GGUF metadata parsing
		meta, err := scanner.Scan(ctx, fullPath)
		if err != nil {
			// Fallback to filename-based name if parsing fails
			meta.Name = strings.TrimSuffix(d.Name(), ext)
		}

		if _, ok := seenNames[meta.Name]; ok {
			return nil
		}

		var sizeBytes int64
		if targetInfo, err := os.Stat(fullPath); err == nil {
			sizeBytes = targetInfo.Size()
		} else if info, err := d.Info(); err == nil {
			sizeBytes = info.Size()
		}

		found = append(found, adminAvailableModel{
			Name:         meta.Name,
			Filename:     d.Name(),
			ResolvedPath: fullPath,
			SizeBytes:    sizeBytes,
			Metadata:     meta,
		})
		return nil
	})

	sort.Slice(found, func(i, j int) bool {
		return found[i].Name < found[j].Name
	})

	return found
}

func nextAvailablePort(modelsList []models.ModelConfig, activePort int) int {
	if activePort != 0 {
		return activePort
	}
	port := 8081
	for _, m := range modelsList {
		if m.Port >= port {
			port = m.Port + 1
		}
	}
	return port
}
