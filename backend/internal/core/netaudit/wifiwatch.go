package netaudit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"llm-proxy/internal/core/netaudit/wifilook"
	"llm-proxy/models"
)

// The Wi-Fi look-alike watch: every few minutes it reads the access points this computer's NetworkManager already knows
// (core/netaudit/wifilook, a guarded nmcli that cannot use the network) and tells the operator, in fixed words, of one that advertises
// the operator's own network name from a hardware address the operator did not confirm: what an evil twin looks like. Only the operator's
// own names are judged; the neighbours' networks are counted nowhere and kept nowhere. Constitution II.3 (proposed), ADR-0031.
const (
	// wifiWatchEvery is how often it looks; a look is one 30 ms program run reading a list NetworkManager already holds.
	wifiWatchEvery = 10 * time.Minute
	// wifiFirstLook is how long after start-up the first look is made.
	wifiFirstLook = time.Minute
	// wifiAlertGap is the least time between two messages about the same hardware address.
	wifiAlertGap = 6 * time.Hour
	// wifiRecent is how long a look-alike stays in the nightly report after it was last seen.
	wifiRecent = 24 * time.Hour
	// maxWifiRecent bounds the look-alikes kept, whatever a hostile neighbourhood advertises; maxWifiSeen bounds the addresses of the
	// operator's names kept for confirmation; maxWifiNetworks and maxWifiAddresses bound what the operator saves.
	maxWifiRecent    = 32
	maxWifiSeen      = 64
	maxWifiNetworks  = 8
	maxWifiAddresses = 32
	// maxWifiListed is how many addresses one message names.
	maxWifiListed = 3

	CheckWifiLookalike = "wifi_lookalike"
)

func init() {
	checkTitles[CheckWifiLookalike] = "A Wi-Fi network is using your network's name"
	remediations[CheckWifiLookalike] = "An access point you have not confirmed is advertising your network's name. If you added one (a mesh node, an extender, a new router), confirm it in the Wi-Fi look-alikes card. If you did not, someone may be copying your network's name to catch the devices that join it: do not join it, check your router's list of access points, and change the Wi-Fi password if a device has already connected to it."
	guides[CheckWifiLookalike] = FixGuide{
		Steps: []string{
			"Open the Wi-Fi look-alikes card on the Settings tab and look at the hardware address the finding names.",
			"If you recently added an access point (a mesh node, an extender, a replacement router), confirm its address there.",
			"If not, look at your router's list of wireless access points and connected devices for the address, and check whether any of your devices joined it.",
			"If a device did join it, change the Wi-Fi password, and forget and re-join the network on your devices.",
		},
		Risk: "Changing the Wi-Fi password disconnects every device until it is given the new one.",
		Undo: "Set the old Wi-Fi password again, or accept this finding to say it is understood.",
	}
}

// WifiConfig reads and changes the look's switch and the operator's network names with the addresses they confirmed.
type WifiConfig struct{ store PolicyStore }

func NewWifiConfig(store PolicyStore) *WifiConfig { return &WifiConfig{store: store} }

// Enabled reports whether the look is switched on (it is off until the operator turns it on).
func (c *WifiConfig) Enabled() bool { return c.store.Get().WifiLookalikesEnabled }

// Set switches the look on or off.
func (c *WifiConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error { cfg.WifiLookalikesEnabled = enabled; return nil }); err != nil {
		return fmt.Errorf("failed to save the Wi-Fi look-alikes setting: %w", err)
	}
	return nil
}

// Networks returns a copy of the operator's network names and confirmed addresses.
func (c *WifiConfig) Networks() []models.WifiNetwork {
	src := c.store.Get().WifiNetworks
	out := make([]models.WifiNetwork, len(src))
	for i, n := range src {
		out[i] = models.WifiNetwork{Name: n.Name, Addresses: slices.Clone(n.Addresses)}
	}
	return out
}

