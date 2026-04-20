package models

// Template represents a reusable task definition for agents.
type Template struct {
	ID          string `json:"id"`          // Unique identifier (from markdown metadata)
	Name        string `json:"name"`        // Display name
	Category    string `json:"category"`    // Category (e.g., Recon, Audit, Security)
	Description string `json:"description"` // Brief summary
	Content     string `json:"content"`     // The full markdown content
}

// TemplateMetadata is a lightweight version of a template for listing.
type TemplateMetadata struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}
