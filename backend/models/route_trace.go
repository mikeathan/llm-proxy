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
}

// The places a hop can sit.
const (
	HopThisNetwork    = "this_network"
	HopProviderShared = "provider_shared"
	HopInternet       = "internet"
)
