package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/knownbad"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/platform/loginlog"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

type memPolicy struct{ cfg models.NetauditConfig }

func (m *memPolicy) Get() models.NetauditConfig { return m.cfg }
func (m *memPolicy) Update(f func(*models.NetauditConfig) error) error {
	next := m.cfg
	if err := f(&next); err != nil {
		return err
	}
	m.cfg = next
	return nil
}

type memAgents struct{ cfg models.AgentsConfig }

func (m *memAgents) Get() models.AgentsConfig { return m.cfg }
func (m *memAgents) Update(f func(*models.AgentsConfig) error) error {
	next := m.cfg
	if err := f(&next); err != nil {
		return err
	}
	m.cfg = next
	return nil
}

func testNetworkServices(store netaudit.PolicyStore) *netauditServices {
	return &netauditServices{
		PresenceConfig:      netaudit.NewPresenceConfig(store),
		Discovery6:          netaudit.NewDiscovery6Config(store),
		NamesConfig:         netaudit.NewNamesConfig(store),
		NearbyConfig:        netaudit.NewNearbyConfig(store),
		RouterLogConfig:     netaudit.NewRouterLogConfig(store),
		DecoyConfig:         netaudit.NewDecoyConfig(store),
		FollowUpConfig:      netaudit.NewFollowUpConfig(store),
		OutsideLookupConfig: netaudit.NewOutsideLookupConfig(store),
		DNSCheckConfig:      netaudit.NewDNSCheckConfig(store),
		Summary:             netaudit.NewSummaryConfig(store),
		CheckInConfig:       netaudit.NewCheckInConfig(store),
		AttackPathsConfig:   netaudit.NewAttackPathsConfig(store),
		PhoneHomeConfig:     netaudit.NewPhoneHomeConfig(store),
		WebChecksConfig:     netaudit.NewWebChecksConfig(store),
		LoginWatchConfig:    netaudit.NewLoginWatchConfig(store),
		KnownBadConfig:      netaudit.NewKnownBadConfig(store),
		KnownBadList:        &knownBadList{manager: mustKnownBadManager()},
	}
}

type testStackDeps struct {
	store  *memPolicy
	agents *memAgents
	guard  models.NetworkGuardrailsConfig
	reg    *stack.Registry
}

func newTestStacks(t *testing.T) *testStackDeps {
	t.Helper()
	d := &testStackDeps{store: &memPolicy{}, agents: &memAgents{}, guard: models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: true}}
	reg, err := newStackRegistry(stackDeps{
		Network: testNetworkServices(d.store),
		Agents:  d.agents,
		Guard:   func() models.NetworkGuardrailsConfig { return d.guard },
	})
	if err != nil {
		t.Fatalf("newStackRegistry: %v", err)
	}
	d.reg = reg
	return d
}

// The Network stack has sixteen switches today (eleven when this was first written) (the plan said twelve, which was a miscount: the nightly audit is an automation,
// not a switch). This pins the ids and their layers, so adding or moving one is a deliberate edit.
func TestStackRegistry_NetworkModulesAndTheirLayers(t *testing.T) {
	want := map[string]string{
		"presence": "discover", "ipv6_discovery": "discover", "device_names": "discover", "nearby": "discover",
		"router_log": "listen", "phone_home": "listen", "known_bad": "listen", "decoys": "listen", "login_watch": "listen",
		"scan_on_arrival": "test", "web_checks": "test",
		"outside_lookup": "outside", "dns_check": "outside",
		"attack_paths": "understand", "weekly_summary": "tell", "daily_check_in": "tell",
	}
	got := map[string]string{}
	for _, m := range allModules(t, newTestStacks(t).reg) {
		if m.Stack == stack.Network {
			got[m.ID] = m.Layer
		}
	}
	if len(got) != len(want) {
		t.Fatalf("network modules = %v, want %v", got, want)
	}
	for id, layer := range want {
		if got[id] != layer {
			t.Errorf("module %s on layer %q, want %q", id, got[id], layer)
		}
	}
}

