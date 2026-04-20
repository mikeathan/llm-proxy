package models

import "testing"

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