// validWifiName says whether text can be a network name: 1 to 32 bytes of UTF-8 with no control character, as 802.11 allows.
func validWifiName(name string) bool {
	if strings.TrimSpace(name) == "" || len(name) > wifilook.MaxSSID || !utf8.ValidString(name) {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// AddNetwork adds one of the operator's own network names, with no address confirmed yet.
func (c *WifiConfig) AddNetwork(name string) error {
	if !validWifiName(name) {
		return fmt.Errorf("a Wi-Fi network name is 1 to %d characters with no control character", wifilook.MaxSSID)
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		if len(cfg.WifiNetworks) >= maxWifiNetworks {
			return fmt.Errorf("at most %d Wi-Fi network names can be watched", maxWifiNetworks)
		}
		if slices.ContainsFunc(cfg.WifiNetworks, func(n models.WifiNetwork) bool { return n.Name == name }) {
			return fmt.Errorf("%q is already watched", name)
		}
		cfg.WifiNetworks = append(cfg.WifiNetworks, models.WifiNetwork{Name: name})
		return nil
	})
}

// RemoveNetwork stops watching a name and forgets the addresses confirmed for it.
func (c *WifiConfig) RemoveNetwork(name string) error {
	return c.update(func(cfg *models.NetauditConfig) error {
		i := slices.IndexFunc(cfg.WifiNetworks, func(n models.WifiNetwork) bool { return n.Name == name })
		if i < 0 {
			return fmt.Errorf("%q is not watched", name)
		}
		cfg.WifiNetworks = slices.Delete(slices.Clone(cfg.WifiNetworks), i, i+1)
		return nil
	})
}

// confirm adds addresses (already checked by the watch) to a network's confirmed set.
func (c *WifiConfig) confirm(name string, addrs []string) error {
	return c.update(func(cfg *models.NetauditConfig) error {
		nets := slices.Clone(cfg.WifiNetworks)
		i := slices.IndexFunc(nets, func(n models.WifiNetwork) bool { return n.Name == name })
		if i < 0 {
			return fmt.Errorf("%q is not watched", name)
		}
		merged := slices.Clone(nets[i].Addresses)
		for _, a := range addrs {
			if !slices.Contains(merged, a) {
				merged = append(merged, a)
			}
		}
		if len(merged) > maxWifiAddresses {
			return fmt.Errorf("at most %d access points can be confirmed for one network", maxWifiAddresses)
		}
		nets[i].Addresses = merged
		cfg.WifiNetworks = nets
		return nil
	})
}

func (c *WifiConfig) update(f func(*models.NetauditConfig) error) error {
	var userErr error
	if err := c.store.Update(func(cfg *models.NetauditConfig) error { userErr = f(cfg); return userErr }); err != nil {
		if userErr != nil {
			return userErr
		}
		return fmt.Errorf("failed to save the Wi-Fi look-alikes settings: %w", err)
	}
	return nil
}

// WifiWatchDeps are what the watch needs. List reads the access points (a guarded nmcli); Notify sends one message through the alert log
// and fails when it could not be delivered, so it is tried again at the next look.
type WifiWatchDeps struct {
	List    func(ctx context.Context) ([]wifilook.AP, error)
	Config  *WifiConfig
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
	Now     func() time.Time
}

// WifiWatch keeps, in memory, the addresses the last look saw advertising the operator's names (for confirming), the look-alikes of
// the last day (for the nightly report) and when each was told. All of it is bounded.
type WifiWatch struct {
	deps WifiWatchDeps

	mu       sync.Mutex
	seen     []wifilook.AP // the last look's access points that use one of the operator's names
	lastLook time.Time
	lastGood time.Time
	failed   bool
	lastErr  string
	recent   map[string]recentLookalike
	told     map[string]time.Time
}

type recentLookalike struct {
	look wifilook.Lookalike
	at   time.Time
}

func NewWifiWatch(deps WifiWatchDeps) *WifiWatch {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &WifiWatch{deps: deps, recent: map[string]recentLookalike{}, told: map[string]time.Time{}}
}

// Run looks a minute after start-up and then every ten minutes until ctx ends.
func (w *WifiWatch) Run(ctx context.Context) {
	timer := time.NewTimer(wifiFirstLook)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.Tick(ctx)
			timer.Reset(wifiWatchEvery)
		}
	}
}

func networksOf(cfg []models.WifiNetwork) []wifilook.Network {
	out := make([]wifilook.Network, len(cfg))
	for i, n := range cfg {
		out[i] = wifilook.Network{Name: n.Name, Addresses: n.Addresses}
	}
	return out
}

// Tick looks once, if the watch is on and the operator named a network.
func (w *WifiWatch) Tick(ctx context.Context) {
	if !w.deps.Config.Enabled() || len(w.deps.Config.Networks()) == 0 {
		return
	}
	if aps, ok := w.look(ctx); ok {
		w.tell(ctx, aps)
	}
}

// Look makes one look at the operator's request, whether the watch is on or not, so the operator can see which addresses advertise a
// name and confirm them. It reads the same list and changes nothing.
func (w *WifiWatch) Look(ctx context.Context) error {
	if _, ok := w.look(ctx); !ok {
		w.mu.Lock()
		defer w.mu.Unlock()
		return errors.New(w.lastErr)
	}
	return nil
}

