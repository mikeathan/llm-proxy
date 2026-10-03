// memory_mode.go — the MemoryMode value object: an override of the global hot-memory default.
// It lives in the leaf models package because Automation and WorkspaceConfig persist it.
package models

// MemoryMode overrides the global hot-memory default for one surface (an automation, or a workspace's assistant).
// Unknown non-empty values are rejected at the HTTP boundary (400).
type MemoryMode string

const (
	MemoryModeInherit MemoryMode = ""
	MemoryModeOn      MemoryMode = "on"
	MemoryModeOff     MemoryMode = "off"
)

// Valid reports whether the value is an explicit override; inherit is resolved by Effective instead.
func (m MemoryMode) Valid() bool {
	return m == MemoryModeOn || m == MemoryModeOff
}

// Effective resolves the override against the global default; an unreadable value reads as inherit.
func (m MemoryMode) Effective(globalDefault bool) bool {
	switch m {
	case MemoryModeOn:
		return true
	case MemoryModeOff:
		return false
	default:
		return globalDefault
	}
}
