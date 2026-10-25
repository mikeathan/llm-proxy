package toolregistry

import (
	"context"
	"testing"
	"testing/fstest"

	"llm-proxy/internal/core/proxy"
)

const stubManifest = `{"tool_name":"stub","version":"1.0","description":"A stub tool.","parameters":{"type":"object","properties":{"word":{"type":"string"}}}}`

func stubModule(available func(context.Context) bool) Module {
	return Module{
		Manifests: fstest.MapFS{"general.json": {Data: []byte(stubManifest)}},
		Handlers: map[string]ToolHandler{
			"stub_tool": Handle(func(_ context.Context, a struct {
				Word string `json:"word"`
			}) (any, error) {
				return "got " + a.Word, nil
			}),
		},
		Available: available,
	}
}

func listed(t *testing.T, r *LocalToolRegistry) bool {
	t.Helper()
	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Function.Name == "stub_tool" {
			return true
		}
	}
	return false
}

func TestUse_RegistersASelfContainedModule(t *testing.T) {
	r := NewLocalToolRegistry(nil, nil, nil, nil, nil, nil).Use(stubModule(nil))
	if !listed(t, r) {
		t.Fatal("a module tool must be listed without any edit to the registry")
	}
	got, err := r.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "stub_tool", Arguments: `{"word":"hi"}`}})
	if err != nil || got != "got hi" {
		t.Errorf("ExecuteTool = %v, %v", got, err)
	}
}

func TestUse_HidesTheModuleWhileUnavailable(t *testing.T) {
	ready := false
	r := NewLocalToolRegistry(nil, nil, nil, nil, nil, nil).Use(stubModule(func(context.Context) bool { return ready }))
	if listed(t, r) {
		t.Error("an unavailable module must be hidden from the schema")
	}
	ready = true
	if !listed(t, r) {
		t.Error("the module must be listed once available")
	}
}

func TestUse_SkipsAToolWhoseManifestIsMissing(t *testing.T) {
	m := stubModule(nil)
	m.Manifests = fstest.MapFS{}
	r := NewLocalToolRegistry(nil, nil, nil, nil, nil, nil).Use(m)
	if listed(t, r) {
		t.Error("a tool without a manifest must not be listed")
	}
}