func TestStackRegistry_AgentsHoldsTheTwoPartsAndLockDownIsItsAction(t *testing.T) {
	v, err := newTestStacks(t).reg.View(stack.Agents, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, l := range v.Layers {
		for _, m := range l.Modules {
			ids = append(ids, m.ID)
			if !m.StepUp {
				t.Errorf("%s: switching off a security record must need the passphrase", m.ID)
			}
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"agent_activity", "secrets_pass"}) {
		t.Fatalf("agents modules = %v", ids)
	}
	if len(v.Actions) != 1 || v.Actions[0].ID != "lockdown" {
		t.Fatalf("actions = %+v, want only lock down", v.Actions)
	}
}

// legacyPaths are the routes each module was switched on before the registry. A module needs the passphrase exactly when its
// old route does, so the two lists cannot drift apart.
var legacyPaths = map[string]string{
	"presence": "/admin/api/netaudit/presence", "ipv6_discovery": "/admin/api/netaudit/ipv6-discovery",
	"device_names": "/admin/api/netaudit/device-names", "nearby": "/admin/api/netaudit/nearby",
	"router_log": "/admin/api/netaudit/router-log", "decoys": "/admin/api/netaudit/decoys",
	"scan_on_arrival": "/admin/api/netaudit/follow-up-scan", "outside_lookup": "/admin/api/netaudit/outside-lookup",
	"dns_check": "/admin/api/netaudit/dns-check", "weekly_summary": "/admin/api/netaudit/weekly-summary",
	"daily_check_in": "/admin/api/netaudit/daily-check-in",
}

// modulesWithNoOldRoute are the modules that never had a switch route of their own, and whether each needs the passphrase. Only a
// module that probes the network or opens a socket does.
var modulesWithNoOldRoute = map[string]bool{"attack_paths": false, "phone_home": false, "known_bad": false, "web_checks": true, "login_watch": false}

func TestStackRegistry_StepUpMatchesTheOldRoutes(t *testing.T) {
	steps := api.StepUpRoutes()
	for _, m := range allModules(t, newTestStacks(t).reg) {
		if m.Stack != stack.Network {
			continue
		}
		if want, ok := modulesWithNoOldRoute[m.ID]; ok {
			if m.StepUp != want {
				t.Errorf("module %s: StepUp = %v, want %v", m.ID, m.StepUp, want)
			}
			continue
		}
		path, ok := legacyPaths[m.ID]
		if !ok {
			t.Fatalf("module %s has no entry in legacyPaths", m.ID)
		}
		old := slices.Contains(steps, api.StepUpRoute{Method: http.MethodPut, Path: path})
		if m.StepUp != old {
			t.Errorf("module %s: StepUp = %v but its old route PUT %s needs the passphrase = %v", m.ID, m.StepUp, path, old)
		}
	}
}

func TestStackRegistry_AModuleSitsOnTheSwitchItAlwaysHad(t *testing.T) {
	d := newTestStacks(t)
	for id, read := range map[string]func() bool{
		"presence":        func() bool { return d.store.cfg.PresenceCheck },
		"ipv6_discovery":  func() bool { return d.store.cfg.IPv6Discovery },
		"device_names":    func() bool { return !d.store.cfg.DeviceNamesOff },
		"nearby":          func() bool { return d.store.cfg.NearbyEnabled },
		"scan_on_arrival": func() bool { return d.store.cfg.FollowUpScan },
		"outside_lookup":  func() bool { return d.store.cfg.OutsideLookupEnabled },
		"dns_check":       func() bool { return d.store.cfg.DNSCheckEnabled },
		"weekly_summary":  func() bool { return d.store.cfg.WeeklySummary },
		"daily_check_in":  func() bool { return d.store.cfg.DailyCheckIn },
		"attack_paths":    func() bool { return !d.store.cfg.AttackPathsOff },
		"phone_home":      func() bool { return d.store.cfg.PhoneHomeEnabled },
		"known_bad":       func() bool { return d.store.cfg.KnownBadEnabled },
		"web_checks":      func() bool { return d.store.cfg.WebChecksEnabled },
		"login_watch":     func() bool { return d.store.cfg.LoginWatchEnabled },
	} {
		m, _ := d.reg.Module(stack.Network, id)
		want := !read()
		if err := m.Switch.Set(want); err != nil {
			t.Fatalf("%s: Set: %v", id, err)
		}
		if read() != want || m.Switch.Enabled() != want {
			t.Errorf("%s: after Set(%v) the stored setting is %v and the module reads %v", id, want, read(), m.Switch.Enabled())
		}
	}
}

func TestStackRegistry_SwitchingThePresenceCheckKeepsItsInterval(t *testing.T) {
	d := newTestStacks(t)
	d.store.cfg.PresenceIntervalMinutes = 30
	m, _ := d.reg.Module(stack.Network, "presence")
	if err := m.Switch.Set(true); err != nil {
		t.Fatal(err)
	}
	if d.store.cfg.PresenceIntervalMinutes != 30 || !d.store.cfg.PresenceCheck {
		t.Fatalf("config = %+v; the switch must not reset the interval", d.store.cfg)
	}
}

func TestStackRegistry_SwitchesThatNeedAnotherModuleSayWhich(t *testing.T) {
	d := newTestStacks(t)
	for _, id := range []string{"scan_on_arrival", "weekly_summary"} {
		_, err := d.reg.Switch(stack.Network, id, true)
		var ne *stack.NeedError
		if !errors.As(err, &ne) || ne.Need != "Presence check" {
			t.Errorf("%s: err = %v, want a refusal naming the Presence check", id, err)
		}
	}
	if _, err := d.reg.Switch(stack.Network, "presence", true); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reg.Switch(stack.Network, "scan_on_arrival", true); err != nil {
		t.Fatalf("with presence on: %v", err)
	}
}

func TestStackRegistry_GuardrailsShowAsBlockedAndNeverRefuseASwitch(t *testing.T) {
	d := newTestStacks(t)
	d.guard = models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: false, AllowInternetAccess: false}
	blocked := map[string]string{}
	v, _ := d.reg.View(stack.Network, nil)
	for _, l := range v.Layers {
		for _, m := range l.Modules {
			if m.Blocked != "" {
				blocked[m.ID] = m.Blocked
			}
		}
	}
	for _, id := range []string{"nearby", "dns_check", "outside_lookup"} {
		if blocked[id] == "" {
			t.Errorf("%s must be blocked while the Guardrails block the network", id)
		}
	}
	if _, err := d.reg.Switch(stack.Network, "nearby", true); err != nil {
		t.Fatalf("Nearby can be switched on with Guardrails off today and must stay so: %v", err)
	}
	d.guard = models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, AllowInternetAccess: false}
	v, _ = d.reg.View(stack.Network, nil)
	for _, l := range v.Layers {
		for _, m := range l.Modules {
			if m.ID == "nearby" && m.Blocked != "" {
				t.Errorf("nearby blocked with the local network allowed: %q", m.Blocked)
			}
			if m.ID == "dns_check" && m.Blocked == "" {
				t.Error("the DNS check asks an outside resolver, so it stays blocked while the internet is blocked")
			}
		}
	}
}

