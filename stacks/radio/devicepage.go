package radio

import (
	"context"
	"encoding/hex"
	"math"
	"net/http"
	"sort"

	"llm-proxy-stacks/sdk"
)

// The device page, the networks around the house and the owner's answers (ADR-0058). Everything is read from the live picture and the stored
// rows at the time of the request; an answer of the owner's is written to the device's own row.
const (
	// DeviceViewPath answers one device in full (GET).
	DeviceViewPath = "/admin/api/radio/air/device/{id}"
	// AroundNetworksPath answers every network around the house, each router's radios and the devices that use them (GET).
	AroundNetworksPath = "/admin/api/radio/air/networks"
	// AskPath answers the few devices the owner's answer would help most with (GET).
	AskPath = "/admin/api/radio/air/ask"
	// KindPath sets what a device is (PUT {"kind": ...}); ClassPath sets its class, or clears the owner's word with "" (PUT {"class": ...}).
	KindPath  = "/admin/api/radio/air/kind/{id}"
	ClassPath = "/admin/api/radio/air/class/{id}"
	// GuestPath marks one of the owner's access points as a guest network (PUT {"guest": true}).
	GuestPath = "/admin/api/radio/networks/{id}/guest"

	maxGroups       = 200
	maxGroupMembers = 100
	maxUnconnected  = 300
	maxAsk          = 5
	askStrongDBm    = -62
	askUnsure       = 60
	maxDeviceLog    = 10
)

// Member is a device as a line in a list: enough to name it and open it.
type Member struct {
	ID      string       `json:"id"`
	Mine    bool         `json:"mine"`
	Label   string       `json:"label,omitempty"`
	Class   string       `json:"class"`
	Heard   bool         `json:"heard"`
	Dbm     int          `json:"dbm"`
	Network string       `json:"network,omitempty"`
	Channel int          `json:"channel,omitempty"`
	Ident   *DeviceIdent `json:"ident,omitempty"`
	// Reason is why the device is asked about (AskPath only).
	Reason string `json:"reason,omitempty"`
}

func memberOf(d LiveDevice) Member {
	return Member{ID: d.ID, Mine: d.Mine, Label: d.Label, Class: d.Class, Heard: d.Heard && !d.Usual, Dbm: d.Dbm, Network: d.Network, Channel: d.Channel, Ident: d.Ident}
}

// DeviceView is the answer of DeviceViewPath.
type DeviceView struct {
	Device LiveDevice `json:"device"`
	// Chances is the learned model's chance of each class (percent); Factors the numbers that decided it.
	Chances map[string]int `json:"chances"`
	Factors []Factor       `json:"factors"`
	// Uses is the access point the device uses; Radios the other radios of its router; Clients the devices that use it (an access point).
	Uses    *Member       `json:"uses,omitempty"`
	Radios  []Member      `json:"radios"`
	Clients []Member      `json:"clients"`
	History []ClassChange `json:"history"`
	Model   DistanceModel `json:"model"`
	// Evidence is what the sorting read, in numbers the owner can check (after nodeherder's evidence view): "heard in 82% of readings over 6
	// days, at night on 5 of them, moved in 3% of readings".
	Evidence *Evidence `json:"evidence,omitempty"`
	// Listened is how many cycles the sensor has listened through in the window, MinListened how many sorting needs (as on the live view), so
	// the page can say what an unsorted device is waiting for.
	Listened    int `json:"listened_cycles"`
	MinListened int `json:"min_listened_cycles"`
}

// Evidence is a device's behaviour over the window.
type Evidence struct {
	SharePercent int `json:"share_percent"`
	Days         int `json:"days"`
	NightDays    int `json:"night_days"`
	MovePercent  int `json:"move_percent"`
	CyclesHeard  int `json:"cycles_heard"`
	StrongestDBm int `json:"strongest_dbm,omitempty"`
	MeanDBm      int `json:"mean_dbm,omitempty"`
}

