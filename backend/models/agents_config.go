package models

// AgentsConfig is the host-wide policy of the Agents stack (settings.yml → agents:). Like NetauditConfig it is not part of
// UserSettings, so saving the ordinary settings form cannot overwrite it. Both parts have always run, so each is a negative
// flag: an install with nothing stored keeps them on.
type AgentsConfig struct {
	// ActivityOff stops the agent security record: refused tool calls, approvals and new destinations are no longer kept or
	// alerted about.
	ActivityOff bool `yaml:"activity_off,omitempty" json:"activity_off"`
	// SecretsPassOff stops the nightly look through workspace files for credentials.
	SecretsPassOff bool `yaml:"secrets_pass_off,omitempty" json:"secrets_pass_off"`
}
