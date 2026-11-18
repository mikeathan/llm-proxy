package radio

import (
	"context"
	"encoding/hex"
	"net/http"
	"slices"

	"llm-proxy-stacks/sdk"
)

// The routes that show what the air holds, and the owner's own networks.
const (
	// AirPath answers what is around: other people's devices by class, how long the sensor has listened, the rules in force and what changed.
	AirPath = "/admin/api/radio/air"
	// NetworksPath lists the owner's network names, from the core's one list, and the access points heard broadcasting them (GET). The names
	// and which access points are the owner's are set on the Network page only (ADR-0063).
	NetworksPath = "/admin/api/radio/networks"
)

// AirView is the answer of AirPath. Everything about other people is a count by class: never an address, a tag or a name.
type AirView struct {
	Rules RulesView `json:"rules"`
	// Listened is how many full cycles the sensor listened through over the rules' window.
	Listened int            `json:"listened_cycles"`
	Counts   map[string]int `json:"counts"`
	// Total is how many devices of other people are known.
	Total   int           `json:"total"`
	Changes []ClassChange `json:"changes"`
	// Learned is what the classifier learned in the last cycle: how many of the owner's answers and evidence cases it trained on, and how many
	// owner answers it gives back when each is held out. Sums only: no device.
	Learned *LearnedView `json:"learned,omitempty"`
}

// LearnedView is the learned classifier as the page shows it.
type LearnedView struct {
	OwnerLabels    int   `json:"owner_labels"`
	EvidenceLabels int   `json:"evidence_labels"`
	SelfLabels     int   `json:"self_labels"`
	Agree          int   `json:"agree"`
	Checked        int   `json:"checked"`
	TrainedAt      int64 `json:"trained_at"`
}

// RulesView is the rules in force, as the page shows them.
type RulesView struct {
	Version           int  `json:"version"`
	Provisional       bool `json:"provisional"`
	WindowDays        int  `json:"window_days"`
	MinCyclesListened int  `json:"min_cycles_listened"`
}

// NetworkAP is an access point heard broadcasting one of the owner's network names. Confirmed says the Network page's list confirms it;
// ConfirmedBefore that the owner confirmed it on this page before the lists were one, so it still needs one press on the Network page.
type NetworkAP struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Channel         int    `json:"channel"`
	Security        string `json:"security"`
	Confirmed       bool   `json:"confirmed"`
	ConfirmedBefore bool   `json:"confirmed_before,omitempty"`
	LastSeen        string `json:"last_seen"`
	// Guest is the owner's word that it is a visitors' network (a device on it is no evidence of being the owner's).
	Guest bool `json:"guest"`
}

// NetworksView is the answer of NetworksPath.
type NetworksView struct {
	Names        []string    `json:"names"`
	AccessPoints []NetworkAP `json:"access_points"`
}

