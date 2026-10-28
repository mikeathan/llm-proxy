package models

// RequirementCode names a prerequisite a feature is missing.
type RequirementCode string

const (
	RequirementScopeEmpty      RequirementCode = "scope_empty"
	RequirementHostNetworkOff  RequirementCode = "host_network_off"
	RequirementNmapMissing     RequirementCode = "nmap_missing"
	RequirementNoSandbox       RequirementCode = "no_sandbox"
	RequirementVulnDataMissing RequirementCode = "vuln_data_missing"
	RequirementVulnDataOld     RequirementCode = "vuln_data_old"
)

// FixKind is what a person can do about a requirement.
type FixKind string

const (
	// FixOpenSettings opens a settings section.
	FixOpenSettings FixKind = "open_settings"
	// FixCopy offers text to copy, such as an install command.
	FixCopy FixKind = "copy"
	// FixConfirm asks the person to confirm an action that has a consequence.
	FixConfirm FixKind = "confirm"
)

// RequirementFix is the one step that would meet a requirement. Exactly one of Section, Text or Action is set,
// matching Kind.
type RequirementFix struct {
	Kind    FixKind `json:"kind"`
	Section string  `json:"section,omitempty"`
	Text    string  `json:"text,omitempty"`
	Action  string  `json:"action,omitempty"`
}

// Requirement is one unmet prerequisite with a message a person can act on. Blocking requirements hide the feature;
// the others limit it (a screen shows them, the feature still works in a reduced way).
type Requirement struct {
	Code     RequirementCode `json:"code"`
	Message  string          `json:"message"`
	Blocking bool            `json:"blocking"`
	Fix      *RequirementFix `json:"fix,omitempty"`
}
