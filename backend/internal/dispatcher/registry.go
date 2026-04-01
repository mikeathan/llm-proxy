package dispatcher

import (
	"fmt"
	"sync"

	"llm-proxy/internal/trigger"
	"llm-proxy/models"
)

// AutomationEntry holds a registered automation with its parsed trigger.
type AutomationEntry struct {
	ID        string // "workspaceID/automationName"
	Workspace string
	Name      string
	Trigger   trigger.Trigger
	TaskFile  string
	Strategy  ExecutionStrategy
}

// AutomationRegistry manages registered automations.
type AutomationRegistry struct {
	mu         sync.RWMutex
	automations map[string]*AutomationEntry // key: "workspaceID/automationName"
}

// NewAutomationRegistry creates a new registry.
func NewAutomationRegistry() *AutomationRegistry {
	return &AutomationRegistry{
		automations: make(map[string]*AutomationEntry),
	}
}

func key(workspaceID, automationName string) string {
	return workspaceID + "/" + automationName
}

// Register registers an automation. If already registered, it updates the entry.
func (r *AutomationRegistry) Register(workspaceID string, auto *models.Automation) error {
	tr, err := trigger.New(auto.Trigger)
	if err != nil {
		return fmt.Errorf("failed to create trigger for automation %q: %w", auto.Name, err)
	}

	strategy := StrategyFromAutomation(auto)

	entry := &AutomationEntry{
		ID:        key(workspaceID, auto.Name),
		Workspace: workspaceID,
		Name:      auto.Name,
		Trigger:   tr,
		TaskFile:  auto.TaskFile,
		Strategy:  strategy,
	}

	r.mu.Lock()
	r.automations[key(workspaceID, auto.Name)] = entry
	r.mu.Unlock()

	return nil
}

// Unregister removes an automation from the registry.
func (r *AutomationRegistry) Unregister(workspaceID, automationName string) {
	r.mu.Lock()
	delete(r.automations, key(workspaceID, automationName))
	r.mu.Unlock()
}

// UnregisterWorkspace removes all automations for a workspace.
func (r *AutomationRegistry) UnregisterWorkspace(workspaceID string) {
	r.mu.Lock()
	for k, v := range r.automations {
		if v.Workspace == workspaceID {
			delete(r.automations, k)
		}
	}
	r.mu.Unlock()
}

// Get returns an automation entry by workspace and name.
func (r *AutomationRegistry) Get(workspaceID, automationName string) (*AutomationEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.automations[key(workspaceID, automationName)]
	return entry, ok
}

// List returns all automations for a workspace.
func (r *AutomationRegistry) List(workspaceID string) []*AutomationEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*AutomationEntry
	for _, v := range r.automations {
		if v.Workspace == workspaceID {
			result = append(result, v)
		}
	}
	return result
}

// ListAll returns all registered automations.
func (r *AutomationRegistry) ListAll() []*AutomationEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AutomationEntry, 0, len(r.automations))
	for _, v := range r.automations {
		result = append(result, v)
	}
	return result
}

// Count returns the total number of registered automations.
func (r *AutomationRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.automations)
}
