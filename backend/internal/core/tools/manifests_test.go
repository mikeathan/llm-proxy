package tools

import (
	"testing"
	"testing/fstest"
)

func TestLoadManifestFromFS(t *testing.T) {
	fsys := fstest.MapFS{"demo.json": {Data: []byte(`{"tool_name":"demo","version":"1.0","description":"Demo.","parameters":{"type":"object","properties":{"a":{"type":"string"}}}}`)}}

	params, desc, err := LoadManifestFromFS(fsys, "demo", "demo")
	if err != nil {
		t.Fatalf("LoadManifestFromFS: %v", err)
	}
	if desc != "Demo." || params["type"] != "object" {
		t.Errorf("got description %q, params %v", desc, params)
	}

	if _, _, err := LoadManifestFromFS(fsys, "missing", "missing"); err == nil {
		t.Error("a missing manifest must be an error")
	}
}
