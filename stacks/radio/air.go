package radio

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode/utf8"

	"llm-proxy-stacks/sdk"
)

// The routes that show what the air holds, and the owner's own networks.
const (
	// AirPath answers what is around: other people's devices by class, how long the sensor has listened, the rules in force and what changed.
	AirPath = "/admin/api/radio/air"
	// NetworksPath lists the owner's network names and the access points that broadcast them (GET), and sets the names (PUT).
	NetworksPath = "/admin/api/radio/networks"
	// NetworkConfirmPath confirms one proposed access point as the owner's (POST); NetworkPath says it is not theirs (DELETE).
	NetworkConfirmPath = "/admin/api/radio/networks/{id}/confirm"
	NetworkPath        = "/admin/api/radio/networks/{id}"

	maxNetworkNames = 8
	maxNetworkName  = 32 // an SSID is at most 32 bytes
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
}

// RulesView is the rules in force, as the page shows them.
type RulesView struct {
	Version           int  `json:"version"`
	Provisional       bool `json:"provisional"`
	WindowDays        int  `json:"window_days"`
	MinCyclesListened int  `json:"min_cycles_listened"`
}

// NetworkAP is an access point that broadcasts one of the owner's network names. It is a proposal until confirmed.
type NetworkAP struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Channel   int    `json:"channel"`
	Security  string `json:"security"`
	Confirmed bool   `json:"confirmed"`
	LastSeen  string `json:"last_seen"`
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
	id := req.Params["id"]
	routes := map[string]func() (stacksdk.Response, error){
		http.MethodGet + " " + AirPath:             func() (stacksdk.Response, error) { return s.air(ctx, store) },
		http.MethodGet + " " + AirDevicesPath:      func() (stacksdk.Response, error) { return s.airDevices(ctx, store, req.Query) },
		http.MethodGet + " " + AirwavesPath:        func() (stacksdk.Response, error) { return s.airwaves(ctx, store) },
		http.MethodGet + " " + NetworksPath:        func() (stacksdk.Response, error) { return s.networks(ctx, h, store) },
		http.MethodPut + " " + NetworksPath:        func() (stacksdk.Response, error) { return s.setNetworkNames(ctx, h, rec, req.Body) },
		http.MethodPost + " " + NetworkConfirmPath: func() (stacksdk.Response, error) { return s.confirmNetwork(ctx, rec, store, id) },
		http.MethodDelete + " " + NetworkPath:      func() (stacksdk.Response, error) { return s.dropNetwork(ctx, rec, store, id) },
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

func (s *Stack) networks(ctx context.Context, h stacksdk.Host, store *Store) (stacksdk.Response, error) {
	aps, err := store.OwnAPs(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the networks could not be read")
	}
	v := NetworksView{Names: append([]string{}, s.settings(ctx, h).OwnNetworks...), AccessPoints: []NetworkAP{}}
	for _, a := range aps {
		v.AccessPoints = append(v.AccessPoints, NetworkAP{ID: hex.EncodeToString(a.Tag), Name: a.Name, Channel: a.Channel, Security: a.Security, Confirmed: a.Confirmed, LastSeen: dayString(a.LastDay)})
	}
	return jsonResponse(http.StatusOK, v)
}

type namesBody struct {
	Names []string `json:"names"`
}

func (s *Stack) setNetworkNames(ctx context.Context, h stacksdk.Host, rec *Recorder, body []byte) (stacksdk.Response, error) {
	var b namesBody
	if err := strictDecode(body, &b); err != nil || len(b.Names) > maxNetworkNames {
		return plainError(http.StatusBadRequest, "give up to 8 network names")
	}
	names := make([]string, 0, len(b.Names))
	for _, n := range b.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if len(n) > maxNetworkName || !utf8.ValidString(n) || strings.ContainsFunc(n, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return plainError(http.StatusBadRequest, "a network name is at most 32 bytes of plain text")
		}
		names = append(names, n)
	}
	if h.Config == nil {
		return plainError(http.StatusConflict, "the settings are not available to the Radio stack")
	}
	doc, _ := jsonBytes(map[string]any{"own_networks": names})
	if err := h.Config.Save(ctx, doc); err != nil {
		return plainError(http.StatusInternalServerError, "the names could not be saved")
	}
	rec.SetOwnNetworks(names)
	return jsonResponse(http.StatusOK, map[string]any{"names": names})
}

func (s *Stack) confirmNetwork(ctx context.Context, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	tag, err := hex.DecodeString(id)
	if err != nil || len(tag) == 0 {
		return plainError(http.StatusNotFound, "no such access point")
	}
	ok, err := store.SetOwnAPConfirmed(ctx, tag, true)
	switch {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the access point could not be confirmed")
	case !ok:
		return plainError(http.StatusNotFound, "no such access point")
	}
	if err := rec.RefreshOwnAPs(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the access point could not be confirmed")
	}
	return jsonResponse(http.StatusOK, map[string]bool{"confirmed": true})
}

func (s *Stack) dropNetwork(ctx context.Context, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	tag, err := hex.DecodeString(id)
	if err != nil || len(tag) == 0 {
		return plainError(http.StatusNotFound, "no such access point")
	}
	ok, err := store.DeleteOwnAP(ctx, tag)
	switch {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the access point could not be removed")
	case !ok:
		return plainError(http.StatusNotFound, "no such access point")
	}
	if err := rec.RefreshOwnAPs(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the access point could not be removed")
	}
	return jsonResponse(http.StatusOK, map[string]bool{"removed": true})
}
