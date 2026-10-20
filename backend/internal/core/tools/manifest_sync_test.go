package tools

import (
	"encoding/json"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolManifestConsistency(t *testing.T) {
	// This test ensures that the Go constants in models/tools.go
	// stay in perfect sync with the tool_name property in our JSON manifests.

	manifestsDir := "manifests"

	tests := []struct {
		filename string
		expected string
	}{
		{"terminal.json", models.ToolTerminalExecute},
		{"communication.json", models.ToolNotifyUser},
		{"search.json", models.ToolInternetSearch},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			path := filepath.Join(manifestsDir, tt.filename)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read manifest %s: %v", tt.filename, err)
			}

			var manifest struct {
				ToolName string `json:"tool_name"`
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.filename, err)
			}

			if manifest.ToolName != tt.expected {
				t.Errorf("Manifest %s mismatch: JSON has %q, Go constant has %q. Please synchronize models/tools.go",
					tt.filename, manifest.ToolName, tt.expected)
			}
		})
	}
}

// TestTerminalManifest_NoCdPersistencePromise guards the terminal tool contract:
// the description must NOT claim that 'cd' persists across calls (the persistent
// shell runs each command from the workspace root). It must instead direct callers
// to chain 'cd subdir && ...' in one command, so plan-and-execute and other callers
// never produce cd-dependent multi-step plans.
func TestTerminalManifest_NoCdPersistencePromise(t *testing.T) {
	_, description, err := LoadManifestAsTool("terminal", models.ToolTerminalExecute)
	if err != nil {
		t.Fatalf("LoadManifestAsTool failed: %v", err)
	}
	if strings.Contains(description, "will persist") {
		t.Errorf("terminal description must not promise cd persistence, got: %q", description)
	}
	if !strings.Contains(description, "workspace root") {
		t.Errorf("terminal description must state commands run in the workspace root, got: %q", description)
	}
	if !strings.Contains(description, "chain `cd subdir") {
		t.Errorf("terminal description must direct chaining `cd subdir && ...`, got: %q", description)
	}
}

// models.ToolsInCategory is checked against the manifests: each tool a category lists must load from that category's manifest.
func TestToolCategoriesMatchManifests(t *testing.T) {
	for _, category := range models.ToolCategories() {
		for _, tool := range models.ToolsInCategory(category) {
			if _, _, err := LoadManifestAsTool(category, tool); err != nil {
				t.Errorf("category %s lists %s, but its manifest cannot load it: %v", category, tool, err)
			}
		}
	}
}

// A model writes a tool call in the order the schema lists the arguments: with read_file's properties sorted
// alphabetically (limit, offset, path) a 16k local model left out offset 9 times in 10; listed as authored (path,
// offset, limit) it used it 10 times in 10. Go marshals map keys sorted, so the order the manifest author wrote is
// carried in an ordered type and must reach the wire for every tool.
func TestLoadManifestAsTool_PropertiesReachTheWireInManifestOrder(t *testing.T) {
	files, _ := filepath.Glob("manifests/*.json")
	checked := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			ToolName   string                     `json:"tool_name"`
			Runtime    map[string]json.RawMessage `json:"runtime"`
			Parameters json.RawMessage            `json:"parameters"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		key := strings.TrimSuffix(filepath.Base(file), ".json")
		tools := map[string]json.RawMessage{}
		if len(m.Parameters) > 0 {
			tools[m.ToolName] = m.Parameters
		}
		for name, raw := range m.Runtime {
			var rt struct {
				Parameters json.RawMessage `json:"parameters"`
			}
			if json.Unmarshal(raw, &rt) == nil && len(rt.Parameters) > 0 { // a runtime entry that is a setting, not a tool, has none
				tools[name] = rt.Parameters
			}
		}
		for name, raw := range tools {
			want := marshaledPropertyOrder(t, raw)
			if len(want) < 2 {
				continue
			}
			params, _, err := LoadManifestAsTool(key, name)
			if err != nil {
				t.Fatalf("%s/%s: %v", key, name, err)
			}
			wire, _ := json.Marshal(params)
			if got := marshaledPropertyOrder(t, wire); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s/%s: wire order %v, manifest order %v", key, name, got, want)
			}
			checked++
		}
	}
	if checked < 5 {
		t.Errorf("only %d multi-argument tools were checked; the manifests moved?", checked)
	}
}

func TestLoadManifestAsTool_ReadFileListsPathBeforeOffsetAndLimit(t *testing.T) {
	params, _, err := LoadManifestAsTool("filesystem", models.ToolFileRead)
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(params)
	if got := marshaledPropertyOrder(t, wire); strings.Join(got, ",") != "path,offset,limit" {
		t.Errorf("read_file properties on the wire: %v", got)
	}
}

// marshaledPropertyOrder reads the "properties" object of a schema's JSON and returns its keys in document order.
func marshaledPropertyOrder(t *testing.T, schemaJSON []byte) []string {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(schemaJSON, &top); err != nil {
		t.Fatal(err)
	}
	props, ok := top["properties"]
	if !ok {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(props)))
	if _, err := dec.Token(); err != nil { // opening brace
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}
