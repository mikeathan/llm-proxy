package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// OrderedProperties is a tool schema's "properties" object that keeps the order its author wrote. Go marshals a map's
// keys sorted, which puts a tool's arguments in alphabetical order on the wire, and a model writes a call in the order
// the schema lists the arguments: with read_file's properties sorted (limit, offset, path) a 16k local model left out
// the optional offset 9 times in 10; listed as authored (path, offset, limit) it used it 10 times in 10.
type OrderedProperties struct {
	keys   []string
	values map[string]any
}

// ParseOrderedProperties reads a JSON object, remembering the order of its keys.
func ParseOrderedProperties(raw []byte) (OrderedProperties, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	open, err := dec.Token()
	if err != nil {
		return OrderedProperties{}, err
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return OrderedProperties{}, fmt.Errorf("properties must be a JSON object")
	}
	props := OrderedProperties{values: map[string]any{}}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return OrderedProperties{}, err
		}
		key, _ := keyToken.(string)
		var value any
		if err := dec.Decode(&value); err != nil {
			return OrderedProperties{}, err
		}
		props.keys = append(props.keys, key)
		props.values[key] = value
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return OrderedProperties{}, err
	}
	return props, nil
}

// MarshalJSON writes the properties in their authored order.
func (p OrderedProperties) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range p.keys {
		if i > 0 {
			out.WriteByte(',')
		}
		name, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(p.values[key])
		if err != nil {
			return nil, err
		}
		out.Write(name)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// Keys are the property names in authored order.
func (p OrderedProperties) Keys() []string { return append([]string(nil), p.keys...) }

// Map is the properties as a plain map, for code that looks one up by name.
func (p OrderedProperties) Map() map[string]any {
	out := make(map[string]any, len(p.values))
	for k, v := range p.values {
		out[k] = v
	}
	return out
}

// SchemaProperties returns a tool schema's properties as a map, whether the schema holds them as a plain map or as
// OrderedProperties; false when it has none.
func SchemaProperties(schema map[string]any) (map[string]any, bool) {
	switch props := schema["properties"].(type) {
	case map[string]any:
		return props, true
	case OrderedProperties:
		return props.Map(), true
	}
	return nil, false
}

// OrderedByRequired orders a schema's properties for a schema that carries no authored order (one decoded from an MCP
// server's JSON into a map): the required names first, in the order the schema lists them, then the rest
// alphabetically. Required-first matters for the same reason authored order does: against the real 16k model with
// read_file's properties, alphabetical order (limit, offset, path) used the optional offset 1 time in 10, required
// first (path, limit, offset) 10 in 10. Names listed as required but absent are ignored.
func OrderedByRequired(props map[string]any, required []string) OrderedProperties {
	ordered := OrderedProperties{values: make(map[string]any, len(props))}
	for k, v := range props {
		ordered.values[k] = v
	}
	seen := make(map[string]bool, len(required))
	for _, name := range required {
		if _, ok := props[name]; ok && !seen[name] {
			ordered.keys = append(ordered.keys, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(props))
	for name := range props {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	ordered.keys = append(ordered.keys, rest...)
	return ordered
}
