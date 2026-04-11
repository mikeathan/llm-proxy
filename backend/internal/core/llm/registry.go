package llm

import (
	"embed"
	"encoding/json"
	"fmt"
	"llm-proxy/models"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed definitions/*.json
var manifestFS embed.FS

// ProviderRegistry manages the dynamic loading of provider metadata.
type ProviderRegistry struct {
	mu        sync.RWMutex
	manifests map[string]models.ProviderManifest
}

var (
	registry *ProviderRegistry
	once     sync.Once
)

// GetRegistry returns the singleton provider registry.
func GetRegistry() *ProviderRegistry {
	once.Do(func() {
		registry = &ProviderRegistry{
			manifests: make(map[string]models.ProviderManifest),
		}
		registry.loadEmbedded()
	})
	return registry
}

func (r *ProviderRegistry) loadEmbedded() {
	entries, err := manifestFS.ReadDir("definitions")
	if err != nil {
		fmt.Printf("registry: failed to read embedded definitions: %v\n", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := manifestFS.ReadFile(filepath.Join("definitions", entry.Name()))
		if err != nil {
			fmt.Printf("registry: failed to read %s: %v\n", entry.Name(), err)
			continue
		}

		var manifest models.ProviderManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Printf("registry: failed to parse %s: %v\n", entry.Name(), err)
			continue
		}

		r.Register(manifest)
	}
}

// Register adds or updates a provider manifest.
func (r *ProviderRegistry) Register(m models.ProviderManifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifests[m.ID] = m
}

// Get finds a provider manifest by ID.
func (r *ProviderRegistry) Get(id string) (models.ProviderManifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.manifests[id]
	return m, ok
}

// List returns all registered provider IDs.
func (r *ProviderRegistry) List() []models.ProviderManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]models.ProviderManifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		list = append(list, m)
	}
	return list
}
