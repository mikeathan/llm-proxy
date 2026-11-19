package toolregistry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

type fakeSource struct {
	tools   []stacksdk.DynamicTool
	listErr error
	result  stacksdk.ToolResult
	callErr error
	called  []string
	args    []string
}

func (f *fakeSource) ListTools(context.Context) ([]stacksdk.DynamicTool, error) {
	return f.tools, f.listErr
}
func (f *fakeSource) CallDynamic(_ context.Context, name string, args json.RawMessage) (stacksdk.ToolResult, error) {
	f.called = append(f.called, name)
	f.args = append(f.args, string(args))
	return f.result, f.callErr
}

func dyn(name, schema string) stacksdk.DynamicTool {
	return stacksdk.DynamicTool{Name: name, Description: "d", Schema: json.RawMessage(schema), Source: "files", Ask: true}
}

func TestStackTools_WithNoStackItOffersNothingAndRefusesCalls(t *testing.T) {
	s := NewStackTools()
	if tools, err := s.ListTools(t.Context()); err != nil || len(tools) != 0 {
		t.Errorf("list: %v %v", tools, err)
	}
	if _, err := s.CallTool(t.Context(), "mcp__a__b", nil); !errors.Is(err, ErrDynamicToolsUnavailable) {
		t.Errorf("call: %v", err)
	}
	if managed, ask := s.Policy("mcp__a__b"); !managed || !ask {
		t.Error("with no stack a name that looks like a stack's still asks")
	}
}

func TestStackTools_ListsOnlyWhatPassesTheChecks(t *testing.T) {
	big := `{"type":"object","properties":{"x":{"description":"` + strings.Repeat("a", stacksdk.MaxDynamicToolSchema) + `"}}}`
	src := &fakeSource{tools: []stacksdk.DynamicTool{
		dyn("mcp__files__ok", `{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"string"}},"required":["a"]}`),
		dyn("bad name", `{"type":"object"}`),
		dyn("mcp__files__ok", `{"type":"object"}`),
		dyn(models.ToolTerminalExecute, `{"type":"object"}`),
		dyn("mcp__files__huge", big),
		dyn("mcp__files__notjson", `[1]`),
		dyn("mcp__files__notype", `{"properties":{}}`),
	}}
	s := NewStackTools()
	s.Set(src)
	tools, err := s.ListTools(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Function.Name)
		if tl.Type != "function" {
			t.Errorf("%s: type %q", tl.Function.Name, tl.Type)
		}
	}
	if strings.Join(names, ",") != "mcp__files__ok,mcp__files__notype" {
		t.Fatalf("offered %v: a bad name, a duplicate, a built-in's name, a schema that is too large or not an object are all left out", names)
	}
	if p, ok := tools[1].Function.Parameters.(map[string]any); !ok || p["type"] != "object" {
		t.Error("a schema with no type is an object")
	}
	raw, _ := json.Marshal(tools[0].Function.Parameters)
	if strings.Index(string(raw), `"a"`) > strings.Index(string(raw), `"b"`) {
		t.Errorf("the required argument comes first: %s", raw)
	}
}

func TestStackTools_TextFromAServerIsCutAndCleaned(t *testing.T) {
	d := dyn("mcp__files__t", `{"type":"object"}`)
	d.Description = "line\x1b[31m" + strings.Repeat("é", stacksdk.MaxDynamicToolDescription)
	s := NewStackTools()
	s.Set(&fakeSource{tools: []stacksdk.DynamicTool{d}})
	tools, _ := s.ListTools(t.Context())
	got := tools[0].Function.Description
	if len(got) > stacksdk.MaxDynamicToolDescription || strings.Contains(got, "\x1b") {
		t.Errorf("description: %d bytes, %q", len(got), got[:20])
	}
}

func TestStackTools_ThereIsAHardCountLimit(t *testing.T) {
	var many []stacksdk.DynamicTool
	for i := range stacksdk.MaxDynamicTools + 20 {
		many = append(many, dyn("mcp__s__t"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('a'+i/26)), `{"type":"object"}`))
	}
	s := NewStackTools()
	s.Set(&fakeSource{tools: many})
	tools, _ := s.ListTools(t.Context())
	if len(tools) > stacksdk.MaxDynamicTools {
		t.Errorf("%d tools", len(tools))
	}
}

func TestStackTools_PolicyAsksUnlessTheServerIsSetToAllowAndFailsClosed(t *testing.T) {
	allow := dyn("mcp__trusted__echo", `{"type":"object"}`)
	allow.Ask = false
	src := &fakeSource{tools: []stacksdk.DynamicTool{dyn("mcp__files__echo", `{"type":"object"}`), allow}}
	s := NewStackTools()
	s.Set(src)
	cases := []struct {
		name         string
		managed, ask bool
	}{
		{"mcp__files__echo", true, true},
		{"mcp__trusted__echo", true, false},
		{"mcp__removed__echo", true, true}, // looks like a stack's, nothing offers it now: ask
		{"read_file", false, false},
		{models.ToolTerminalExecute, false, false},
		{"someone_elses_tool", false, false},
	}
	for _, c := range cases {
		if managed, ask := s.Policy(c.name); managed != c.managed || ask != c.ask {
			t.Errorf("%s: managed=%v ask=%v, want %v %v", c.name, managed, ask, c.managed, c.ask)
		}
	}
	src.listErr = errors.New("the stack failed")
	if managed, ask := s.Policy("mcp__trusted__echo"); !managed || !ask {
		t.Error("when the stack cannot be asked, a call asks and never passes")
	}
}

func TestStackTools_CallsAndTheirResults(t *testing.T) {
	src := &fakeSource{result: stacksdk.ToolResult{Text: `{"a":1}`}}
	s := NewStackTools()
	s.Set(src)
	got, err := s.CallTool(t.Context(), "mcp__files__echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := got.(map[string]any); !ok || m["a"] != float64(1) {
		t.Errorf("JSON text is returned as the structure it is: %#v", got)
	}
	if src.args[0] != `{"text":"hi"}` {
		t.Errorf("args = %s", src.args[0])
	}
	src.result = stacksdk.ToolResult{Text: "plain words"}
	if got, _ := s.CallTool(t.Context(), "mcp__files__echo", nil); got != "plain words" {
		t.Errorf("text stays text: %#v", got)
	}
	src.result = stacksdk.ToolResult{Text: "it failed", IsError: true}
	if got, err := s.CallTool(t.Context(), "mcp__files__echo", nil); err == nil || got != "it failed" {
		t.Errorf("a tool that ran and failed is its own words and an error: %#v %v", got, err)
	}
	src.callErr = stacksdk.Errorf(stacksdk.CodeUnavailable, "MCP servers are switched off")
	if _, err := s.CallTool(t.Context(), "mcp__files__echo", nil); err == nil || err.Error() != "MCP servers are switched off" {
		t.Errorf("a stack's refusal is its message, without the code: %v", err)
	}
}