// look reads the access points and records what concerns the operator's names. A look that fails is a failed part, said once; it is
// never read as "no look-alikes".
func (w *WifiWatch) look(ctx context.Context) ([]wifilook.AP, bool) {
	now := w.deps.Now()
	aps, err := w.deps.List(ctx)
	if err != nil {
		w.mu.Lock()
		w.failed = true
		same := w.lastErr == err.Error()
		w.lastErr = err.Error()
		w.mu.Unlock()
		if !same && w.deps.OnError != nil {
			w.deps.OnError(err)
		}
		return nil, false
	}
	nets := networksOf(w.deps.Config.Networks())
	names := map[string]bool{}
	for _, n := range nets {
		names[n.Name] = true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failed, w.lastErr, w.lastGood, w.lastLook = false, "", now, now
	w.seen = w.seen[:0:0]
	for _, a := range aps {
		if a.SSID != "" && names[a.SSID] && len(w.seen) < maxWifiSeen {
			w.seen = append(w.seen, a)
		}
	}
	for _, l := range wifilook.Lookalikes(aps, nets) {
		w.recent[l.BSSID] = recentLookalike{look: l, at: now}
	}
	w.prune(now)
	return aps, true
}

// prune drops what is older than a day and, past the bound, the oldest.
func (w *WifiWatch) prune(now time.Time) {
	for k, r := range w.recent {
		if now.Sub(r.at) > wifiRecent {
			delete(w.recent, k)
		}
	}
	for len(w.recent) > maxWifiRecent {
		oldest, at := "", now
		for k, r := range w.recent {
			if oldest == "" || r.at.Before(at) {
				oldest, at = k, r.at
			}
		}
		delete(w.recent, oldest)
	}
	for k, at := range w.told {
		if now.Sub(at) > wifiAlertGap {
			delete(w.told, k)
		}
	}
}

// tell sends one message naming the look-alikes of this look that were not told within the gap, and marks them told once it was delivered.
func (w *WifiWatch) tell(ctx context.Context, aps []wifilook.AP) {
	now := w.deps.Now()
	var fresh []wifilook.Lookalike
	w.mu.Lock()
	for _, l := range wifilook.Lookalikes(aps, networksOf(w.deps.Config.Networks())) {
		if at, ok := w.told[l.BSSID]; !ok || now.Sub(at) >= wifiAlertGap {
			fresh = append(fresh, l)
		}
	}
	w.mu.Unlock()
	if len(fresh) == 0 || ctx.Err() != nil {
		return
	}
	if err := w.deps.Notify(ctx, wifiMessage(fresh)); err != nil {
		if w.deps.OnError != nil {
			w.deps.OnError(fmt.Errorf("failed to send the Wi-Fi look-alike alert: %w", err))
		}
		return
	}
	w.mu.Lock()
	for _, l := range fresh {
		w.told[l.BSSID] = now
	}
	w.mu.Unlock()
}

// wifiMessage is the alert in fixed words: the operator's own name for the network and up to three hardware addresses, which are hex.
func wifiMessage(fresh []wifilook.Lookalike) string {
	addrs := make([]string, 0, maxWifiListed)
	for _, l := range fresh[:min(len(fresh), maxWifiListed)] {
		addrs = append(addrs, l.BSSID)
	}
	text := strings.Join(addrs, ", ")
	if rest := len(fresh) - len(addrs); rest > 0 {
		text += fmt.Sprintf(" and %d more", rest)
	}
	return fmt.Sprintf("A Wi-Fi network named %q is being advertised from hardware address(es) you have not confirmed: %s. If you did not add an access point, someone may be copying your network's name to catch devices that join it. Do not join it; open the Network page to review it.", fresh[0].Name, text)
}

// Confirm records that the operator says these hardware addresses are their own access points for a network. Only addresses the last
// look saw advertising that very name can be confirmed, so nothing can be added by typing an address.
func (w *WifiWatch) Confirm(name string, addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("choose at least one access point to confirm")
	}
	want := make([]string, 0, len(addrs))
	w.mu.Lock()
	for _, a := range addrs {
		a = strings.ToLower(strings.TrimSpace(a))
		if !slices.ContainsFunc(w.seen, func(ap wifilook.AP) bool { return ap.SSID == name && ap.BSSID == a }) {
			w.mu.Unlock()
			return fmt.Errorf("%s was not seen advertising %q in the last look; look again first", a, name)
		}
		want = append(want, a)
	}
	w.mu.Unlock()
	if err := w.deps.Config.confirm(name, want); err != nil {
		return err
	}
	w.mu.Lock()
	for _, a := range want {
		delete(w.recent, a)
	}
	w.mu.Unlock()
	return nil
}