func evidenceOf(f Features, r FeatureRow) *Evidence {
	e := &Evidence{SharePercent: int(math.Round(100 * f.Share)), Days: r.Days, NightDays: r.NightDays, MovePercent: int(math.Round(100 * f.MoveShare)), CyclesHeard: r.Cycles}
	if f.HasSignal {
		e.StrongestDBm, e.MeanDBm = f.SignalMax, int(math.Round(f.SignalMean))
	}
	return e
}

// NetworkGroup is one router (or one access point with no box) and the devices on it.
type NetworkGroup struct {
	ID        string   `json:"id"`
	Yours     bool     `json:"yours"`
	Names     []string `json:"names"`
	Radios    []Member `json:"radios"`
	Clients   []Member `json:"clients"`
	Strongest int      `json:"strongest_dbm"`
	Heard     bool     `json:"heard"`
}

// AroundNetworks is the answer of AroundNetworksPath.
type AroundNetworks struct {
	Groups      []NetworkGroup `json:"groups"`
	Unconnected []Member       `json:"unconnected"`
	Total       int            `json:"total"`
}

// AskView is the answer of AskPath.
type AskView struct {
	Devices []Member `json:"devices"`
	Learned Model    `json:"learned"`
}

// dotsByID is the live picture by short id; a short id two devices share is left out, because it cannot name either.
func dotsByID(v LiveView) map[string]LiveDevice {
	out := map[string]LiveDevice{}
	twice := map[string]bool{}
	for _, d := range v.Devices {
		if _, has := out[d.ID]; has {
			twice[d.ID] = true
		}
		out[d.ID] = d
	}
	for id := range twice {
		delete(out, id)
	}
	return out
}

func (s *Stack) deviceView(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store, id string) (stacksdk.Response, error) {
	v, set, err := s.liveView(ctx, h, rec, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	dots := dotsByID(v)
	d, ok := dots[id]
	if !ok {
		return plainError(http.StatusNotFound, "that device is not in the picture: it has not been heard in the last few days")
	}
	row, tag := rowByShort(set, id)
	out := DeviceView{Device: d, Chances: map[string]int{}, Factors: []Factor{}, Radios: []Member{}, Clients: []Member{}, History: []ClassChange{}, Model: v.Model,
		Listened: v.Listened, MinListened: v.MinListened}
	if tag != nil {
		s.explain(set, row, &out)
		out.History, _ = store.DeviceClassLog(ctx, tag, maxDeviceLog)
		relatives(set, dots, row, &out)
	}
	return jsonResponse(http.StatusOK, out)
}

// rowByShort finds the stored row of a short id, when exactly one has it.
func rowByShort(set liveSet, id string) (FeatureRow, []byte) {
	r, ok := set.byShort[id]
	if !ok {
		return FeatureRow{}, nil
	}
	return r, r.Tag
}

// explain fills the model's chances and the numbers behind the class.
func (s *Stack) explain(set liveSet, row FeatureRow, out *DeviceView) {
	f := FeaturesOf(row, set.listened)
	out.Evidence = evidenceOf(f, row)
	vec := ownedVector(vectorOf(set.rules, f, KindOf(row, set.ident.votes).Kind, signalOffset(set.rows)), row)
	m := s.model(set)
	for c, p := range m.Posterior(vec) {
		out.Chances[string(c)] = int(math.Round(100 * p))
	}
	if c := Class(row.Class); c != "" && c != ClassUnknown {
		out.Factors = m.Explain(vec, c)
	}
}

// model is the model the last cycle decided with (kept by ClassifyAllWith), so the page explains a class with what decided it; before the first
// cycle has trained one, the same training is done here.
func (s *Stack) model(set liveSet) *Model {
	if st, err := s.existingStore(); err == nil && st != nil {
		if m, ok, err := st.LearnedModel(context.Background()); err == nil && ok && len(m.Stats) == len(learnClasses) && m.Version == set.rules.Version {
			return &m
		}
	}
	m, _ := s.learned(set)
	return m
}

// learned trains the model on the set (cheap: a few hundred devices).
func (s *Stack) learned(set liveSet) (*Model, bool) {
	var examples []example
	offset := signalOffset(set.rows)
	for _, r := range set.rows {
		f := FeaturesOf(r, set.listened)
		if e, ok := labelOf(set.rules, r, f); ok {
			e.v = vectorOf(set.rules, f, KindOf(r, set.ident.votes).Kind, offset)
			examples = append(examples, e)
		}
	}
	return Train(set.rules, examples, 0), len(examples) > 0
}

// relatives fills the access point a device uses, the other radios of its router and the devices that use it.
func relatives(set liveSet, dots map[string]LiveDevice, row FeatureRow, out *DeviceView) {
	for _, r := range set.rows {
		d, heard := dots[shortID(r.Tag)]
		if !heard || string(r.Tag) == string(row.Tag) {
			continue
		}
		relate(set, row, r, memberOf(d), out)
	}
	sortMembers(out.Radios)
	sortMembers(out.Clients)
}

// relate puts one other device where it belongs on a device's page: the access point it uses, a radio of its router, or one of its clients.
func relate(set liveSet, row, other FeatureRow, m Member, out *DeviceView) {
	switch {
	case same(row.Net, other.Tag):
		out.Uses = &m
	case same(row.Box, other.Box) && len(out.Radios) < maxGroupMembers:
		out.Radios = append(out.Radios, m)
	}
	if len(other.Net) > 0 && len(out.Clients) < maxGroupMembers && (same(other.Net, row.Tag) || same(row.Box, []byte(boxOfNet(set, other.Net)))) {
		out.Clients = append(out.Clients, m)
	}
}

// same is two tags that are set and equal.
func same(a, b []byte) bool { return len(a) > 0 && string(a) == string(b) }

func boxOfNet(set liveSet, net []byte) string {
	if r, ok := set.byTag[string(net)]; ok {
		return string(r.Box)
	}
	return ""
}

func sortMembers(list []Member) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Heard != list[j].Heard {
			return list[i].Heard
		}
		return list[i].Dbm > list[j].Dbm
	})
}

