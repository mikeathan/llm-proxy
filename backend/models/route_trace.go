package models

import "time"

// RouteTrace is one trace of the route from this computer to an internet address (Constitution I.14, plan network-traffic-view-redesign): the
// routers that answered on the way, each with what the tables kept on this computer say about it. It is kept in memory only, is never a finding,
// and stays out of the audit report and every prompt.
type RouteTrace struct {
	Address string `json:"address"`
	// State is running, done or failed (fixed words); Error is fixed words too, set when failed.
	State     string     `json:"state"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	// Reached says the address itself answered; a server that ignores the probes is not reached even when the route got there.
	Reached bool       `json:"reached"`
	Hops    []RouteHop `json:"hops"`
	// Repeat says the operator asked for the route to be probed again and again after the first pass; Rounds is how many rounds that is
	// and RoundsDone how many have finished.
	Repeat     bool `json:"repeat,omitempty"`
	Rounds     int  `json:"rounds,omitempty"`
	RoundsDone int  `json:"rounds_done,omitempty"`
}

// The states of a RouteTrace.
const (
	RouteRunning = "running"
	RouteDone    = "done"
	RouteFailed  = "failed"
)

// RouteHop is one step of a route: the router that said the time ran out at this distance (empty Address when none answered), how many of the
// probes it answered, the quickest answer, and where the address sits (a fixed word: this_network, provider_shared or internet).
type RouteHop struct {
	TTL      int           `json:"ttl"`
	Address  string        `json:"address,omitempty"`
	Sent     int           `json:"sent"`
	Answered int           `json:"answered"`
	BestMs   float64       `json:"best_ms,omitempty"`
	Where    string        `json:"where,omitempty"`
	Reached  bool          `json:"reached,omitempty"`
	Owner    *TrafficOwner `json:"owner,omitempty"`
	// Stats is what the repeated rounds measured at this step; absent on a plain trace.
	Stats *RouteHopStats `json:"stats,omitempty"`
}

// RouteHopStats is how one router answered over the repeated rounds: probes sent and answered, and the quickest, average and slowest answers.
type RouteHopStats struct {
	Sent     int     `json:"sent"`
	Answered int     `json:"answered"`
	BestMs   float64 `json:"best_ms,omitempty"`
	AvgMs    float64 `json:"avg_ms,omitempty"`
	WorstMs  float64 `json:"worst_ms,omitempty"`
}

// The places a hop can sit.
const (
	HopThisNetwork    = "this_network"
	HopProviderShared = "provider_shared"
	HopInternet       = "internet"
)

// RouterConnections is one read of the router's connection table (Constitution I.15): which device on the network held a connection to which
// place on the internet. It is kept in memory only, never a finding, and stays out of every prompt, alert and log. It has no byte counts: the
// router's accounting is off by default, and a line that carries none shows none.
type RouterConnections struct {
	At time.Time `json:"at"`
	// Lines is how many lines the router's table held; Kept how many were connections from a device to the internet; Truncated says the read
	// stopped at the line limit.
	Lines     int  `json:"lines"`
	Kept      int  `json:"kept"`
	Truncated bool `json:"truncated,omitempty"`
	// Devices are the devices that held a connection, the busiest first.
	Devices []RouterDevice `json:"devices"`
}

// RouterDevice is one device's connections, grouped by place, with how many connections each place held.
type RouterDevice struct {
	// Address is the device's address on this network, as the table records it.
	Address string        `json:"address"`
	Total   int           `json:"total"`
	Places  []RouterPlace `json:"places"`
}

// RouterPlace is one internet address, port and protocol a device held connections to, with what the tables kept on this computer say of it.
type RouterPlace struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Proto   string `json:"proto"`
	// State is the state of the first such connection (tcp only): ESTABLISHED, TIME_WAIT and so on, as the router names it.
	State string        `json:"state,omitempty"`
	Count int           `json:"count"`
	Owner *TrafficOwner `json:"owner,omitempty"`
	// New says the company or the country of this place was first seen for the device in the last day, after the device's first day of
	// history. It is set only while the router watch keeps a history (Constitution I.15).
	New bool `json:"new,omitempty"`
}
