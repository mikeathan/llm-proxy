// module.go — Module, the seam a self-contained tool family registers through, so adding a family
// does not mean editing the registry's struct, its constructor and its register* list.
package toolregistry

import (
	"context"
	"io/fs"

	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// Module is a family of tools with its own manifests. Each tool's schema comes from
// `<category>.json` in Manifests, the category being the one the models tool catalog gives the tool.
type Module struct {
	// Manifests holds the family's manifest files at its root. Nil means the built-in manifests (core/tools/manifests).
	Manifests fs.FS
	// Handlers maps a tool name to its handler (build one with Handle).
	Handlers map[string]ToolHandler
	// Available hides the family's tools — from the schema, the manual and argument validation —
	// while it returns false. Nil means always available.
	Available func(ctx context.Context) bool
}

// Handle adapts a typed function to a ToolHandler: the arguments are decoded into T first.
func Handle[T any](fn func(context.Context, T) (any, error)) ToolHandler {
	return func(ctx context.Context, rawArgs string) (any, error) {
		var args T
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return fn(ctx, args)
	}
}

// Use registers a module's tools. A tool whose manifest cannot be loaded is left out and logged, as
// for the built-in families.
func (r *LocalToolRegistry) Use(m Module) *LocalToolRegistry {
	for name, handler := range m.Handlers {
		key := models.CategoryOf(name)
		params, desc, err := m.loadManifest(key, name)
		if err != nil {
			logging.Warn("failed to load tool manifest", "key", key, "tool", name, "error", err)
			continue
		}
		r.addDefinition(name, desc, params)
		r.handlers[name] = handler
		if m.Available != nil {
			r.hideWhenUnavailable(name, m.Available)
		}
	}
	return r
}

func (m Module) loadManifest(key, name string) (map[string]any, string, error) {
	if m.Manifests == nil {
		return tools.LoadManifestAsTool(key, name)
	}
	return tools.LoadManifestFromFS(m.Manifests, key, name)
}