func (s *Stack) handleAir(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	rec, store, err := s.recorder(h)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the radio database could not be opened")
	}
	hours := s.settings(ctx, h).TrackHours
	rec.SetTrackHours(hours)
	s.pruneIfDue(ctx, store, hours)
	id := req.Params["id"]
	routes := map[string]func() (stacksdk.Response, error){
		http.MethodGet + " " + AirPath:            func() (stacksdk.Response, error) { return s.air(ctx, store) },
		http.MethodGet + " " + AirDevicesPath:     func() (stacksdk.Response, error) { return s.airDevices(ctx, store, req.Query) },
		http.MethodGet + " " + AirwavesPath:       func() (stacksdk.Response, error) { return s.airwaves(ctx, rec, store) },
		http.MethodGet + " " + LivePath:           func() (stacksdk.Response, error) { return s.live(ctx, h, rec, store) },
		http.MethodGet + " " + TrackPath:          func() (stacksdk.Response, error) { return s.track(ctx, h, rec, store, id) },
		http.MethodGet + " " + KeepPath:           func() (stacksdk.Response, error) { return s.keepView(ctx, h, store) },
		http.MethodPut + " " + KeepPath:           func() (stacksdk.Response, error) { return s.setKeep(ctx, h, rec, store, req.Body) },
		http.MethodDelete + " " + KeepPath:        func() (stacksdk.Response, error) { return s.dropTracks(ctx, h, store) },
		http.MethodPost + " " + CalibratePath:     func() (stacksdk.Response, error) { return s.calibrate(ctx, rec, store, id, req.Body) },
		http.MethodGet + " " + DeviceViewPath:     func() (stacksdk.Response, error) { return s.deviceView(ctx, h, rec, store, id) },
		http.MethodGet + " " + AroundNetworksPath: func() (stacksdk.Response, error) { return s.aroundNetworks(ctx, h, rec, store) },
		http.MethodGet + " " + AskPath:            func() (stacksdk.Response, error) { return s.ask(ctx, h, rec, store) },
		http.MethodPut + " " + KindPath:           func() (stacksdk.Response, error) { return s.setKind(ctx, store, id, req.Body) },
		http.MethodPut + " " + ClassPath:          func() (stacksdk.Response, error) { return s.setClass(ctx, store, id, req.Body) },
		http.MethodPut + " " + GuestPath:          func() (stacksdk.Response, error) { return s.setGuest(ctx, rec, store, id, req.Body) },
		http.MethodPost + " " + MarkMinePath:      func() (stacksdk.Response, error) { return s.markMine(ctx, rec, store, id, req.Body) },
		http.MethodPut + " " + MarkMinePath:       func() (stacksdk.Response, error) { return s.renameMine(ctx, store, id, req.Body) },
		http.MethodGet + " " + NetworksPath:       func() (stacksdk.Response, error) { return s.networks(ctx, rec) },
	}
	if f, ok := routes[req.Method+" "+req.Path]; ok {
		return f()
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

func (s *Stack) air(ctx context.Context, store *Store) (stacksdk.Response, error) {
	now := s.now()
	rules, err := store.CurrentRules(ctx, now)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the rules could not be read")
	}
	since := dayNumber(now) - int64(rules.WindowDays) + 1
	counts, err := store.ClassCounts(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	air, err := store.airDays(ctx, since)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	changes, err := store.RecentClassChanges(ctx, since, 20)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	v := AirView{
		Rules:   RulesView{Version: rules.Version, Provisional: rules.Provisional, WindowDays: rules.WindowDays, MinCyclesListened: rules.MinCyclesListened},
		Counts:  map[string]int{},
		Changes: changes,
	}
	for _, n := range air {
		v.Listened += n
	}
	if m, ok, err := store.LearnedModel(ctx); err == nil && ok {
		v.Learned = &LearnedView{OwnerLabels: m.Owner, EvidenceLabels: m.Evidence, SelfLabels: m.Self, Agree: m.Agree, Checked: m.Checked, TrainedAt: m.TrainedAt}
	}
	for _, c := range Classes {
		v.Counts[string(c)] = 0
	}
	for c, n := range counts {
		if c == "" {
			c = string(ClassUnknown)
		}
		v.Counts[c] += n
		v.Total += n
	}
	return jsonResponse(http.StatusOK, v)
}

func (s *Stack) networks(ctx context.Context, rec *Recorder) (stacksdk.Response, error) {
	_ = rec.SyncOwnNetworks(ctx) // a failed read keeps the last list
	aps, err := rec.OwnAPs(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the networks could not be read")
	}
	names := rec.OwnNames()
	slices.Sort(names)
	v := NetworksView{Names: names, AccessPoints: []NetworkAP{}}
	for _, a := range aps {
		v.AccessPoints = append(v.AccessPoints, NetworkAP{ID: hex.EncodeToString(a.Tag), Name: a.Name, Channel: a.Channel, Security: a.Security, Confirmed: a.Confirmed,
			ConfirmedBefore: a.ConfirmedBefore && !a.Confirmed, LastSeen: dayString(a.LastDay), Guest: a.Guest})
	}
	return jsonResponse(http.StatusOK, v)
}
