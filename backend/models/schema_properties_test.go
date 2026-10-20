package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOrderedProperties_MarshalsInAuthoredOrder(t *testing.T) {
	props, err := ParseOrderedProperties([]byte(`{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}}`))
	if err != nil {
		t.Fatal(err)
	}
	schema := map[string]any{"type": "object", "properties": props, "required": []string{"path"}}
	got, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !(strings.Index(text, `"path"`) < strings.Index(text, `"offset"`) && strings.Index(text, `"offset"`) < strings.Index(text, `"limit"`)) {
		t.Errorf("properties not in authored order: %s", text)
	}
	if !strings.Contains(text, `"required":["path"]`) || !strings.Contains(text, `"type":"object"`) {
		t.Errorf("the rest of the schema changed: %s", text)
	}
}

func TestOrderedProperties_KeepsValuesForReaders(t *testing.T) {
	props, _ := ParseOrderedProperties([]byte(`{"b":{"type":"string"},"a":{"type":"integer"}}`))
	if got := props.Keys(); len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Errorf("keys: %v", got)
	}
	m := props.Map()
	if m["a"].(map[string]any)["type"] != "integer" || len(m) != 2 {
		t.Errorf("map: %v", m)
	}
}

func TestSchemaProperties_AcceptsPlainAndOrderedForms(t *testing.T) {
	plain := map[string]any{"properties": map[string]any{"x": map[string]any{"type": "string"}}}
	ordered, _ := ParseOrderedProperties([]byte(`{"x":{"type":"string"}}`))
	for name, schema := range map[string]map[string]any{"plain": plain, "ordered": {"properties": ordered}} {
		got, ok := SchemaProperties(schema)
		if !ok || len(got) != 1 {
			t.Errorf("%s: %v %v", name, got, ok)
		}
	}
	if _, ok := SchemaProperties(map[string]any{}); ok {
		t.Error("a schema with no properties must report none")
	}
	if _, ok := SchemaProperties(map[string]any{"properties": 5}); ok {
		t.Error("a non-object properties value must report none")
	}
}

func TestParseOrderedProperties_RejectsWhatIsNotAnObject(t *testing.T) {
	for _, in := range []string{`[1]`, `"x"`, `{"a":`, ``} {
		if _, err := ParseOrderedProperties([]byte(in)); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
	empty, err := ParseOrderedProperties([]byte(`{}`))
	if err != nil || len(empty.Keys()) != 0 {
		t.Errorf("empty object: %v %v", empty, err)
	}
}
