package models

// NewsroomConfig is the Newsroom stack's settings (settings.yml → newsroom:). Like RadioConfig it is not part of UserSettings, so saving the ordinary
// settings form cannot overwrite it. The module is on until the operator switches it off.
type NewsroomConfig struct {
	// Timezone is the IANA name the feed groups its days by; empty means UTC.
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	// Disabled is the Watches module's switch, stored as an "off" flag so a fresh install is on.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	// BriefsDisabled is the Briefs module's switch, stored the same way.
	BriefsDisabled bool `yaml:"briefs_disabled,omitempty" json:"briefs_disabled,omitempty"`
	// ModelCallsPerDay is how many model calls the stack may make a day, watches and briefs together. Zero means the core's default; the core holds it to
	// its own ceiling.
	ModelCallsPerDay int `yaml:"model_calls_per_day,omitempty" json:"model_calls_per_day,omitempty"`
}