func TestStackRegistry_TheAgentPartsAreOnUntilSwitchedOff(t *testing.T) {
	d := newTestStacks(t)
	for id, off := range map[string]func() bool{
		"agent_activity": func() bool { return d.agents.cfg.ActivityOff },
		"secrets_pass":   func() bool { return d.agents.cfg.SecretsPassOff },
	} {
		m, _ := d.reg.Module(stack.Agents, id)
		if !m.Switch.Enabled() {
			t.Errorf("%s must be on when nothing is stored", id)
		}
		if _, err := d.reg.Switch(stack.Agents, id, false); err != nil || !off() || m.Switch.Enabled() {
			t.Errorf("%s: off err = %v, stored off = %v", id, err, off())
		}
		if _, err := d.reg.Switch(stack.Agents, id, true); err != nil || off() {
			t.Errorf("%s: on err = %v, stored off = %v", id, err, off())
		}
	}
}

type pokeSwitch struct {
	on  bool
	err error
}

func (p *pokeSwitch) Enabled() bool { return p.on }
func (p *pokeSwitch) Set(e bool) error {
	if p.err != nil {
		return p.err
	}
	p.on = e
	return nil
}

func TestWithPoke_WakesTheListenerAfterAChangeAndNotAfterAFailure(t *testing.T) {
	inner := &pokeSwitch{}
	pokes := 0
	sw := withPoke(inner, func() { pokes++ })
	if err := sw.Set(true); err != nil || !inner.on || pokes != 1 {
		t.Fatalf("Set: err = %v, on = %v, pokes = %d; want the change then one poke", err, inner.on, pokes)
	}
	inner.err = errors.New("disk full")
	if err := sw.Set(false); err == nil || pokes != 1 {
		t.Fatalf("a failed write must not poke: err = %v, pokes = %d", err, pokes)
	}
	if !sw.Enabled() {
		t.Error("Enabled must read through")
	}
}

// The one switching route cannot be listed per module among the middleware's step-up routes, so the handler asks. This walks
// every module marked as needing the passphrase and expects a refusal without it, and no change.
func TestStackRoute_EveryStepUpModuleIsRefusedWithoutThePassphrase(t *testing.T) {
	d := newTestStacks(t)
	mux := http.NewServeMux()
	h := handlers.NewStackHandlers(d.reg, nil)
	mux.HandleFunc("PUT /admin/api/stacks/{stack}/modules/{id}", h.PutModule)

	walked := 0
	for _, m := range allModules(t, d.reg) {
		if !m.StepUp {
			continue
		}
		walked++
		before := m.Switch.Enabled()
		req := httptest.NewRequest(http.MethodPut, "/admin/api/stacks/"+string(m.Stack)+"/modules/"+m.ID, strings.NewReader(`{"enabled":`+strconv.FormatBool(!before)+`}`)).
			WithContext(api.WithStepUp(context.Background(), false))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s/%s: PUT without the passphrase = %d %s, want 403", m.Stack, m.ID, rec.Code, rec.Body.String())
		}
		if m.Switch.Enabled() != before {
			t.Errorf("%s/%s changed although the passphrase was missing", m.Stack, m.ID)
		}
	}
	if walked < 8 {
		t.Fatalf("walked only %d step-up modules; the check is not covering the registry", walked)
	}
}