// groupKey is the router a device belongs to as an access point: its box, or itself when it has none.
func groupKey(r FeatureRow) string {
	if len(r.Box) > 0 {
		return "b" + string(r.Box)
	}
	return "a" + string(r.Tag)
}

func isAccessPoint(r FeatureRow, clients map[string]int) bool {
	return r.WifiGen > 0 || r.OwnAP || clients[string(r.Tag)] > 0
}

func (s *Stack) aroundNetworks(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store) (stacksdk.Response, error) {
	v, set, err := s.liveView(ctx, h, rec, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	dots := dotsByID(v)
	groups := map[string]*NetworkGroup{}
	apGroup := map[string]string{} // access point tag to its group
	out := AroundNetworks{Groups: []NetworkGroup{}, Unconnected: []Member{}}
	gatherRouters(set, dots, groups, apGroup)
	for _, r := range set.rows {
		d, ok := dots[shortID(r.Tag)]
		if !ok || apGroup[string(r.Tag)] != "" {
			continue
		}
		out.Total++
		placeClient(&out, groups[apGroup[string(r.Net)]], memberOf(d))
	}
	out.Groups = orderGroups(groups)
	sortMembers(out.Unconnected)
	return jsonResponse(http.StatusOK, out)
}

// gatherRouters makes a group for each router heard in the window and puts its radios in it.
func gatherRouters(set liveSet, dots map[string]LiveDevice, groups map[string]*NetworkGroup, apGroup map[string]string) {
	for _, r := range set.rows {
		d, ok := dots[shortID(r.Tag)]
		if !ok || !isAccessPoint(r, set.ident.clients) {
			continue
		}
		key := groupKey(r)
		g := groups[key]
		if g == nil {
			g = &NetworkGroup{ID: shortID([]byte(key[1:])), Names: []string{}, Radios: []Member{}, Clients: []Member{}, Strongest: d.Dbm}
			groups[key] = g
		}
		addRadio(g, d, r)
		apGroup[string(r.Tag)] = key
	}
}

// placeClient puts a device under the router it uses, or with the unconnected ones.
func placeClient(out *AroundNetworks, g *NetworkGroup, m Member) {
	switch {
	case g != nil && len(g.Clients) < maxGroupMembers:
		g.Clients = append(g.Clients, m)
	case g == nil && len(out.Unconnected) < maxUnconnected:
		out.Unconnected = append(out.Unconnected, m)
	}
}

func addRadio(g *NetworkGroup, d LiveDevice, r FeatureRow) {
	if len(g.Radios) < maxGroupMembers {
		g.Radios = append(g.Radios, memberOf(d))
	}
	g.Yours = g.Yours || r.OwnAP
	g.Heard = g.Heard || (d.Heard && !d.Usual)
	g.Strongest = max(g.Strongest, d.Dbm)
	if d.Network != "" && !containsString(g.Names, d.Network) {
		g.Names = append(g.Names, d.Network)
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// orderGroups puts the owner's routers first, then the ones heard now, then the strongest.
func orderGroups(groups map[string]*NetworkGroup) []NetworkGroup {
	out := make([]NetworkGroup, 0, len(groups))
	for _, g := range groups {
		sortMembers(g.Radios)
		sortMembers(g.Clients)
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Yours != b.Yours {
			return a.Yours
		}
		if a.Heard != b.Heard {
			return a.Heard
		}
		if a.Strongest != b.Strongest {
			return a.Strongest > b.Strongest
		}
		return a.ID < b.ID
	})
	return out[:min(len(out), maxGroups)]
}

// ask picks the few devices an answer would help most with: first those the evidence says are probably the owner's, then strong ones the
// classifier is least sure of. A device the owner already answered for is never asked about again.
func (s *Stack) ask(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store) (stacksdk.Response, error) {
	v, set, err := s.liveView(ctx, h, rec, store)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the air could not be read")
	}
	var yours, unsure []Member
	for _, d := range v.Devices {
		row, ok := set.byShort[d.ID]
		if !ok || !askable(d, row) {
			continue
		}
		m := memberOf(d)
		switch m.Reason = askReason(d); m.Reason {
		case "likely_yours":
			yours = append(yours, m)
		case "close_and_unsure":
			unsure = append(unsure, m)
		}
	}
	sortMembers(yours)
	sortMembers(unsure)
	out := AskView{Devices: append(yours, unsure...)}
	out.Devices = out.Devices[:min(len(out.Devices), maxAsk)]
	if out.Devices == nil {
		out.Devices = []Member{}
	}
	if m, ok, err := store.LearnedModel(ctx); err == nil && ok {
		out.Learned = m
	}
	return jsonResponse(http.StatusOK, out)
}

// answered says whether the owner has given a class or a kind for the device.
// askable: heard now, not the owner's, not a confirmed access point, and not already answered.
func askable(d LiveDevice, row FeatureRow) bool {
	return !d.Mine && !d.Usual && d.Ident != nil && !d.Ident.YourAP && !answered(row)
}

// askReason is why the owner's answer would help with a device, "" when it would not much.
func askReason(d LiveDevice) string {
	switch {
	case d.Ident.LikelyYours():
		return "likely_yours"
	case d.Dbm >= askStrongDBm && (d.Class == string(ClassUnknown) || d.Ident.Confidence < askUnsure):
		return "close_and_unsure"
	}
	return ""
}

func answered(r FeatureRow) bool {
	return (r.Class != "" && r.ClassVer == 0) || r.KindBy == kindByOwner
}

// uniqueTag is the one stored device a short id names; a short id two devices share names neither.
func uniqueTag(ctx context.Context, store *Store, id string) ([]byte, int, string) {
	if b, err := hex.DecodeString(id); err != nil || len(b) != shortIDBytes {
		return nil, http.StatusNotFound, "no such device"
	}
	tags, err := store.TagsByShortID(ctx, id)
	switch {
	case err != nil:
		return nil, http.StatusInternalServerError, "the device could not be read"
	case len(tags) == 0:
		return nil, http.StatusNotFound, "that device is no longer known"
	case len(tags) > 1:
		return nil, http.StatusConflict, "two devices share that short id; wait for the next reading"
	}
	return tags[0], 0, ""
}

func (s *Stack) setKind(ctx context.Context, store *Store, id string, body []byte) (stacksdk.Response, error) {
	var b struct {
		Kind string `json:"kind"`
	}
	if err := strictDecode(body, &b); err != nil || !validKind(DeviceKind(b.Kind)) {
		return plainError(http.StatusBadRequest, "say what the device is, from the list")
	}
	tag, status, msg := uniqueTag(ctx, store, id)
	if tag == nil {
		return plainError(status, msg)
	}
	by := kindByOwner
	if DeviceKind(b.Kind) == DeviceUnknown {
		by = "" // "I don't know" hands the kind back to the evidence
	}
	if _, err := store.SetKind(ctx, tag, DeviceKind(b.Kind), by); err != nil {
		return plainError(http.StatusInternalServerError, "the answer could not be saved")
	}
	return jsonResponse(http.StatusOK, map[string]string{"kind": b.Kind})
}

func (s *Stack) setClass(ctx context.Context, store *Store, id string, body []byte) (stacksdk.Response, error) {
	var b struct {
		Class string `json:"class"`
	}
	if err := strictDecode(body, &b); err != nil || (b.Class != "" && b.Class != notMine && !validClass(Class(b.Class))) {
		return plainError(http.StatusBadRequest, "give a class from the list, or nothing to let the classifier decide")
	}
	tag, status, msg := uniqueTag(ctx, store, id)
	if tag == nil {
		return plainError(status, msg)
	}
	if b.Class == notMine {
		b.Class = string(s.notMineClass(ctx, store, tag))
	}
	if err := store.SetOwnerClass(ctx, tag, Class(b.Class), s.now()); err != nil {
		return plainError(http.StatusInternalServerError, "the answer could not be saved")
	}
	return jsonResponse(http.StatusOK, map[string]string{"class": b.Class})
}

// notMine is the owner's "not mine" without saying whose: a neighbour's or a passer-by. Recording every such answer as a neighbour's would teach
// the classifier that passers-by are neighbours, so the server picks whichever of the two the current model rates higher for the device.
const notMine = "not_mine"

func (s *Stack) notMineClass(ctx context.Context, store *Store, tag []byte) Class {
	rules, rows, listened, err := s.airRows(ctx, store)
	if err != nil {
		return ClassNeighbour
	}
	set := newLiveSet(rules, rows, listened, nil)
	row, ok := set.byTag[string(tag)]
	if !ok {
		return ClassNeighbour
	}
	p := s.model(set).Posterior(vectorOf(rules, FeaturesOf(row, listened), KindOf(row, set.ident.votes).Kind, signalOffset(rows)))
	if p[ClassTransient] > p[ClassNeighbour] {
		return ClassTransient
	}
	return ClassNeighbour
}

func validClass(c Class) bool {
	for _, k := range Classes {
		if k == c && c != ClassUnknown {
			return true
		}
	}
	return false
}

func (s *Stack) setGuest(ctx context.Context, rec *Recorder, store *Store, id string, body []byte) (stacksdk.Response, error) {
	var b struct {
		Guest bool `json:"guest"`
	}
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "say whether it is a guest network")
	}
	tag, err := hex.DecodeString(id)
	if err != nil || len(tag) == 0 {
		return plainError(http.StatusNotFound, "no such access point")
	}
	ok, err := store.SetOwnAPGuest(ctx, tag, b.Guest)
	switch {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the access point could not be saved")
	case !ok:
		return plainError(http.StatusNotFound, "no such access point")
	}
	if err := rec.RefreshOwnAPs(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the access point could not be saved")
	}
	return jsonResponse(http.StatusOK, map[string]bool{"guest": b.Guest})
}
