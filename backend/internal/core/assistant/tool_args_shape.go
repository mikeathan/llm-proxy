package assistant

import (
	"slices"
	"strings"
)

// argumentShape renders a tool's parameter schema as the call a model should send, for example
// {tasks: [{agent, task, context?}]}: required fields first, optional ones marked "?", nested objects and lists shown
// in place. A model that sent the wrong shape needs the right one in the error, not only the name of the missing field.
func argumentShape(schema map[string]any) string {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return ""
	}
	required := requiredFields(schema)
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		ra, rb := slices.Contains(required, a), slices.Contains(required, b)
		switch {
		case ra != rb && ra:
			return -1
		case ra != rb:
			return 1
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		part := name
		if child, ok := props[name].(map[string]any); ok {
			if nested := nestedShape(child); nested != "" {
				part += ": " + nested
			}
		}
		if !slices.Contains(required, name) {
			part += "?"
		}
		parts = append(parts, part)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// nestedShape is the shape of an object property, or of the items of a list property in brackets; scalars have none.
func nestedShape(schema map[string]any) string {
	if items, ok := schema["items"].(map[string]any); ok {
		if inner := nestedShape(items); inner != "" {
			return "[" + inner + "]"
		}
		return ""
	}
	return argumentShape(schema)
}

// requiredFields reads a schema's required list whether it was decoded from JSON ([]any) or built in Go ([]string).
func requiredFields(schema map[string]any) []string {
	switch r := schema["required"].(type) {
	case []string:
		return r
	case []any:
		out := make([]string, 0, len(r))
		for _, v := range r {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
