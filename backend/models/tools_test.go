package models

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestToolConstants(t *testing.T) {
	// Verify critical tool names remain unchanged to prevent breaking LLM prompts/tool-calling
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"TerminalExecute", ToolTerminalExecute, "execute_terminal_command"},
		{"InternetSearch", ToolInternetSearch, "internet_search"},
		{"NotifyUser", ToolNotifyUser, "notify_user"},
		{"FileRead", ToolFileRead, "read_file"},
		{"FileWrite", ToolFileWrite, "write_file"},
		{"DirectoryList", ToolDirectoryList, "list_directory"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("Tool constant %s mismatch: got %v, want %v", tt.name, tt.constant, tt.expected)
		}
	}
}

func TestCategoryConstants(t *testing.T) {
	// Verify category names match manifest filenames
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Terminal", CategoryTerminal, "terminal"},
		{"FileSystem", CategoryFileSystem, "filesystem"},
		{"Search", CategorySearch, "search"},
		{"Communication", CategoryCommunication, "communication"},
		{"Security", CategoryGlobal, "security"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("Category constant %s mismatch: got %v, want %v", tt.name, tt.constant, tt.expected)
		}
	}
}

// A rejected credential carries a short reason for people, while errors.Is still classifies it and Error() keeps the
// full provider detail for logs and the model.
func TestUnavailableReason(t *testing.T) {
	cause := errors.New(`brave API error (status 422): {"error":{"detail":"The provided API key is invalid."}}`)
	rejected := fmt.Errorf("internet search failed: %w", &ToolUnavailableError{Reason: "brave rejected the API key (HTTP 422)", Cause: cause})

	if !errors.Is(rejected, ErrToolUnavailable) {
		t.Fatal("a ToolUnavailableError must still match ErrToolUnavailable")
	}
	if got := UnavailableReason(rejected); got != "brave rejected the API key (HTTP 422)" {
		t.Errorf("UnavailableReason = %q", got)
	}
	if msg := rejected.Error(); !strings.Contains(msg, "The provided API key is invalid.") || !strings.Contains(msg, "operator action required") {
		t.Errorf("Error() must keep the full detail and the classification text: %q", msg)
	}
	plain := fmt.Errorf("not configured: %w", ErrToolUnavailable)
	if got := UnavailableReason(plain); got != plain.Error() {
		t.Errorf("an error without a short reason falls back to its text, got %q", got)
	}
}