func TestStackRegistry_ThePhoneHomeWatchNeedsTheRouterLog(t *testing.T) {
	d := newTestStacks(t)
	_, err := d.reg.Switch(stack.Network, "phone_home", true)
	var ne *stack.NeedError
	if !errors.As(err, &ne) || ne.Need != "Router log" {
		t.Fatalf("err = %v, want a refusal naming the Router log", err)
	}
	d.store.cfg.RouterLogEnabled = true
	if _, err := d.reg.Switch(stack.Network, "phone_home", true); err != nil {
		t.Fatalf("with the router log on: %v", err)
	}
}

type emptyKnownBadStore struct{}

func (emptyKnownBadStore) Read() (models.KnownBad, error) { return models.KnownBad{}, nil }
func (emptyKnownBadStore) Write(models.KnownBad) error    { return nil }

func mustKnownBadManager() *knownbad.Manager {
	m, _ := knownbad.NewManager(knownbad.Deps{Store: emptyKnownBadStore{}, Address: func() string { return "" }})
	return m
}

func TestStackRegistry_TheKnownBadComparisonNeedsThePhoneHomeWatch(t *testing.T) {
	d := newTestStacks(t)
	_, err := d.reg.Switch(stack.Network, "known_bad", true)
	var ne *stack.NeedError
	if !errors.As(err, &ne) || ne.Need != "Phone-home watch" {
		t.Fatalf("err = %v, want a refusal naming the Phone-home watch", err)
	}
}

// allModules lists every module of every stack, drawn from the views the screen reads.
func allModules(t *testing.T, reg *stack.Registry) []stack.Module {
	t.Helper()
	var out []stack.Module
	for _, id := range []stack.StackID{stack.Network, stack.Agents} {
		v, err := reg.View(id, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range v.Layers {
			for _, mv := range l.Modules {
				m, ok := reg.Module(id, mv.ID)
				if !ok {
					t.Fatalf("module %s of %s is in the view but not in the registry", mv.ID, id)
				}
				out = append(out, m)
			}
		}
	}
	return out
}

func TestLoginRecordReady_ExplainsAMissingGroupInPlainWords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "btmp")
	if err := os.WriteFile(path, nil, 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	ok, why := loginRecordReady(loginlog.Reader{Path: path})
	if ok || !strings.Contains(why, "utmp group") {
		t.Fatalf("ready = %v why = %q, want the fix named", ok, why)
	}
	if ok, why := loginRecordReady(loginlog.Reader{Path: filepath.Join(dir, "none")}); !ok || why != "" {
		t.Fatalf("a record that does not exist is no failures: ready = %v why = %q", ok, why)
	}
}

// Every module tells a person what it watches, what it never does and where its results appear, in one plain sentence each, so the
// Modules tab and the Help can say it without anyone writing it twice.
func TestStackRegistry_EveryModuleSaysWhatItDoes(t *testing.T) {
	d := newTestStacks(t)
	claimed := map[string]string{}
	for _, m := range allModules(t, d.reg) {
		for field, text := range map[string]string{"watches": m.About.Watches, "never": m.About.Never, "shows up": m.About.ShowsUp} {
			if text == "" || strings.ContainsAny(text, "\n`<>|") || !strings.HasSuffix(text, ".") || len(text) > 220 {
				t.Errorf("module %s: %s is %q, want one plain sentence of at most 220 characters", m.ID, field, text)
			}
		}
		for _, check := range m.Finds {
			if netaudit.CheckTitle(check) == "" {
				t.Errorf("module %s claims %q, which is not a check with a title", m.ID, check)
			}
			if other, dup := claimed[check]; dup {
				t.Errorf("check %q is claimed by %s and %s", check, other, m.ID)
			}
			claimed[check] = m.ID
		}
	}
}

func TestStackRegistry_TheLayersAreNamedByWhatYouWantToKnow(t *testing.T) {
	v, err := newTestStacks(t).reg.View(stack.Network, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range v.Layers {
		got[l.ID] = l.Title
	}
	want := map[string]string{
		"discover": "Who is on the network", "listen": "What the network says", "test": "Look closer",
		"outside": "What the internet sees", "understand": "Decide what to fix", "tell": "Reach you",
	}
	for id, title := range want {
		if got[id] != title {
			t.Errorf("layer %s is titled %q, want %q", id, got[id], title)
		}
	}
}

func TestStackRegistry_TheCertificateChecksAreNamedWithTheWebChecks(t *testing.T) {
	m, ok := newTestStacks(t).reg.Module(stack.Network, "web_checks")
	if !ok || m.Title != "Web and certificate checks, nightly" {
		t.Fatalf("module = %+v", m)
	}
}
