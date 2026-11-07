package example

import (
	"context"
	"encoding/json"
	"testing"

	"llm-proxy-stacks/sdk"
)

func TestManifestIsValid(t *testing.T) {
	if err := New().Manifest().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEveryDeclaredToolIsCalledByNameWithJSON(t *testing.T) {
	s := New()
	if err := s.Start(context.Background(), stacksdk.Host{}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range s.Manifest().Tools {
		out, err := s.CallTool(context.Background(), spec.Name, json.RawMessage(`{"text":"hi"}`))
		if err != nil || string(out) != `{"text":"hi"}` {
			t.Errorf("%s: %s, %v", spec.Name, out, err)
		}
	}
	if _, err := s.CallTool(context.Background(), "nope", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an undeclared tool is not_found, got %v", err)
	}
	if _, err := s.CallTool(context.Background(), ToolEcho, json.RawMessage(`{`)); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("bad arguments are invalid, got %v", err)
	}
}
