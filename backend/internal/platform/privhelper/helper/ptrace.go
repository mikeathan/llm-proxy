package helper

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const ptraceScopeFile = "/proc/sys/kernel/yama/ptrace_scope"

// YamaPtraceScope reads the host's ptrace_scope. A missing file means Yama is not loaded, which lets any process of one user read another's
// memory, so the caller reads it as an error and refuses (design decision D8).
func YamaPtraceScope() (int, error) {
	b, err := os.ReadFile(ptraceScopeFile)
	if err != nil {
		return 0, fmt.Errorf("ptrace_scope: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("ptrace_scope: %w", err)
	}
	return n, nil
}
