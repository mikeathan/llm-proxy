package assistant

import (
	"strings"
	"testing"
)

func TestBuildJailPrompt(t *testing.T) {
	relWs := "custom/path"
	workspaceID := "my-workspace"
	
	prompt := BuildJailPrompt(relWs, workspaceID)
	
	if !strings.Contains(prompt, "STRICT WORKSPACE RULES:") {
		t.Errorf("prompt missing header: %s", prompt)
	}
	
	expectedPath := "custom/path/my-workspace/"
	if !strings.Contains(prompt, expectedPath) {
		t.Errorf("prompt missing expected path %s: %s", expectedPath, prompt)
	}
	
	// Test escaping/prefixing
	if !strings.Contains(prompt, "prefix the path with 'custom/path/my-workspace/'") {
		t.Errorf("prompt has incorrect instructions: %s", prompt)
	}
}
