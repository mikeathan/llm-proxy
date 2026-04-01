package workspace

import (
	"fmt"
		"os"
	"sync"
	"testing"
)

func TestManager_AtomicStateWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "test-ws"

	var wg sync.WaitGroup
	workers := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, err := m.AcquireLock(wsID)
			if err != nil {
				t.Errorf("Worker %d failed to acquire lock: %v", i, err)
				return
			}
			defer m.ReleaseLock(f)
			state, err := m.ReadState(wsID)
			if err != nil {
				t.Errorf("Worker %d failed to read state: %v", i, err)
				return
			}
			state.LastOutput = fmt.Sprintf("output from worker %d", i)
			if err := m.WriteState(wsID, state); err != nil {
				t.Errorf("Worker %d failed to write state: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	state, err := m.ReadState(wsID)
	if err != nil {
		t.Fatalf("Failed to read final state: %v", err)
	}
	if state.LastOutput == "" {
		t.Errorf("Final state is empty")
	}
}
