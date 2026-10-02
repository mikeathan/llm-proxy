// memory_mode.go — the MemoryMode value object: the per-automation switch for
// hot-memory injection (docs/PLANS/memory/small-context-memory.md, Phase 3).
// It lives in the leaf models package because Automation persists it.
package models

// MemoryMode selects whether an automation run receives the frozen hot-memory
// block in its head system message. The empty string means unset and behaves
// as MemoryModeOff, so existing automations are unchanged. Unknown non-empty
// values are rejected at the HTTP boundary (400).
//
// "hot+hints" (step-aware hints delivered with tool results) is deliberately not
// a value yet: it ships only after a measured win on the memory scoreboard.
type MemoryMode string

const (
	MemoryModeOff MemoryMode = "off"
	MemoryModeHot MemoryMode = "hot"
)

// Valid reports whether the value is a registered, non-empty memory mode.
func (m MemoryMode) Valid() bool {
	return m == MemoryModeOff || m == MemoryModeHot
}

// HotEnabled reports whether the run should receive hot memory.
func (m MemoryMode) HotEnabled() bool {
	return m == MemoryModeHot
}
