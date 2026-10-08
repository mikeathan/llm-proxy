package process

import (
	"slices"
	"testing"
)

// ps prints the command line with spaces unescaped, so an executable path that
// contains a space spans several fields. The executable is recovered without
// mistaking an interpreter's script argument for it.
func TestSplitExecutable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		command  string
		wantBin  string
		wantArgs []string
		wantOK   bool
	}{
		{"bare name on PATH", "llama-server -m /m/q.gguf --port 8081", "llama-server", []string{"-m", "/m/q.gguf", "--port", "8081"}, true},
		{"absolute path", "/opt/homebrew/bin/llama-server --port 8081", "/opt/homebrew/bin/llama-server", []string{"--port", "8081"}, true},
		{"path with a space", "/Users/me/Library/Application Support/llama/llama-server -m /m/q.gguf --port 8081",
			"/Users/me/Library/Application Support/llama/llama-server", []string{"-m", "/m/q.gguf", "--port", "8081"}, true},
		{"variant build name still matches", "/opt/llama-server-cuda --port 1", "/opt/llama-server-cuda", []string{"--port", "1"}, true},
		{"interpreter running a script path", "/usr/bin/python3 /x/llama-server --port 8081", "", nil, false},
		{"interpreter running a same-named script", "/usr/bin/python3 llama-server.py", "", nil, false},
		{"unrelated process", "/usr/sbin/sshd -D", "", nil, false},
		{"empty", "", "", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, args, ok := splitExecutable(tc.command, "llama-server")
			if ok != tc.wantOK || bin != tc.wantBin || !slices.Equal(args, tc.wantArgs) {
				t.Errorf("splitExecutable(%q) = %q, %q, %v; want %q, %q, %v", tc.command, bin, args, ok, tc.wantBin, tc.wantArgs, tc.wantOK)
			}
		})
	}
}
