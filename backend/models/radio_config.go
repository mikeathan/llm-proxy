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
	// Channels, WindowSeconds and CycleSeconds are the operator's scan plan: the channels a listening run visits, how long it stays on each and the
	// time from one cycle's start to the next. Empty or zero means the stack's default; the stack validates a change and applies its caps.
	Channels      []int `yaml:"channels,omitempty" json:"channels"`
	WindowSeconds int   `yaml:"window_seconds,omitempty" json:"window_seconds"`
	CycleSeconds  int   `yaml:"cycle_seconds,omitempty" json:"cycle_seconds"`
	// TrackHours is how long a per-device signal track is kept on disk: 0 (off, the default), 24 or 48 (ADR-0055). The stack validates it.
	TrackHours int `yaml:"track_hours,omitempty" json:"track_hours"`
}
