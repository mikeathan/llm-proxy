package proxy

import (
	"fmt"
	"strings"
)

// RequestConstraint applies a provider-specific output constraint to a
// ChatRequest to prevent malformed tool call output at the generation layer.
type RequestConstraint interface {
	Apply(req *ChatRequest, tools []Tool) bool
}

// GBNFConstraint generates a GBNF grammar from tool schemas and sets
// req.Grammar.  The grammar constrains the model's token output so it can
// only produce valid JSON matching one of the available tool schemas.
// Only effective with llama.cpp (or TGI/vLLM with GBNF support).
type GBNFConstraint struct{}

func (c *GBNFConstraint) Apply(req *ChatRequest, tools []Tool) bool {
	if len(tools) == 0 {
		return false
	}
	grammar := buildGBNF(tools)
	if grammar == "" {
		return false
	}
	req.Grammar = &grammar
	return true
}

// buildGBNF generates a disjunctive GBNF grammar covering all tool schemas.
func buildGBNF(tools []Tool) string {
	var rules []string
	for _, t := range tools {
		schema, ok := t.Function.Parameters.(map[string]any)
		if !ok {
			continue
		}
		body := buildObjectGBNF(schema)
		if body != "" {
			rules = append(rules, body)
		}
	}
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("root ::= ")
	for i, r := range rules {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(r)
	}
	b.WriteString("\n\n")
	b.WriteString(commonRules())
	return b.String()
}

// buildObjectGBNF generates GBNF for a JSON object with the given schema.
// Pattern: "{" (pair ("," pair)*)? "}"
// Required-field enforcement is handled by the parser, not GBNF.
// GBNF prevents structural errors (invalid JSON); missing fields trigger
// system_error tool recovery (existing behaviour).
func buildObjectGBNF(schema map[string]any) string {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return ""
	}
	var allPairs []string
	for name, ps := range props {
		p, _ := ps.(map[string]any)
		if p == nil {
			continue
		}
		g := buildFieldGBNF(name, p)
		if g == "" {
			continue
		}
		allPairs = append(allPairs, g)
	}
	if len(allPairs) == 0 {
		return ""
	}
	pairAlt := strings.Join(allPairs, " | ")
	// (pair ("," pair)*)? — zero or more key:value pairs, comma-separated.
	pairList := fmt.Sprintf("(%s (\",\" %s)*)?", pairAlt, pairAlt)
	return fmt.Sprintf("\"{\" %s \"}\"", pairList)
}

// buildFieldGBNF generates GBNF for a single field name + value.
func buildFieldGBNF(name string, p map[string]any) string {
	fieldKey := fmt.Sprintf("\"%s\"", name)
	val := buildValueGBNF(p)
	if val == "" {
		return ""
	}
	return fmt.Sprintf("%s \":\" %s", fieldKey, val)
}

// buildValueGBNF generates GBNF for a JSON value from its schema definition.
func buildValueGBNF(p map[string]any) string {
	// Enum takes priority over type
	if enum, ok := p["enum"].([]any); ok && len(enum) > 0 {
		var parts []string
		for _, e := range enum {
			parts = append(parts, fmt.Sprintf("\"%v\"", e))
		}
		return strings.Join(parts, " | ")
	}
	typ, _ := p["type"].(string)
	switch typ {
	case "string":
		return "string-lit"
	case "integer", "number":
		return "int-lit"
	case "boolean":
		return "bool-lit"
	case "array":
		items, _ := p["items"].(map[string]any)
		if items == nil {
			return "array-lit"
		}
		itemVal := buildValueGBNF(items)
		if itemVal == "" {
			return "array-lit"
		}
		return fmt.Sprintf("\"[\" (%s (\",\" %s)*)? \"]\"", itemVal, itemVal)
	default:
		return ""
	}
}

// commonRules returns the shared GBNF terminal rules used by all tool grammars.
func commonRules() string {
	return `string-lit ::= "\"" ([^"]*) "\""
int-lit ::= [0-9]+
bool-lit ::= "true" | "false"
array-lit ::= "[" (string-lit ("," string-lit)*)? "]"
`
}
