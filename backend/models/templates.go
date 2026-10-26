package models

// Template represents a reusable task definition for agents.
type Template struct {
	ID          string `json:"id"`          // Unique identifier (from markdown metadata)
	Name        string `json:"name"`        // Display name
	Category    string `json:"category"`    // Category (e.g., Recon, Audit, Security)
	Description string `json:"description"` // Brief summary
	Content     string `json:"content"`     // The full markdown content
	// Defaults are the automation settings the playbook wants when an automation is created from it, declared in its
	// header. The form applies them and the user can still change each one.
	Defaults TemplateDefaults `json:"defaults"`
}

// TemplateDefaults are the optional automation settings a playbook declares. A field left empty is not declared.
type TemplateDefaults struct {
	Profile      string `json:"profile,omitempty"`       // prompt profile (agent) id
	NetworkGrant string `json:"network_grant,omitempty"` // none | lan | internet_only | internet
	Journal      *bool  `json:"journal,omitempty"`       // nil = not declared
	Schedule     string `json:"schedule,omitempty"`      // a cron expression the form offers as the first schedule
}

// TemplateMetadata is a lightweight version of a template for listing.
type TemplateMetadata struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	// Description is the playbook's one-line summary, so the library can say what it is for.
	Description string           `json:"description,omitempty"`
	Defaults    TemplateDefaults `json:"defaults"`
}
