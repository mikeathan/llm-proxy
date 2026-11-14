package models

// RadioConfig is the host-wide policy of the Radio stack (settings.yml → radio:). Like AgentsConfig it is not part of UserSettings, so
// saving the ordinary settings form cannot overwrite it. Unlike the Agents parts it is off until the operator switches it on: it is a
// sensor for the radio around the computer, and an install that never opted in listens to nothing.
type RadioConfig struct {
	// Enabled is the master switch of the Radio stack (module radio_sensors).
	Enabled bool `yaml:"enabled,omitempty" json:"enabled"`
	// OwnNetworks are the names the operator's own Wi-Fi networks go by. A beacon with one of these names only proposes an access point: the
	// operator confirms which addresses are theirs, because anyone can broadcast the same name.
	OwnNetworks []string `yaml:"own_networks,omitempty" json:"own_networks"`
}
