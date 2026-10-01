package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The frontend mirrors MergeWith to show which workspace guardrail values are
// inherited, overridden or exceptions (frontend/src/domain/guardrailLayers.ts).
// Both sides run the same fixture, so the mirror cannot drift from this code.
const mergeFixtureDir = "../../frontend/src/__TESTS__/fixtures"

type mergeCase struct {
	Name      string          `json:"name"`
	Workspace json.RawMessage `json:"workspace"`
	Effective json.RawMessage `json:"effective"`
}

func TestMergeWithMatchesFrontendFixture(t *testing.T) {
	var cases []mergeCase
	readFixture(t, "guardrailMerge.cases.json", &cases)
	globalRaw, err := os.ReadFile(filepath.Join(mergeFixtureDir, "guardrailMerge.global.json"))
	if err != nil {
		t.Fatalf("read global fixture: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var merged AgentGuardrailsConfig
			if err := json.Unmarshal(globalRaw, &merged); err != nil {
				t.Fatalf("decode global: %v", err)
			}
			if layer := decodeLayer(t, tc.Workspace); layer != nil {
				merged.MergeWith(layer)
			}
			assertSameJSON(t, merged, tc.Effective)
		})
	}
}

// decodeLayer decodes a workspace layer the way the runtime sees a saved one:
// it is read back from YAML, where a written network block is always present.
func decodeLayer(t *testing.T, raw json.RawMessage) *AgentGuardrailsConfig {
	t.Helper()
	if string(raw) == "null" {
		return nil
	}
	var layer AgentGuardrailsConfig
	if err := json.Unmarshal(raw, &layer); err != nil {
		t.Fatalf("decode layer: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("decode layer keys: %v", err)
	}
	_, layer.Network.present = keys["network"]
	return &layer
}

func readFixture(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(mergeFixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
}

func assertSameJSON(t *testing.T, got AgentGuardrailsConfig, want json.RawMessage) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode merged: %v", err)
	}
	var gotMap, wantMap map[string]any
	if err := json.Unmarshal(gotRaw, &gotMap); err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	// Round-trip the expectation through the struct too, so both sides drop
	// the same omitempty fields.
	var wantCfg AgentGuardrailsConfig
	if err := json.Unmarshal(want, &wantCfg); err != nil {
		t.Fatalf("decode expected: %v", err)
	}
	wantRaw, _ := json.Marshal(wantCfg)
	if err := json.Unmarshal(wantRaw, &wantMap); err != nil {
		t.Fatalf("decode expected map: %v", err)
	}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("merged policy differs from the fixture\n got: %s\nwant: %s", gotRaw, wantRaw)
	}
}
