package utils_test

import (
	"testing"

	"llm-proxy/utils"
)

func TestSanitiseUrl(t *testing.T) {
	if got := utils.SanitiseUrl("http://example.com/"); got != "http://example.com" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := utils.SanitiseUrl("http://example.com"); got != "http://example.com" {
		t.Fatalf("unexpected URL: %s", got)
	}
}
