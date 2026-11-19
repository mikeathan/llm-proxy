package toolregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// DynamicTools is how the run path sees the tools a stack offers at run time (ADR-0069): they are listed, called by name, and judged by the
// guardrail engine through Policy.
type DynamicTools interface {
	ListTools(ctx context.Context) ([]proxy.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
	UseNativeTools() bool
	// Policy says whether name is a tool of a stack's, and whether its owner wants to approve each call.
	Policy(name string) (managed, ask bool)
}

// dynamicListTimeout bounds one look at what a stack offers. A stack answers from its caches; the bound only guards against one that does not.
const dynamicListTimeout = 2 * time.Second

// dynamicNamePrefix is the prefix of a name a stack's tool must carry for the guardrail to fail closed on it once it is no longer offered: a
// call to such a name that nothing offers now still asks, and never passes.
const dynamicNamePrefix = "mcp__"

// ErrDynamicToolsUnavailable is the answer to a call while no stack provides tools.
var ErrDynamicToolsUnavailable = errors.New("no tool server is available")

// StackTools adapts a stack that implements stacksdk.ToolSource to the run path. It is built before the stack is hosted and takes the stack later
// (Set): until then it offers nothing and refuses every call. It owns what the core must not leave to the stack: it checks every name, cuts and
// cleans the text a server wrote, bounds the schemas and the counts, and orders each schema required-first.
type StackTools struct {
	mu  sync.RWMutex
	src stacksdk.ToolSource
}

// NewStackTools returns an adapter with no stack behind it.
func NewStackTools() *StackTools { return &StackTools{} }

// Set attaches the stack's tool source.
func (s *StackTools) Set(src stacksdk.ToolSource) {
	s.mu.Lock()
	s.src = src
	s.mu.Unlock()
}

func (s *StackTools) source() stacksdk.ToolSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.src
}

// UseNativeTools is true: these tools are always passed as API-level tool definitions, as MCP tools were.
func (s *StackTools) UseNativeTools() bool { return true }

func (s *StackTools) offered(ctx context.Context) ([]stacksdk.DynamicTool, error) {
	src := s.source()
	if src == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, dynamicListTimeout)
	defer cancel()
	return src.ListTools(ctx)
}

// ListTools lists what the stack offers, with every name checked and every text bounded. A tool that fails a check is left out and logged by name.
func (s *StackTools) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	offered, err := s.offered(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the tools of a stack: %w", err)
	}
	seen := make(map[string]bool, len(offered))
	out := make([]proxy.Tool, 0, len(offered))
	for _, t := range offered {
		if len(out) >= stacksdk.MaxDynamicTools {
			logging.Warn("a stack offers more tools than the limit; the rest are left out", "limit", stacksdk.MaxDynamicTools)
			break
		}
		tool, reason := convertDynamic(t, seen)
		if reason != "" {
			logging.Warn("a tool a stack offers was left out", "tool", cutForLog(t.Name), "reason", reason)
			continue
		}
		seen[t.Name] = true
		out = append(out, tool)
	}
	return out, nil
}

// convertDynamic checks one tool and turns it into the run path's shape; a non-empty reason says why it was refused.
func convertDynamic(t stacksdk.DynamicTool, seen map[string]bool) (proxy.Tool, string) {
	switch {
	case !stacksdk.ValidDynamicToolName(t.Name):
		return proxy.Tool{}, "the name is not valid"
	case seen[t.Name]:
		return proxy.Tool{}, "the name is offered twice"
	case isBuiltin(t.Name):
		return proxy.Tool{}, "the name belongs to a built-in tool"
	case len(t.Schema) > stacksdk.MaxDynamicToolSchema:
		return proxy.Tool{}, "the schema is too large"
	}
	var schema map[string]any
	if err := json.Unmarshal(t.Schema, &schema); err != nil || schema == nil {
		return proxy.Tool{}, "the schema is not a JSON object"
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	schemaRequiredFirst(schema)
	return proxy.Tool{Type: "function", Function: proxy.FunctionSchema{
		Name: t.Name, Description: stacksdk.CleanText(t.Description, stacksdk.MaxDynamicToolDescription), Parameters: schema,
	}}, ""
}

func isBuiltin(name string) bool {
	_, ok := models.SpecOf(name)
	return ok
}

func cutForLog(s string) string { return stacksdk.CleanText(s, stacksdk.MaxDynamicToolName) }

// Policy says whether a name is a tool the stack offers now and whether its owner wants to approve each call. It fails closed: a name that looks like
// a stack's but that nothing offers now (the server was removed or its list changed mid-run), or any failure to ask, means ask.
func (s *StackTools) Policy(name string) (managed, ask bool) {
	looksOurs := strings.HasPrefix(name, dynamicNamePrefix)
	if isBuiltin(name) {
		return false, false
	}
	offered, err := s.offered(context.Background())
	if err != nil {
		return looksOurs, looksOurs
	}
	for _, t := range offered {
		if t.Name == name {
			return true, t.Ask
		}
	}
	return looksOurs, looksOurs
}

// CallTool runs one tool by name. The stack's own failure is returned with its message; a tool that ran and failed returns its own words as the result
// and an error, as the agent loop reads a failed tool.
func (s *StackTools) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	src := s.source()
	if src == nil {
		return nil, ErrDynamicToolsUnavailable
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the tool arguments: %w", err)
	}
	res, err := src.CallDynamic(ctx, name, raw)
	if err != nil {
		var coded *stacksdk.Error
		if errors.As(err, &coded) {
			return nil, errors.New(coded.Message)
		}
		return nil, fmt.Errorf("tool call failed: %w", err)
	}
	if res.IsError {
		return res.Text, fmt.Errorf("tool execution error: %s", res.Text)
	}
	return structuredOrText(res.Text), nil
}

// structuredOrText returns a JSON object or array as the structure it is, and anything else as the text.
func structuredOrText(text string) any {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var v any
		if json.Unmarshal([]byte(trimmed), &v) == nil {
			return v
		}
	}
	return text
}

// schemaRequiredFirst puts a tool schema's required arguments first in its properties (see models.OrderedByRequired), because the schema reaches us
// already decoded into a map and a map has no order of its own. A schema without a properties object, or whose required list is not a list of names, is
// left as it is.
func schemaRequiredFirst(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	var required []string
	if list, ok := schema["required"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				required = append(required, name)
			}
		}
	}
	schema["properties"] = models.OrderedByRequired(props, required)
}

var _ DynamicTools = (*StackTools)(nil)