// WifiSnapshot is what the nightly report reads: the look-alikes seen in the last day.
type WifiSnapshot struct {
	Lookalikes []wifilook.Lookalike
}

// Snapshot is the look-alikes of the last day, empty while the watch is off.
func (w *WifiWatch) Snapshot(now time.Time) WifiSnapshot {
	if !w.deps.Config.Enabled() {
		return WifiSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []wifilook.Lookalike
	for _, r := range w.recent {
		if now.Sub(r.at) <= wifiRecent {
			out = append(out, r.look)
		}
	}
	slices.SortFunc(out, func(a, b wifilook.Lookalike) int { return strings.Compare(a.BSSID, b.BSSID) })
	return WifiSnapshot{Lookalikes: out}
}

// WifiSeen is one access point the last look saw using one of the operator's names.
type WifiSeen struct {
	BSSID     string            `json:"bssid"`
	Signal    int               `json:"signal"`
	Security  wifilook.Security `json:"security"`
	Confirmed bool              `json:"confirmed"`
}

// WifiNetworkView is one of the operator's names with what is confirmed and what the last look saw.
type WifiNetworkView struct {
	Name      string     `json:"name"`
	Confirmed []string   `json:"confirmed"`
	Seen      []WifiSeen `json:"seen"`
}

// WifiView is what the Settings card shows.
type WifiView struct {
	Enabled  bool              `json:"enabled"`
	LastLook time.Time         `json:"last_look,omitzero"`
	Problem  string            `json:"problem,omitempty"`
	Networks []WifiNetworkView `json:"networks"`
}

// View is the card's state: each name, its confirmed addresses, and the addresses the last look saw advertising it.
func (w *WifiWatch) View() WifiView {
	nets := w.deps.Config.Networks()
	w.mu.Lock()
	defer w.mu.Unlock()
	v := WifiView{Enabled: w.deps.Config.Enabled(), LastLook: w.lastLook, Networks: make([]WifiNetworkView, 0, len(nets))}
	if w.failed {
		v.Problem = w.lastErr
	}
	for _, n := range nets {
		nv := WifiNetworkView{Name: n.Name, Confirmed: append([]string{}, n.Addresses...), Seen: []WifiSeen{}}
		for _, a := range w.seen {
			if a.SSID == n.Name {
				nv.Seen = append(nv.Seen, WifiSeen{BSSID: a.BSSID, Signal: a.Signal, Security: a.Security, Confirmed: slices.Contains(n.Addresses, a.BSSID)})
			}
		}
		v.Networks = append(v.Networks, nv)
	}
	return v
}

// Reading is the part's health for the monitor: failed while the last look failed, working once one has worked.
func (w *WifiWatch) Reading() MonitorReading {
	w.mu.Lock()
	defer w.mu.Unlock()
	return MonitorReading{On: w.deps.Config.Enabled() && len(w.deps.Config.Networks()) > 0, LastGood: w.lastGood, Limit: 3 * wifiWatchEvery, Failed: w.failed}
}

// WifiFindings turns the last day's look-alikes into one finding about this computer (the look is made from here), with the worst
// security among them as its severity and a version that changes with the set, so an acceptance lapses when another copy appears.
func WifiFindings(s WifiSnapshot) []Finding {
	if len(s.Lookalikes) == 0 {
		return nil
	}
	severity, addrs := SeverityMedium, make([]string, 0, len(s.Lookalikes))
	for _, l := range s.Lookalikes {
		addrs = append(addrs, l.BSSID)
		if l.Security.Weak() {
			severity = SeverityHigh
		}
	}
	shown := addrs[:min(len(addrs), maxWifiListed)]
	text := strings.Join(shown, ", ")
	if rest := len(addrs) - len(shown); rest > 0 {
		text += fmt.Sprintf(" and %d more", rest)
	}
	return []Finding{{Device: HostDevice, Check: CheckWifiLookalike, Severity: severity, Version: namesDigest(addrs),
		Evidence: fmt.Sprintf("A Wi-Fi network named %q was advertised in the last day from hardware address(es) you have not confirmed: %s.", s.Lookalikes[0].Name, text)}}
}

// WifiSource is what the reporter reads of the Wi-Fi watch.
type WifiSource interface {
	Snapshot(now time.Time) WifiSnapshot
}
