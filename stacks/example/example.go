// Package example is the reference stack: the smallest package that satisfies the stack contract (ADR-0032, Constitution IV.6). The
// contract tests and the boundary tests in internal/archtest run against it, and a new stack starts as a copy of it. It is not
// registered with the service.
package example

import (
	"context"
	"encoding/json"

	"llm-proxy-stacks/sdk"
)

// ToolEcho is the one tool the reference stack declares.
const ToolEcho = "example_echo"

// Stack is the reference stack.
type Stack struct {
	host stacksdk.Host
}

// New returns the reference stack.
func New() *Stack { return &Stack{} }

// Manifest declares the stack as data.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: "example", Title: "Example", Purpose: "The reference stack the contract tests run against.",
		Layers: []stacksdk.Layer{{ID: "base", Title: "Base", Purpose: "The only layer"}},
		Modules: []stacksdk.ModuleSpec{{
			ID: "echo", Title: "Echo", Purpose: "Answers with what it was given.", Layer: "base", Sends: stacksdk.SendsNothing,
			About: stacksdk.About{Watches: "Nothing.", Never: "Sends anything.", ShowsUp: "Nowhere."},
		}},
		Tools: []stacksdk.ToolSpec{{
			Name: ToolEcho, Description: "Return the text given.", Category: "example",
			Schema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		}},
		Routes: []stacksdk.RouteSpec{
			{Method: "GET", Path: "/admin/api/example/echo/{text}"},
			{Method: "PUT", Path: "/admin/api/example/echo", Access: stacksdk.AccessStepUp},
		},
		Settings: stacksdk.SettingsSchema{Key: "example", Version: 1},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog},
	}
}

// Start keeps the host the core handed over.
func (s *Stack) Start(_ context.Context, h stacksdk.Host) error {
	s.host = h
	return nil
}

// Stop has nothing to release.
func (s *Stack) Stop(context.Context) error { return nil }

// ConfigChanged has nothing to re-read.
func (s *Stack) ConfigChanged(context.Context) error { return nil }

// CallTool runs a declared tool by name.
func (s *Stack) CallTool(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if name != ToolEcho {
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInvalid, "arguments are not valid JSON")
	}
	return json.Marshal(in)
}

// Handle answers the declared routes: a read that echoes a path value, and a write that echoes its body.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	switch req.Method + " " + req.Path {
	case "PUT /admin/api/example/echo":
		out, err := s.CallTool(ctx, ToolEcho, req.Body)
		if err != nil {
			return stacksdk.Response{}, err
		}
		return stacksdk.Response{Status: 200, ContentType: "application/json", Body: out}, nil
	}
	if text, ok := req.Params["text"]; ok && req.Method == "GET" {
		out, err := json.Marshal(struct {
			Text string `json:"text"`
		}{text})
		return stacksdk.Response{Status: 200, ContentType: "application/json", Body: out}, err
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}
