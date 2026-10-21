package prompts

import (
	"strings"
	"testing"

	"llm-proxy/models"
)

func TestTranslateJSONError(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"python literal", "invalid character 'T' in literal true (expecting 'r')", "Python-style True/False/None"},
		{"false literal", "invalid character 'F' in literal false (expecting 'a')", "Python-style True/False/None"},
		{"null literal", "invalid character 'N' in literal null (expecting 'u')", "Python-style True/False/None"},
		{"extra text", "invalid character 'x' looking for beginning of value", "extra text after the closing }"},
		{"incomplete", "unexpected end of JSON input", "missing closing } or ]"},
		{"syntax", "invalid character '}' after object key", "syntax error: invalid character '}' after object key"},
		{"anything else", "json: cannot unmarshal string into Go value of type int", "JSON parse error: json: cannot unmarshal"},
	}
	for _, c := range cases {
		if got := TranslateJSONError(c.raw, "{}"); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q does not contain %q", c.name, got, c.want)
		}
	}
}

// The plan prompt lists a tool's arguments by name; a schema's properties may be a plain map (decoded JSON) or the ordered
// form a manifest or an MCP schema now carries, and both must read the same.
func TestFormatToolParameters_OrderedAndPlainProperties(t *testing.T) {
	plain := map[string]any{
		"properties": map[string]any{
			"path":   map[string]any{"type": "string"},
			"offset": map[string]any{"type": "integer"},
			"mode":   map[string]any{},
		},
		"required": []any{"path", 7},
	}
	want := "mode, offset (integer), path (string, required)"
	if got := formatToolParameters(plain); got != want {
		t.Errorf("plain map: %q, want %q", got, want)
	}
	ordered, err := models.ParseOrderedProperties([]byte(`{"path":{"type":"string"},"offset":{"type":"integer"},"mode":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := formatToolParameters(map[string]any{"properties": ordered, "required": []any{"path"}}); got != want {
		t.Errorf("ordered properties: %q, want %q", got, want)
	}
	for name, schema := range map[string]any{
		"not a map":     "x",
		"no properties": map[string]any{"type": "object"},
		"empty":         map[string]any{"properties": map[string]any{}},
		"odd shape":     map[string]any{"properties": 5},
	} {
		if got := formatToolParameters(schema); got != "" {
			t.Errorf("%s: %q, want empty", name, got)
		}
	}
}
